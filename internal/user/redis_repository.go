package user

import (
	"context"
	"errors"
	"strconv"

	"github.com/redis/go-redis/v9"
)

type redisSessions struct{ client *redis.Client }

var storeCode = redis.NewScript(`
if redis.call('EXISTS', KEYS[1], KEYS[2]) > 0 then return 0 end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
return 1
`)

var consumeCode = redis.NewScript(`
local code = redis.call('GET', KEYS[1])
if not code then return 0 end
if code == ARGV[1] then
  redis.call('DEL', KEYS[1], KEYS[2])
  return 1
end
local attempts = redis.call('INCR', KEYS[2])
if attempts == 1 then redis.call('PEXPIRE', KEYS[2], math.max(1, redis.call('PTTL', KEYS[1]))) end
if attempts >= 5 then redis.call('DEL', KEYS[1]) end
return 0
`)

var discardCode = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1], KEYS[2]) end
return 0
`)

func loginCodeKeys(phone string) []string {
	return []string{"login:code:" + phone, "login:attempts:" + phone}
}

func (r redisSessions) putCode(ctx context.Context, phone, code string) (bool, error) {
	n, err := storeCode.Run(ctx, r.client, loginCodeKeys(phone), code, loginCodeTTL.Milliseconds()).Int()
	return n == 1, err
}

func (r redisSessions) consumeCode(ctx context.Context, phone, code string) (bool, error) {
	n, err := consumeCode.Run(ctx, r.client, loginCodeKeys(phone), code).Int()
	return n == 1, err
}

func (r redisSessions) discardCode(ctx context.Context, phone, code string) error {
	return discardCode.Run(ctx, r.client, loginCodeKeys(phone), code).Err()
}

func (r redisSessions) saveSession(ctx context.Context, token string, identity Summary) error {
	key := "login:token:" + token
	fields := map[string]any{"id": identity.ID}
	if identity.NickName != nil {
		fields["nickName"] = *identity.NickName
	}
	if identity.Icon != nil {
		fields["icon"] = *identity.Icon
	}
	_, err := r.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, key, fields)
		pipe.Expire(ctx, key, loginUserTTL)
		return nil
	})
	return err
}
func (r redisSessions) lookupSession(ctx context.Context, token string) (*Summary, error) {
	key := "login:token:" + token
	var lookup *redis.MapStringStringCmd
	_, err := r.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		lookup = pipe.HGetAll(ctx, key)
		pipe.Expire(ctx, key, loginUserTTL)
		return nil
	})
	if err != nil {
		return nil, err
	}
	fields := lookup.Val()
	id, err := strconv.ParseInt(fields["id"], 10, 64)
	if err != nil || id <= 0 {
		return nil, nil
	}
	user := Summary{ID: id}
	if name, ok := fields["nickName"]; ok {
		user.NickName = &name
	}
	if icon, ok := fields["icon"]; ok {
		user.Icon = &icon
	}
	return &user, nil
}

func (r redisSessions) dropSession(ctx context.Context, token string) error {
	return r.client.Del(ctx, "login:token:"+token).Err()
}

func (r redisSessions) setSign(ctx context.Context, key string, offset int64) error {
	return r.client.SetBit(ctx, key, offset, 1).Err()
}

func (r redisSessions) getSigns(ctx context.Context, key string) ([]byte, error) {
	bytes, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	return bytes, err
}
