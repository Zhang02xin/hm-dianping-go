package voucher

import (
	"context"
	"database/sql"
	"time"
)

type sqlVouchers struct {
	db *sql.DB
}

const listVouchersSQL = `SELECT v.id, v.shop_id, v.title, v.sub_title, v.rules, v.pay_value, v.actual_value, v.type, s.stock, s.begin_time, s.end_time
FROM tb_voucher v LEFT JOIN tb_seckill_voucher s ON v.id = s.voucher_id WHERE v.shop_id = ? AND v.status = 1 ORDER BY v.id`

func (r sqlVouchers) list(ctx context.Context, shopID int64) ([]voucher, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rows, err := r.db.QueryContext(ctx, listVouchersSQL, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]voucher, 0)
	for rows.Next() {
		var v voucher
		if err := rows.Scan(&v.ID, &v.ShopID, &v.Title, &v.SubTitle, &v.Rules, &v.PayValue, &v.ActualValue, &v.Type, &v.Stock, &v.BeginTime, &v.EndTime); err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	return items, rows.Err()
}

func (r sqlVouchers) create(ctx context.Context, input voucherInput, window *voucherWindow, initializeStock func(context.Context, int64) error) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	voucherType, status := 0, 1
	if window != nil {
		voucherType = 1
	}
	if input.Status != nil {
		status = *input.Status
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO tb_voucher (shop_id, title, sub_title, rules, pay_value, actual_value, type, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		input.ShopID, input.Title, input.SubTitle, input.Rules, *input.PayValue, *input.ActualValue, voucherType, status)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if window != nil {
		if _, err := tx.ExecContext(ctx, "INSERT INTO tb_seckill_voucher (voucher_id, stock, begin_time, end_time) VALUES (?, ?, ?, ?)", id, window.stock, window.begin, window.end); err != nil {
			return 0, err
		}
		if err := initializeStock(ctx, id); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}
