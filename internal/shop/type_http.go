package shop

import (
	"net/http"

	"github.com/Zhang02xin/hm-dianping-go/internal/httpapi"

	"github.com/gin-gonic/gin"
)

func listShopTypesHandler(reader shopTypeReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		types, err := reader.ListShopTypes(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, httpapi.Result{ErrorMsg: "服务器异常"})
			return
		}
		c.JSON(http.StatusOK, httpapi.Result{Success: true, Data: types})
	}
}
