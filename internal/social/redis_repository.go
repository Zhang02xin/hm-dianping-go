package social

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Zhang02xin/hm-dianping-go/internal/user"
	"github.com/redis/go-redis/v9"
)

type socialData struct {
	sqlSocial
	cache *redis.Client
}

var errRedisUnavailable = errors.New("social Redis unavailable")

func likesKey(id int64) string   { return "blog:liked:" + strconv.FormatInt(id, 10) }
func feedKey(id int64) string    { return "feed:" + strconv.FormatInt(id, 10) }
func followsKey(id int64) string { return "follows:" + strconv.FormatInt(id, 10) }

func (s socialData) markLiked(ctx context.Context, b *blog, viewer int64) error {
	if viewer == 0 {
		return nil
	}
	if s.cache == nil {
		return errRedisUnavailable
	}
	_, err := s.cache.ZScore(ctx, likesKey(b.ID), strconv.FormatInt(viewer, 10)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	b.IsLike = new(err == nil)
	return nil
}

func (s socialData) findBlog(ctx context.Context, id, viewerID int64) (blog, error) {
	b, err := s.sqlSocial.findBlog(ctx, id)
	if err == nil {
		err = s.markLiked(ctx, &b, viewerID)
	}
	return b, err
}

func (s socialData) listBlogs(ctx context.Context, authorID, viewerID, offset int64, hot bool) ([]blog, error) {
	items, err := s.sqlSocial.listBlogs(ctx, authorID, offset, hot)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if hot {
			if err := s.markLiked(ctx, &items[i], viewerID); err != nil {
				return nil, err
			}
		}
	}
	return items, nil
}

func (s socialData) createBlog(ctx context.Context, userID int64, input blogInput) (int64, error) {
	if s.cache == nil {
		return 0, errRedisUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	followers, err := s.followers(ctx, userID)
	if err != nil {
		return 0, err
	}
	id, err := s.sqlSocial.createBlog(ctx, userID, input)
	if err != nil || len(followers) == 0 {
		return id, err
	}
	score := float64(time.Now().UnixMilli())
	_, err = s.cache.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, reader := range followers {
			pipe.ZAdd(ctx, feedKey(reader), redis.Z{Score: score, Member: strconv.FormatInt(id, 10)})
		}
		return nil
	})
	return id, err
}

func (s socialData) feed(ctx context.Context, viewerID, max, offset int64) ([]feedEntry, error) {
	if s.cache == nil {
		return nil, errRedisUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	items, err := s.cache.ZRevRangeByScoreWithScores(ctx, feedKey(viewerID), &redis.ZRangeBy{
		Min: "0", Max: strconv.FormatInt(max, 10), Offset: offset, Count: 2,
	}).Result()
	if err != nil {
		return nil, err
	}
	var result []feedEntry
	for _, item := range items {
		member, _ := item.Member.(string)
		id, err := strconv.ParseInt(member, 10, 64)
		if err != nil {
			return nil, err
		}
		b, err := s.findFeedBlog(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			result = append(result, feedEntry{publishedAt: int64(item.Score)})
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := s.markLiked(ctx, &b, viewerID); err != nil {
			return nil, err
		}
		result = append(result, feedEntry{blog: b, publishedAt: int64(item.Score)})
	}
	return result, nil
}

func (s socialData) toggleBlogLike(ctx context.Context, blogID, userID int64) error {
	if s.cache == nil {
		return errRedisUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM tb_blog WHERE id = ? FOR UPDATE", blogID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	member := strconv.FormatInt(userID, 10)
	key := likesKey(blogID)
	_, err = s.cache.ZScore(ctx, key, member).Result()
	liked := err == nil
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	query := "UPDATE tb_blog SET liked = COALESCE(liked, 0) + 1 WHERE id = ?"
	if liked {
		query = "UPDATE tb_blog SET liked = GREATEST(CAST(COALESCE(liked, 0) AS SIGNED) - 1, 0) WHERE id = ?"
	}
	if _, err = tx.ExecContext(ctx, query, blogID); err != nil {
		return err
	}
	if liked {
		err = s.cache.ZRem(ctx, key, member).Err()
	} else {
		err = s.cache.ZAdd(ctx, key, redis.Z{Score: float64(time.Now().UnixMilli()), Member: member}).Err()
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s socialData) blogLikers(ctx context.Context, blogID int64) ([]user.Summary, error) {
	if s.cache == nil {
		return nil, errRedisUnavailable
	}
	ids, err := s.cache.ZRange(ctx, likesKey(blogID), 0, 6).Result()
	if err != nil {
		return nil, err
	}
	return s.usersInOrder(ctx, ids)
}

func (s socialData) setFollow(ctx context.Context, userID, targetID int64, follow bool) error {
	if s.cache == nil {
		return errRedisUnavailable
	}
	if err := s.sqlSocial.setFollow(ctx, userID, targetID, follow); err != nil {
		return err
	}
	if follow {
		return s.cache.SAdd(ctx, followsKey(userID), strconv.FormatInt(targetID, 10)).Err()
	}
	return s.cache.SRem(ctx, followsKey(userID), strconv.FormatInt(targetID, 10)).Err()
}

func (s socialData) commonFollows(ctx context.Context, userID, targetID int64) ([]user.Summary, error) {
	if s.cache == nil {
		return nil, errRedisUnavailable
	}
	ids, err := s.cache.SInter(ctx, followsKey(userID), followsKey(targetID)).Result()
	if err != nil {
		return nil, err
	}
	return s.usersInOrder(ctx, ids)
}

func (s sqlSocial) usersInOrder(ctx context.Context, members []string) ([]user.Summary, error) {
	if len(members) == 0 {
		return []user.Summary{}, nil
	}
	args := make([]any, 0, len(members))
	for _, member := range members {
		id, err := strconv.ParseInt(member, 10, 64)
		if err != nil {
			return nil, err
		}
		args = append(args, id)
	}
	rows, err := s.listSocialUsers(ctx, "SELECT id, nick_name, icon FROM tb_user WHERE id IN ("+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+")", args...)
	if err != nil {
		return nil, err
	}
	values := make(map[int64]user.Summary, len(rows))
	for _, row := range rows {
		values[row.ID] = row
	}
	ordered := make([]user.Summary, 0, len(rows))
	for _, id := range args {
		if row, ok := values[id.(int64)]; ok {
			ordered = append(ordered, row)
		}
	}
	return ordered, nil
}
