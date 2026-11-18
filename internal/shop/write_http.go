package shop

import (
	"net/http"

	"github.com/Zhang02xin/hm-dianping-go/internal/httpapi"

	"github.com/gin-gonic/gin"
)

func saveShopHandler(writer shopWriter, update bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input shopWriteInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, httpapi.Result{ErrorMsg: "invalid shop body"})
			return
		}
		ctx := c.Request.Context()
		var err error
		if update {
			if input.ID == nil {
				c.JSON(http.StatusOK, httpapi.Result{ErrorMsg: "id不能为空"})
				return
			}
			err = writer.UpdateShop(ctx, *input.ID, input)
		} else {
			if input.Name == nil || input.TypeID == nil || input.Images == nil || input.Address == nil || input.X == nil || input.Y == nil {
				c.JSON(http.StatusBadRequest, httpapi.Result{ErrorMsg: "商铺必填字段缺失"})
				return
			}
			if input.Sold == nil {
				input.Sold = new(0)
			}
			if input.Comments == nil {
				input.Comments = new(0)
			}
			if input.Score == nil {
				input.Score = new(0)
			}
			_, err = writer.CreateShop(ctx, input)
		}
		if err != nil {
			c.JSON(http.StatusOK, httpapi.Result{ErrorMsg: "服务器异常"})
			return
		}
		c.JSON(http.StatusOK, httpapi.Result{Success: true})
	}
}
