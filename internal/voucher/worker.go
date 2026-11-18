package voucher

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	orderStream = "stream.orders"
	orderGroup  = "g1"
)

type voucherWorker struct {
	repo     sqlVouchers
	redis    *redis.Client
	consumer string
}

func startVoucherWorker(ctx context.Context, db *sql.DB, client *redis.Client) (func(), error) {
	w := voucherWorker{repo: sqlVouchers{db: db}, redis: client, consumer: rand.Text()}
	if err := w.ensureGroup(ctx); err != nil {
		return nil, errors.New("voucher order stream initialization failed")
	}
	workerCtx, cancel := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	workers.Go(func() { w.consumeLoop(workerCtx) })
	return func() { cancel(); workers.Wait() }, nil
}

func (w voucherWorker) ensureGroup(ctx context.Context) error {
	err := w.redis.XGroupCreateMkStream(ctx, orderStream, orderGroup, "0").Err()
	if redis.HasErrorPrefix(err, "BUSYGROUP") {
		return nil
	}
	return err
}

func (w voucherWorker) consumeLoop(ctx context.Context) {
	cursor := "0-0"
	for ctx.Err() == nil {
		next, err := w.consume(ctx, cursor)
		cursor = next
		if err == nil || errors.Is(err, redis.Nil) {
			continue
		}
		if redis.HasErrorPrefix(err, "NOGROUP") {
			groupCtx, cancel := context.WithTimeout(ctx, time.Second)
			_ = w.ensureGroup(groupCtx)
			cancel()
			cursor = "0-0"
		}
		if ctx.Err() == nil {
			log.Print("voucher stream consumer failed; pending messages will retry")
		}
		if !waitForVoucherRetry(ctx) {
			return
		}
	}
}

func waitForVoucherRetry(ctx context.Context) bool {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (w voucherWorker) consume(ctx context.Context, cursor string) (string, error) {
	claimCtx, cancel := context.WithTimeout(ctx, time.Second)
	claimed, next, err := w.redis.XAutoClaim(claimCtx, &redis.XAutoClaimArgs{
		Stream: orderStream, Group: orderGroup, Consumer: w.consumer,
		MinIdle: 30 * time.Second, Start: cursor, Count: 8,
	}).Result()
	cancel()
	if err != nil && !errors.Is(err, redis.Nil) {
		return cursor, err
	}
	if next == "" {
		next = "0-0"
	}
	for _, msg := range claimed {
		w.handle(ctx, msg)
	}
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	streams, err := w.redis.XReadGroup(readCtx, &redis.XReadGroupArgs{
		Group: orderGroup, Consumer: w.consumer, Streams: []string{orderStream, ">"}, Block: time.Second, Count: 8,
	}).Result()
	cancel()
	if err != nil {
		return next, err
	}
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			w.handle(ctx, msg)
		}
	}
	return next, nil
}

func (w voucherWorker) handle(ctx context.Context, msg redis.XMessage) {
	id, idOK := positiveMessageID(msg, "id")
	userID, userOK := positiveMessageID(msg, "userId")
	voucherID, voucherOK := positiveMessageID(msg, "voucherId")
	if !idOK || !userOK || !voucherOK {
		// Leave malformed legacy messages visible in the pending list for inspection.
		log.Print("malformed voucher stream message remains pending")
		return
	}
	if err := w.repo.persistOrder(ctx, id, userID, voucherID); err != nil {
		if ctx.Err() == nil {
			log.Printf("voucher order %d could not be finalized; it will retry", id)
		}
		return
	}
	ackCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := w.redis.XAck(ackCtx, orderStream, orderGroup, msg.ID).Err(); err != nil && ctx.Err() == nil {
		log.Print("voucher stream acknowledgment failed; message may be redelivered")
	}
}

func positiveMessageID(msg redis.XMessage, key string) (int64, bool) {
	value, _ := msg.Values[key].(string)
	id, err := strconv.ParseInt(value, 10, 64)
	return id, err == nil && id > 0
}
