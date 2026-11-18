package voucher

import (
	"context"
	"errors"
	"strconv"

	"github.com/redis/go-redis/v9"
)

type redisVouchers struct{ client *redis.Client }

func stockKey(id int64) string  { return "seckill:stock:" + strconv.FormatInt(id, 10) }
func buyersKey(id int64) string { return "seckill:order:" + strconv.FormatInt(id, 10) }

var reserveVoucher = redis.NewScript(`
local value = redis.call('GET', KEYS[1])
if not value or tonumber(value) == nil or tonumber(value) <= 0 then return 1 end
if not string.match(value, '^%d+$') or tonumber(value) > 2147483647 then
    return redis.error_reply('invalid seckill stock')
end
if redis.call('SISMEMBER', KEYS[2], ARGV[1]) == 1 then return 2 end
redis.call('XADD', KEYS[3], '*', 'userId', ARGV[1], 'voucherId', ARGV[2], 'id', ARGV[3])
redis.call('DECR', KEYS[1])
redis.call('SADD', KEYS[2], ARGV[1])
return 0
`)

func (r redisVouchers) setStock(ctx context.Context, id int64, stock int) error {
	return r.client.Set(ctx, stockKey(id), stock, 0).Err()
}

func (r redisVouchers) reserve(ctx context.Context, orderID, userID, voucherID int64) error {
	code, err := reserveVoucher.Run(ctx, r.client, []string{stockKey(voucherID), buyersKey(voucherID), orderStream}, userID, voucherID, orderID).Int()
	if err != nil {
		return err
	}
	switch code {
	case 0:
		return nil
	case 1:
		return voucherSoldOut
	case 2:
		return voucherDuplicate
	default:
		return errors.New("invalid seckill result")
	}
}
