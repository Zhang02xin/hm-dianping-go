package voucher

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Zhang02xin/hm-dianping-go/internal/httpapi"
	"github.com/Zhang02xin/hm-dianping-go/internal/user"

	"github.com/gin-gonic/gin"
)

func (s *Service) createHandler(seckill bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input voucherInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, httpapi.Result{ErrorMsg: errInvalidVoucher.Error()})
			return
		}
		id, err := s.createVoucher(c.Request.Context(), input, seckill)
		if errors.Is(err, errInvalidVoucher) || errors.Is(err, errInvalidWindow) {
			c.JSON(http.StatusBadRequest, httpapi.Result{ErrorMsg: err.Error()})
			return
		}
		if errors.Is(err, errAdmissionUnavailable) {
			c.JSON(http.StatusServiceUnavailable, httpapi.Result{ErrorMsg: err.Error()})
			return
		}
		if err != nil {
			httpapi.ServerError(c)
			return
		}
		c.JSON(http.StatusOK, httpapi.Result{Success: true, Data: id})
	}
}

func (s *Service) listHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("shopId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, httpapi.Result{ErrorMsg: "店铺ID无效"})
		return
	}
	items, err := s.listVouchers(c.Request.Context(), id)
	if err != nil {
		httpapi.ServerError(c)
		return
	}
	c.JSON(http.StatusOK, httpapi.Result{Success: true, Data: items})
}

func (s *Service) Register(router *gin.Engine) {
	group := router.Group("/voucher")
	orders := router.Group("/voucher-order", user.RequireLogin)
	group.POST("", s.createHandler(false))
	group.POST("/seckill", s.createHandler(true))
	group.GET("/list/:shopId", s.listHandler)
	if s.nextID == nil {
		orders.POST("/seckill/:id", func(c *gin.Context) {
			c.JSON(http.StatusServiceUnavailable, httpapi.Result{ErrorMsg: "优惠券服务未配置"})
		})
		return
	}
	orders.POST("/seckill/:id", func(c *gin.Context) {
		voucherID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || voucherID <= 0 {
			c.JSON(http.StatusBadRequest, httpapi.Result{ErrorMsg: "优惠券ID无效"})
			return
		}
		ctx := c.Request.Context()
		id, err := s.placeOrder(ctx, user.Current(c).ID, voucherID)
		var rejected voucherRejection
		switch {
		case errors.As(err, &rejected):
			c.JSON(http.StatusOK, httpapi.Result{ErrorMsg: rejected.Error()})
		case err != nil:
			httpapi.ServerError(c)
		default:
			c.JSON(http.StatusOK, httpapi.Result{Success: true, Data: id})
		}
	})
}
