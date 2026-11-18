package shop

import (
	"math"
	"net/http"
	"strconv"

	"github.com/Zhang02xin/hm-dianping-go/internal/httpapi"

	"github.com/gin-gonic/gin"
)

func findShopsByNameHandler(reader shopSearchReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset, ok := httpapi.PageOffset(c, shopPageSize)
		if !ok {
			return
		}
		shops, err := reader.FindShopsByName(c.Request.Context(), c.Query("name"), offset)
		if err != nil {
			c.JSON(http.StatusOK, httpapi.Result{ErrorMsg: "服务器异常"})
			return
		}
		c.JSON(http.StatusOK, httpapi.Result{Success: true, Data: shops})
	}
}

func findShopsByTypeHandler(reader shopSearchReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		typeID, err := strconv.ParseInt(c.Query("typeId"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, httpapi.Result{ErrorMsg: "invalid shop type"})
			return
		}
		offset, ok := httpapi.PageOffset(c, shopPageSize)
		if !ok {
			return
		}
		var point *shopCoordinates
		xText, yText := c.Query("x"), c.Query("y")
		x, xErr := strconv.ParseFloat(xText, 64)
		y, yErr := strconv.ParseFloat(yText, 64)
		if xText != "" && (xErr != nil || math.IsNaN(x) || x < -180 || x > 180) ||
			yText != "" && (yErr != nil || math.IsNaN(y) || y < -85.05112878 || y > 85.05112878) {
			c.JSON(http.StatusBadRequest, httpapi.Result{ErrorMsg: "invalid shop coordinates"})
			return
		}
		if xText != "" && yText != "" {
			point = &shopCoordinates{x, y}
		}
		shops, err := reader.FindShopsByType(c.Request.Context(), typeID, point, offset)
		if err != nil {
			c.JSON(http.StatusOK, httpapi.Result{ErrorMsg: "服务器异常"})
			return
		}
		c.JSON(http.StatusOK, httpapi.Result{Success: true, Data: shops})
	}
}
