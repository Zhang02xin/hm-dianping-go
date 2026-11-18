package shop

import (
	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type Module struct {
	details shopReader
	types   shopTypeReader
	search  shopSearchReader
	writes  shopWriter
	cache   *shopCache
}

func New(db *sql.DB, redisClient *redis.Client) *Module {
	source := sqlShopReader{db: db}
	m := &Module{details: source, types: sqlShopTypeReader{db: db}, search: geoShopSearch{sqlShopReader: source, cache: redisClient}, writes: source}
	if redisClient != nil {
		m.cache = newShopCache(redisClient)
		m.details = cachedShopDetails{cache: m.cache, source: m.details}
		m.types = cachedShopTypes{cache: m.cache, source: m.types}
		m.writes = cachedShopWriter{cache: m.cache, source: m.writes}
	}
	return m
}

func (m *Module) Close() {
	if m.cache != nil {
		m.cache.close()
	}
}

func (m *Module) Register(router *gin.Engine) {
	registerRoutes(router, m.details, m.types, m.search, m.writes)
}

func registerRoutes(router *gin.Engine, details shopReader, types shopTypeReader, search shopSearchReader, writes shopWriter) {
	router.GET("/shop/:id", shopDetailHandler(details))
	router.GET("/shop/of/name", findShopsByNameHandler(search))
	router.GET("/shop/of/type", findShopsByTypeHandler(search))
	router.POST("/shop", saveShopHandler(writes, false))
	router.PUT("/shop", saveShopHandler(writes, true))
	router.GET("/shop-type/list", listShopTypesHandler(types))
}
