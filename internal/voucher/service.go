package voucher

import (
	"context"
	"errors"
	"strings"
	"time"
)

type nextVoucherOrderID func(context.Context) (int64, error)

type voucherRepository interface {
	list(context.Context, int64) ([]voucher, error)
	create(context.Context, voucherInput, *voucherWindow, func(context.Context, int64) error) (int64, error)
}

type voucherAdmission interface {
	setStock(context.Context, int64, int) error
	reserve(context.Context, int64, int64, int64) error
}

type Service struct {
	admission voucherAdmission
	repo      voucherRepository
	loc       *time.Location
	nextID    nextVoucherOrderID
}

var errInvalidVoucher = errors.New("优惠券参数无效")
var errAdmissionUnavailable = errors.New("秒杀 Redis 未配置")
var errInvalidWindow = errors.New("秒杀时间或库存无效")

func (s *Service) createVoucher(ctx context.Context, input voucherInput, seckill bool) (int64, error) {
	if strings.TrimSpace(input.Title) == "" || input.PayValue == nil || *input.PayValue < 0 || input.ActualValue == nil ||
		(input.Status != nil && (*input.Status < 1 || *input.Status > 3)) {
		return 0, errInvalidVoucher
	}
	var window *voucherWindow
	if seckill {
		const layout = "2006-01-02T15:04:05"
		begin, beginErr := time.ParseInLocation(layout, input.BeginTime, s.loc)
		end, endErr := time.ParseInLocation(layout, input.EndTime, s.loc)
		if beginErr != nil || endErr != nil || !end.After(begin) || input.Stock == nil || *input.Stock < 0 || int64(*input.Stock) > 2147483647 {
			return 0, errInvalidWindow
		}
		window = &voucherWindow{stock: *input.Stock, begin: begin, end: end}
	}
	var initializeStock func(context.Context, int64) error
	if window != nil {
		if s.admission == nil {
			return 0, errAdmissionUnavailable
		}
		initializeStock = func(ctx context.Context, id int64) error {
			return s.admission.setStock(ctx, id, window.stock)
		}
	}
	return s.repo.create(ctx, input, window, initializeStock)
}

func (s *Service) listVouchers(ctx context.Context, shopID int64) ([]voucher, error) {
	return s.repo.list(ctx, shopID)
}

func (s *Service) placeOrder(ctx context.Context, userID, voucherID int64) (int64, error) {
	id, err := s.nextID(ctx)
	if err != nil {
		return 0, err
	}
	if err := s.admission.reserve(ctx, id, userID, voucherID); err != nil {
		return 0, err
	}
	return id, nil
}
