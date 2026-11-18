package social

import (
	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func Register(router *gin.Engine, db *sql.DB, cache *redis.Client) {
	handler{repo: socialData{sqlSocial: sqlSocial{db: db}, cache: cache}}.register(router)
}
