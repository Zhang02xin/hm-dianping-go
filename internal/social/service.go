package social

import "context"

type feedEntry struct {
	blog        blog
	publishedAt int64
}

type feedRepository interface {
	feed(ctx context.Context, viewerID, max, offset int64) ([]feedEntry, error)
}

func readFeed(ctx context.Context, repo feedRepository, viewerID, max, offset int64) (blogScroll, error) {
	entries, err := repo.feed(ctx, viewerID, max, offset)
	if err != nil {
		return blogScroll{}, err
	}
	var result blogScroll
	for _, entry := range entries {
		if entry.blog.ID != 0 {
			result.List = append(result.List, entry.blog)
		}
		if entry.publishedAt == result.MinTime {
			result.Offset++
		} else {
			result.MinTime, result.Offset = entry.publishedAt, 1
		}
	}
	if len(entries) > 0 && result.MinTime == max {
		result.Offset += offset
	}
	return result, nil
}
