package httpapi

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Result struct {
	Success  bool   `json:"success"`
	ErrorMsg string `json:"errorMsg,omitempty"`
	Data     any    `json:"data,omitempty"`
}

func PageOffset(c *gin.Context, pageSize int64) (int64, bool) {
	page := c.Query("current")
	if page == "" {
		page = "1"
	}
	current, err := strconv.ParseInt(page, 10, 32)
	if err != nil || current < 1 {
		c.JSON(http.StatusBadRequest, Result{ErrorMsg: "invalid page"})
		return 0, false
	}
	return (current - 1) * pageSize, true
}

func ServerError(c *gin.Context) { c.JSON(http.StatusOK, Result{ErrorMsg: "服务器异常"}) }
