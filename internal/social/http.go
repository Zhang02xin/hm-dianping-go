package social

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Zhang02xin/hm-dianping-go/internal/httpapi"
	"github.com/Zhang02xin/hm-dianping-go/internal/user"

	"github.com/gin-gonic/gin"
)

type handler struct{ repo socialData }

func (h handler) register(router *gin.Engine) {
	router.GET("/blog/hot", h.blogPageHTTP("hot"))
	blogs := router.Group("/blog", user.RequireLogin)
	blogs.POST("", h.createBlogHTTP)
	blogs.PUT("/like/:id", h.toggleBlogLikeHTTP)
	blogs.GET("/of/me", h.blogPageHTTP("me"))
	blogs.GET("/of/user", h.blogPageHTTP("user"))
	blogs.GET("/of/follow", h.feedHTTP)
	blogs.GET("/likes/:id", h.blogLikersHTTP)
	blogs.GET("/:id", h.findBlogHTTP)
	follows := router.Group("/follow", user.RequireLogin)
	follows.PUT("/:id/:isFollow", h.setFollowHTTP)
	follows.GET("/or/not/:id", h.isFollowingHTTP)
	follows.GET("/common/:id", h.commonFollowsHTTP)
}

func (h handler) createBlogHTTP(c *gin.Context) {
	var input blogInput
	if err := c.ShouldBindJSON(&input); err != nil || input.ShopID <= 0 || strings.TrimSpace(input.Title) == "" || input.Images == nil || input.Content == nil {
		c.JSON(http.StatusBadRequest, httpapi.Result{ErrorMsg: "博客参数无效"})
		return
	}
	id, err := h.repo.createBlog(c.Request.Context(), user.Current(c).ID, input)
	if err != nil {
		httpapi.ServerError(c)
		return
	}
	c.JSON(http.StatusOK, httpapi.Result{Success: true, Data: id})
}

func socialID(c *gin.Context, value string) (int64, bool) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, httpapi.Result{ErrorMsg: "ID无效"})
		return 0, false
	}
	return id, true
}

func (h handler) findBlogHTTP(c *gin.Context) {
	id, ok := socialID(c, c.Param("id"))
	if !ok {
		return
	}
	b, err := h.repo.findBlog(c.Request.Context(), id, user.Current(c).ID)
	if errors.Is(err, errBlogNotFound) {
		c.JSON(http.StatusOK, httpapi.Result{ErrorMsg: "博客不存在"})
		return
	}
	if err != nil {
		httpapi.ServerError(c)
		return
	}
	c.JSON(http.StatusOK, httpapi.Result{Success: true, Data: b})
}

func (h handler) blogPageHTTP(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset, ok := httpapi.PageOffset(c, blogPageSize)
		if !ok {
			return
		}
		var authorID, viewerID int64
		switch kind {
		case "hot":
			if value, exists := user.Optional(c); exists {
				viewerID = value.ID
			}
		case "me":
			authorID = user.Current(c).ID
		case "user":
			authorID, ok = socialID(c, c.Query("id"))
			if !ok {
				return
			}
		}
		items, err := h.repo.listBlogs(c.Request.Context(), authorID, viewerID, offset, kind == "hot")
		if err != nil {
			httpapi.ServerError(c)
			return
		}
		c.JSON(http.StatusOK, httpapi.Result{Success: true, Data: items})
	}
}

func (h handler) feedHTTP(c *gin.Context) {
	maxTime, timeErr := strconv.ParseInt(c.Query("lastId"), 10, 64)
	offsetText := c.Query("offset")
	if offsetText == "" {
		offsetText = "0"
	}
	offset, offsetErr := strconv.ParseInt(offsetText, 10, 32)
	if timeErr != nil || offsetErr != nil || maxTime < 0 || offset < 0 {
		c.JSON(http.StatusBadRequest, httpapi.Result{ErrorMsg: "动态游标无效"})
		return
	}
	page, err := readFeed(c.Request.Context(), h.repo, user.Current(c).ID, maxTime, offset)
	if err != nil {
		httpapi.ServerError(c)
		return
	}
	result := httpapi.Result{Success: true}
	if len(page.List) > 0 {
		result.Data = page
	}
	c.JSON(http.StatusOK, result)
}

func (h handler) toggleBlogLikeHTTP(c *gin.Context) {
	id, ok := socialID(c, c.Param("id"))
	if !ok {
		return
	}
	if err := h.repo.toggleBlogLike(c.Request.Context(), id, user.Current(c).ID); err != nil {
		httpapi.ServerError(c)
		return
	}
	c.JSON(http.StatusOK, httpapi.Result{Success: true})
}

func (h handler) blogLikersHTTP(c *gin.Context) {
	id, ok := socialID(c, c.Param("id"))
	if !ok {
		return
	}
	users, err := h.repo.blogLikers(c.Request.Context(), id)
	if err != nil {
		httpapi.ServerError(c)
		return
	}
	c.JSON(http.StatusOK, httpapi.Result{Success: true, Data: users})
}

func (h handler) setFollowHTTP(c *gin.Context) {
	id, ok := socialID(c, c.Param("id"))
	if !ok {
		return
	}
	follow, err := strconv.ParseBool(c.Param("isFollow"))
	if err != nil {
		c.JSON(http.StatusBadRequest, httpapi.Result{ErrorMsg: "关注状态无效"})
		return
	}
	if err := h.repo.setFollow(c.Request.Context(), user.Current(c).ID, id, follow); err != nil {
		httpapi.ServerError(c)
		return
	}
	c.JSON(http.StatusOK, httpapi.Result{Success: true})
}

func (h handler) isFollowingHTTP(c *gin.Context) {
	id, ok := socialID(c, c.Param("id"))
	if !ok {
		return
	}
	following, err := h.repo.isFollowing(c.Request.Context(), user.Current(c).ID, id)
	if err != nil {
		httpapi.ServerError(c)
		return
	}
	c.JSON(http.StatusOK, httpapi.Result{Success: true, Data: following})
}

func (h handler) commonFollowsHTTP(c *gin.Context) {
	id, ok := socialID(c, c.Param("id"))
	if !ok {
		return
	}
	users, err := h.repo.commonFollows(c.Request.Context(), user.Current(c).ID, id)
	if err != nil {
		httpapi.ServerError(c)
		return
	}
	c.JSON(http.StatusOK, httpapi.Result{Success: true, Data: users})
}
