package voucher

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type voucherRejection string

func (r voucherRejection) Error() string { return string(r) }

const (
	voucherSoldOut   voucherRejection = "库存不足"
	voucherDuplicate voucherRejection = "禁止重复下单"
)

const (
	lockVoucherSQL        = "SELECT voucher_id FROM tb_seckill_voucher WHERE voucher_id = ? FOR UPDATE"
	voucherOrderExistsSQL = "SELECT EXISTS(SELECT 1 FROM tb_voucher_order WHERE voucher_id = ? AND user_id = ?)"
	decrementVoucherSQL   = "UPDATE tb_seckill_voucher SET stock = stock - 1 WHERE voucher_id = ? AND stock > 0"
	insertVoucherOrderSQL = "INSERT INTO tb_voucher_order (id, user_id, voucher_id) VALUES (?, ?, ?)"
)

func (r sqlVouchers) persistOrder(ctx context.Context, orderID, userID, voucherID int64) error {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRowContext(ctx, lockVoucherSQL, voucherID).Scan(&id); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, voucherOrderExistsSQL, voucherID, userID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	result, err := tx.ExecContext(ctx, decrementVoucherSQL, voucherID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("reserved order has no MySQL stock")
	}
	if _, err := tx.ExecContext(ctx, insertVoucherOrderSQL, orderID, userID, voucherID); err != nil {
		return err
	}
	return tx.Commit()
}
