package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/Zhang02xin/hm-dianping-go/internal/shop"
	"github.com/Zhang02xin/hm-dianping-go/internal/social"
	"github.com/Zhang02xin/hm-dianping-go/internal/upload"
	"github.com/Zhang02xin/hm-dianping-go/internal/user"
	"github.com/Zhang02xin/hm-dianping-go/internal/voucher"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

type application struct {
	router   *gin.Engine
	shops    *shop.Module
	vouchers *voucher.Module
}

func newApplication(db *sql.DB, loc *time.Location, client *redis.Client, sender user.CodeSender, uploadRoot *os.Root) *application {
	shops := shop.New(db, client)
	vouchers := voucher.New(db, loc, client)
	users := user.New(db, client, sender)

	router := gin.New()
	router.Use(gin.LoggerWithConfig(gin.LoggerConfig{SkipQueryString: true}), gin.Recovery())
	if users != nil {
		router.Use(users.RefreshSession)
	}
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})
	user.Register(router, users)
	shops.Register(router)
	vouchers.Register(router)
	social.Register(router, db, client)
	upload.Register(router, uploadRoot)
	return &application{router: router, shops: shops, vouchers: vouchers}
}

func Run() error {
	db, loc, err := openMySQL(os.Getenv("DIANPING_MYSQL_DSN"))
	if err != nil {
		return err
	}
	defer db.Close()

	client, err := openRedis(os.Getenv("DIANPING_REDIS_URL"))
	if err != nil {
		return err
	}
	if client != nil {
		defer client.Close()
	}

	var sender user.CodeSender
	if endpoint := os.Getenv("DIANPING_SMS_ENDPOINT"); endpoint != "" {
		if client == nil {
			return errors.New("SMS login requires MySQL and Redis configuration")
		}
		sender, err = user.NewCodeSender(endpoint, nil)
		if err != nil {
			return err
		}
	}

	var uploadRoot *os.Root
	if dir := os.Getenv("DIANPING_UPLOAD_DIR"); dir != "" {
		uploadRoot, err = os.OpenRoot(dir)
		if err != nil {
			return errors.New("cannot open DIANPING_UPLOAD_DIR")
		}
		defer uploadRoot.Close()
	}

	app := newApplication(db, loc, client, sender, uploadRoot)
	defer app.shops.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	stopWorker, err := app.vouchers.StartWorker(ctx)
	cancel()
	if err != nil {
		return err
	}
	defer stopWorker()

	addr := os.Getenv("DIANPING_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	return serveHTTP(addr, app.router)
}

func openMySQL(dsn string) (*sql.DB, *time.Location, error) {
	if dsn == "" {
		return nil, nil, errors.New("DIANPING_MYSQL_DSN is required")
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, nil, errors.New("invalid DIANPING_MYSQL_DSN configuration")
	}
	config.ParseTime = true
	connector, err := mysql.NewConnector(config)
	if err != nil {
		return nil, nil, errors.New("invalid MySQL connector configuration")
	}
	db := sql.OpenDB(connector)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, nil, errors.New("MySQL connection check failed")
	}
	return db, config.Loc, nil
}

func openRedis(url string) (*redis.Client, error) {
	if url == "" {
		return nil, nil
	}
	options, err := redis.ParseURL(url)
	if err != nil {
		return nil, errors.New("invalid DIANPING_REDIS_URL configuration")
	}
	options.ContextTimeoutEnabled = true
	client := redis.NewClient(options)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, errors.New("Redis connection check failed")
	}
	return client, nil
}
