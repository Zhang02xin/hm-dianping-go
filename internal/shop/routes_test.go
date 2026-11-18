package shop

import (
	"github.com/gin-gonic/gin"
)

func testShopRouter(details shopReader, types shopTypeReader, search shopSearchReader, writes shopWriter) *gin.Engine {
	router := gin.New()
	registerRoutes(router, details, types, search, writes)
	return router
}
