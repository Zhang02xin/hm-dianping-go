package voucher

import (
	"context"
	"database/sql"
	"time"

	"github.com/redis/go-redis/v9"
)

type Module struct {
	*Service
	db    *sql.DB
	redis *redis.Client
}

func New(db *sql.DB, loc *time.Location, client *redis.Client) *Module {
	service := &Service{repo: sqlVouchers{db: db}, loc: loc}
	if client != nil {
		service.admission = redisVouchers{client: client}
		service.nextID = redisOrderIDs(client, time.Now)
	}
	return &Module{Service: service, db: db, redis: client}
}

func (m *Module) StartWorker(ctx context.Context) (func(), error) {
	if m.redis == nil {
		return func() {}, nil
	}
	return startVoucherWorker(ctx, m.db, m.redis)
}
