package social

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Zhang02xin/hm-dianping-go/internal/user"
)

type sqlSocial struct{ db *sql.DB }

const blogColumns = "b.id, b.shop_id, b.user_id, b.title, b.images, b.content, b.liked, b.comments, b.create_time, b.update_time"

var errBlogNotFound = errors.New("blog not found")

func (b *blog) scan(row interface{ Scan(...any) error }, extra ...any) error {
	fields := []any{&b.ID, &b.ShopID, &b.UserID, &b.Title, &b.Images, &b.Content, &b.Liked, &b.Comments, &b.CreateTime, &b.UpdateTime}
	return row.Scan(append(fields, extra...)...)
}

func (s sqlSocial) createBlog(ctx context.Context, userID int64, input blogInput) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	result, err := s.db.ExecContext(ctx, "INSERT INTO tb_blog (shop_id, user_id, title, images, content) VALUES (?, ?, ?, ?, ?)",
		input.ShopID, userID, input.Title, *input.Images, *input.Content)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s sqlSocial) findBlog(ctx context.Context, id int64) (blog, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var b blog
	row := s.db.QueryRowContext(ctx, "SELECT "+blogColumns+", u.icon, u.nick_name"+
		" FROM tb_blog b LEFT JOIN tb_user u ON u.id = b.user_id WHERE b.id = ?", id)
	err := b.scan(row, &b.Icon, &b.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return blog{}, errBlogNotFound
	}
	return b, err
}

func (s sqlSocial) listBlogs(ctx context.Context, authorID, offset int64, hot bool) ([]blog, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var rows *sql.Rows
	var err error
	if hot {
		rows, err = s.db.QueryContext(ctx, "SELECT "+blogColumns+", u.icon, u.nick_name"+
			" FROM tb_blog b LEFT JOIN tb_user u ON u.id = b.user_id ORDER BY b.liked DESC, b.id DESC LIMIT 10 OFFSET ?", offset)
	} else {
		rows, err = s.db.QueryContext(ctx, "SELECT "+blogColumns+" FROM tb_blog b WHERE b.user_id = ? ORDER BY b.id DESC LIMIT 10 OFFSET ?", authorID, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]blog, 0)
	for rows.Next() {
		var item blog
		if hot {
			err = item.scan(rows, &item.Icon, &item.Name)
		} else {
			err = item.scan(rows)
		}
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s sqlSocial) findFeedBlog(ctx context.Context, id int64) (blog, error) {
	var b blog
	err := b.scan(s.db.QueryRowContext(ctx, "SELECT "+blogColumns+" FROM tb_blog b WHERE b.id = ?", id))
	return b, err
}

func (s sqlSocial) followers(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT user_id FROM tb_follow WHERE follow_user_id = ?", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s sqlSocial) setFollow(ctx context.Context, userID, targetID int64, follow bool) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM tb_user WHERE id = ? FOR UPDATE", userID).Scan(&id); err != nil {
		return err
	}
	if follow {
		_, err = tx.ExecContext(ctx, `INSERT INTO tb_follow (user_id, follow_user_id)
SELECT ?, ? FROM DUAL WHERE NOT EXISTS(SELECT 1 FROM tb_follow WHERE user_id = ? AND follow_user_id = ?)`, userID, targetID, userID, targetID)
	} else {
		_, err = tx.ExecContext(ctx, "DELETE FROM tb_follow WHERE user_id = ? AND follow_user_id = ?", userID, targetID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s sqlSocial) isFollowing(ctx context.Context, userID, targetID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var following bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM tb_follow WHERE user_id = ? AND follow_user_id = ?)", userID, targetID).Scan(&following)
	return following, err
}

func (s sqlSocial) listSocialUsers(ctx context.Context, query string, args ...any) ([]user.Summary, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]user.Summary, 0)
	for rows.Next() {
		var user user.Summary
		if err := rows.Scan(&user.ID, &user.NickName, &user.Icon); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}
