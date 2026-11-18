package user

import (
	"database/sql"
	"time"

	"github.com/redis/go-redis/v9"
)

func New(db *sql.DB, client *redis.Client, sender CodeSender) *Service {
	if client == nil {
		return nil
	}
	return &Service{sessions: redisSessions{client: client}, users: sqlUsers{db: db}, sender: sender, now: time.Now}
}
