package voucher

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestVoucherWorkerRedelivery(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	server := miniredis.RunT(t)
	now := time.Now()
	server.SetTime(now)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), ContextTimeoutEnabled: true})
	t.Cleanup(func() { _ = client.Close() })
	w := voucherWorker{repo: sqlVouchers{db: db}, redis: client, consumer: "replacement"}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	if err := w.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	const id, userID, voucherID int64 = 103, 42, 21
	message := map[string]any{"id": id, "userId": userID, "voucherId": voucherID}
	if err := client.XAdd(ctx, &redis.XAddArgs{Stream: orderStream, Values: message}).Err(); err != nil {
		t.Fatal(err)
	}
	stream, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: orderGroup, Consumer: "old-worker", Streams: []string{orderStream, ">"}, Block: -1}).Result()
	if err != nil || len(stream) != 1 || len(stream[0].Messages) != 1 {
		t.Fatalf("first consumer did not see dispatched order: %v", err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(lockVoucherSQL).WithArgs(voucherID).WillReturnRows(sqlmock.NewRows([]string{"voucher_id"}).AddRow(voucherID))
	mock.ExpectQuery(voucherOrderExistsSQL).WithArgs(voucherID, userID).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(decrementVoucherSQL).WithArgs(voucherID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(insertVoucherOrderSQL).WithArgs(id, userID, voucherID).WillReturnError(errors.New("database unavailable"))
	mock.ExpectRollback()
	w.handle(ctx, stream[0].Messages[0])
	if pending, err := client.XPending(ctx, orderStream, orderGroup).Result(); err != nil || pending.Count != 1 {
		t.Fatalf("failed order was not left pending: %v", err)
	}
	server.SetTime(now.Add(31 * time.Second))
	mock.ExpectBegin()
	mock.ExpectQuery(lockVoucherSQL).WithArgs(voucherID).WillReturnRows(sqlmock.NewRows([]string{"voucher_id"}).AddRow(voucherID))
	mock.ExpectQuery(voucherOrderExistsSQL).WithArgs(voucherID, userID).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(decrementVoucherSQL).WithArgs(voucherID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(insertVoucherOrderSQL).WithArgs(id, userID, voucherID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if _, err := w.consume(ctx, "0-0"); err != nil && !errors.Is(err, redis.Nil) {
		t.Fatal(err)
	}
	if pending, err := client.XPending(ctx, orderStream, orderGroup).Result(); err != nil || pending.Count != 0 {
		t.Fatalf("finalized order was not acknowledged: %v", err)
	}
	if err := client.XAdd(ctx, &redis.XAddArgs{Stream: orderStream, Values: message}).Err(); err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(lockVoucherSQL).WithArgs(voucherID).WillReturnRows(sqlmock.NewRows([]string{"voucher_id"}).AddRow(voucherID))
	mock.ExpectQuery(voucherOrderExistsSQL).WithArgs(voucherID, userID).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()
	if _, err := w.consume(ctx, "0-0"); err != nil {
		t.Fatal(err)
	}
	if pending, err := client.XPending(ctx, orderStream, orderGroup).Result(); err != nil || pending.Count != 0 {
		t.Fatalf("duplicate message was not cleaned up: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
