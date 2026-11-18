package user

import (
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/Zhang02xin/hm-dianping-go/internal/httpapi"

	"github.com/gin-gonic/gin"
)

const authUserKey = "auth.user"
const authTokenKey = "auth.token"

func (a *Service) RefreshSession(c *gin.Context) {
	token := strings.TrimSpace(c.GetHeader("Authorization"))
	if scheme, value, ok := strings.Cut(token, " "); ok && strings.EqualFold(scheme, "Bearer") {
		token = strings.TrimSpace(value)
	}
	if len(token) != 32 {
		return
	}
	if _, err := hex.DecodeString(token); err != nil {
		return
	}
	identity, err := a.lookupSession(c.Request.Context(), token)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusOK, httpapi.Result{ErrorMsg: "服务器异常"})
		return
	}
	if identity != nil {
		SetCurrent(c, *identity)
		c.Set(authTokenKey, token)
	}
}

func SetCurrent(c *gin.Context, identity Summary) { c.Set(authUserKey, identity) }

func Optional(c *gin.Context) (Summary, bool) {
	value, exists := c.Get(authUserKey)
	if !exists {
		return Summary{}, false
	}
	identity, ok := value.(Summary)
	return identity, ok
}

func RequireLogin(c *gin.Context) {
	if _, ok := Optional(c); !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
	}
}

func Current(c *gin.Context) Summary { return c.MustGet(authUserKey).(Summary) }
