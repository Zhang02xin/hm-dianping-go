package shop

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Zhang02xin/hm-dianping-go/internal/httpapi"

	"github.com/gin-gonic/gin"
)

func shopDetailHandler(shops shopReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, httpapi.Result{ErrorMsg: "invalid shop id"})
			return
		}

		shop, err := shops.FindByID(c.Request.Context(), id)
		switch {
		case errors.Is(err, errShopNotFound):
			c.JSON(http.StatusOK, httpapi.Result{ErrorMsg: "店铺不存在"})
		case err != nil:
			// Match the original API's error envelope and HTTP status.
			c.JSON(http.StatusOK, httpapi.Result{ErrorMsg: "服务器异常"})
		default:
			c.JSON(http.StatusOK, httpapi.Result{Success: true, Data: shop})
		}
	}
}
