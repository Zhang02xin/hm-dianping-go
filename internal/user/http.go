package user

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Zhang02xin/hm-dianping-go/internal/httpapi"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, auth *Service) {
	users := router.Group("/user")
	if auth == nil {
		unavailable := func(c *gin.Context) {
			c.JSON(http.StatusServiceUnavailable, httpapi.Result{ErrorMsg: "登录服务未配置"})
		}
		users.POST("/code", unavailable)
		users.POST("/login", unavailable)
	} else {
		users.POST("/code", auth.sendCodeHTTP)
		users.POST("/login", auth.loginHTTP)
	}
	protected := users.Group("", RequireLogin)
	protected.GET("/me", func(c *gin.Context) {
		c.JSON(http.StatusOK, httpapi.Result{Success: true, Data: Current(c)})
	})
	protected.POST("/logout", auth.logoutHTTP)
	protected.GET("/:id", auth.findUserHTTP)
	protected.GET("/info/:id", auth.findUserInfoHTTP)
	protected.POST("/sign", auth.signHTTP)
	protected.GET("/sign/count", auth.signCountHTTP)
}

func (a *Service) sendCodeHTTP(c *gin.Context) {
	phone, found := c.GetQuery("phone")
	if !found {
		phone = c.PostForm("phone")
	}
	if !phonePattern.MatchString(phone) {
		c.JSON(http.StatusOK, httpapi.Result{ErrorMsg: "手机号格式错误"})
		return
	}
	if a.sender == nil {
		c.JSON(http.StatusServiceUnavailable, httpapi.Result{ErrorMsg: "短信网关未配置"})
		return
	}
	err := a.issueCode(c.Request.Context(), phone)
	if errors.Is(err, errCodePending) {
		c.JSON(http.StatusOK, httpapi.Result{ErrorMsg: "请稍后再发送验证码"})
		return
	}
	if err != nil {
		httpapi.ServerError(c)
		return
	}
	c.JSON(http.StatusOK, httpapi.Result{Success: true})
}

func (a *Service) loginHTTP(c *gin.Context) {
	var input struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, httpapi.Result{ErrorMsg: "invalid login body"})
		return
	}
	if !phonePattern.MatchString(input.Phone) {
		c.JSON(http.StatusOK, httpapi.Result{ErrorMsg: "手机号格式错误"})
		return
	}
	if !codePattern.MatchString(input.Code) {
		c.JSON(http.StatusOK, httpapi.Result{ErrorMsg: "验证码错误"})
		return
	}
	token, err := a.login(c.Request.Context(), input.Phone, input.Code)
	if errors.Is(err, errInvalidCode) {
		c.JSON(http.StatusOK, httpapi.Result{ErrorMsg: "验证码错误"})
		return
	}
	if err != nil {
		httpapi.ServerError(c)
		return
	}
	c.JSON(http.StatusOK, httpapi.Result{Success: true, Data: token})
}

func (a *Service) logoutHTTP(c *gin.Context) {
	token := c.MustGet(authTokenKey).(string)
	if err := a.logout(c.Request.Context(), token); err != nil {
		httpapi.ServerError(c)
		return
	}
	c.JSON(http.StatusOK, httpapi.Result{Success: true})
}

func userIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, httpapi.Result{ErrorMsg: "invalid user id"})
		return 0, false
	}
	return id, true
}

func (a *Service) findUserHTTP(c *gin.Context) {
	id, ok := userIDParam(c)
	if !ok {
		return
	}
	user, err := a.account(c.Request.Context(), id)
	if err != nil {
		httpapi.ServerError(c)
		return
	}
	result := httpapi.Result{Success: true}
	if user != nil {
		result.Data = user
	}
	c.JSON(http.StatusOK, result)
}

func (a *Service) findUserInfoHTTP(c *gin.Context) {
	id, ok := userIDParam(c)
	if !ok {
		return
	}
	info, err := a.profile(c.Request.Context(), id)
	if err != nil {
		httpapi.ServerError(c)
		return
	}
	result := httpapi.Result{Success: true}
	if info != nil {
		result.Data = info
	}
	c.JSON(http.StatusOK, result)
}

func (a *Service) signHTTP(c *gin.Context) {
	if err := a.sign(c.Request.Context(), Current(c).ID); err != nil {
		httpapi.ServerError(c)
		return
	}
	c.JSON(http.StatusOK, httpapi.Result{Success: true})
}

func (a *Service) signCountHTTP(c *gin.Context) {
	count, err := a.signCount(c.Request.Context(), Current(c).ID)
	if err != nil {
		httpapi.ServerError(c)
		return
	}
	c.JSON(http.StatusOK, httpapi.Result{Success: true, Data: count})
}
