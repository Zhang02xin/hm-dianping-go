package upload

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/Zhang02xin/hm-dianping-go/internal/httpapi"
	"github.com/Zhang02xin/hm-dianping-go/internal/user"

	"github.com/gin-gonic/gin"
)

const maxBlogImage = 5 << 20

func Register(router *gin.Engine, root *os.Root) {
	images := blogImages{root: root}
	configured := func(c *gin.Context) {
		if images.root == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, httpapi.Result{ErrorMsg: "图片存储未配置"})
		}
	}
	router.POST("/upload/blog", user.RequireLogin, configured, images.upload)
	router.GET("/upload/blog/delete", user.RequireLogin, configured, images.delete)
	router.GET("/imgs/blogs/*name", configured, images.serve)
}

func (s blogImages) upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBlogImage+(64<<10))
	file, _, err := c.Request.FormFile("file")
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, httpapi.Result{ErrorMsg: "图片超过大小限制"})
		} else {
			c.JSON(http.StatusBadRequest, httpapi.Result{ErrorMsg: "缺少图片文件"})
		}
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBlogImage+1))
	if err != nil {
		httpapi.ServerError(c)
		return
	}
	if len(data) > maxBlogImage {
		c.JSON(http.StatusRequestEntityTooLarge, httpapi.Result{ErrorMsg: "图片超过大小限制"})
		return
	}
	ext, ok := map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/gif": "gif", "image/webp": "webp"}[http.DetectContentType(data)]
	if !ok {
		c.JSON(http.StatusBadRequest, httpapi.Result{ErrorMsg: "不支持的图片格式"})
		return
	}
	name, err := s.save(data, ext, user.Current(c).ID)
	if err != nil {
		httpapi.ServerError(c)
		return
	}
	c.JSON(http.StatusOK, httpapi.Result{Success: true, Data: name})
}

func (s blogImages) delete(c *gin.Context) {
	err := s.remove(c.Query("name"), user.Current(c).ID)
	switch {
	case errors.Is(err, errInvalidName):
		c.JSON(http.StatusOK, httpapi.Result{ErrorMsg: "错误的文件名称"})
	case errors.Is(err, errNotOwner):
		c.JSON(http.StatusForbidden, httpapi.Result{ErrorMsg: "无权删除图片"})
	case err != nil:
		httpapi.ServerError(c)
	default:
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, httpapi.Result{Success: true})
	}
}

func (s blogImages) serve(c *gin.Context) {
	name := "/blogs" + c.Param("name")
	file, info, err := s.open(name)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer file.Close()
	c.Header("X-Content-Type-Options", "nosniff")
	http.ServeContent(c.Writer, c.Request, path.Base(name), info.ModTime(), file)
}
