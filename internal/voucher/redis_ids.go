package voucher

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
)

func redisOrderIDs(client *redis.Client, now func() time.Time) nextVoucherOrderID {
	return func(ctx context.Context) (int64, error) {
		current := now()
		// The Java generator treats the local wall clock as UTC before composing the ID.
		wallTime := time.Date(current.Year(), current.Month(), current.Day(), current.Hour(), current.Minute(), current.Second(), 0, time.UTC)
		seconds := wallTime.Unix() - 1640995200
		if seconds < 0 || seconds > math.MaxInt32 {
			return 0, errors.New("order ID timestamp out of range")
		}
		count, err := client.Incr(ctx, "icr:order:"+current.Format("20060102")).Result()
		if err != nil {
			return 0, err
		}
		if count < 1 || count > math.MaxUint32 {
			return 0, errors.New("order ID counter exhausted")
		}
		return seconds<<32 | count, nil
	}
}
