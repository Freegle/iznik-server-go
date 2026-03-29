package database

import (
	"fmt"
	sentrylogpackage "github.com/freegle/iznik-server-go/sentrylog"
	sql "github.com/rocketlaunchr/mysql-go"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"time"
)

var (
	DBConn *gorm.DB
	Pool   *sql.DB
)

func InitDatabase() {
	var err error
	var err2 error

	fmt.Println("Connecting to database", os.Getenv("MYSQL_HOST"), os.Getenv("MYSQL_PORT"), os.Getenv("MYSQL_DBNAME"), os.Getenv("MYSQL_USER"), os.Getenv("MYSQL_PROTOCOL"))

	mysqlCredentials := fmt.Sprintf(
		"%s:%s@%s(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local&interpolateParams=true",
		os.Getenv("MYSQL_USER"),
		os.Getenv("MYSQL_PASSWORD"),
		os.Getenv("MYSQL_PROTOCOL"),
		os.Getenv("MYSQL_HOST"),
		os.Getenv("MYSQL_PORT"),
		os.Getenv("MYSQL_DBNAME"),
	)

	newLogger := sentrylogpackage.New(
		sentrylogpackage.Config(logger.Config{
			SlowThreshold:             time.Second * 30,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true, // Can validly happen for us.
		}),
	)

	DBConn, err = gorm.Open(mysql.Open(mysqlCredentials), &gorm.Config{
		Logger: newLogger,
	})

	// This rocketlaunchr/mysql-go package allows genuine cancellation of the MySQL query, which doesn't happen in
	// the standard library. See https://medium.com/@rocketlaunchr.cloud/canceling-mysql-in-go-827ed8f83b30.
	//
	// We use this in cases where we want to be able to cancel long-running queries.
	Pool, err2 = sql.Open(mysqlCredentials)

	// Database-level retry is available via RetryQuery/RetryExec in retry.go.
	// API-level retry is handled by handler.WithRetry / handler.RetryGroup.
	if err != nil || err2 != nil {
		panic("failed to connect database")
	}

	// We want lots of connections for parallelisation, but must stay below
	// MySQL's max_connections (500 in percona-my.cnf).  Leave headroom for
	// other services (batch, tests) sharing the same MySQL instance.
	dbConfig, err3 := DBConn.DB()
	if err3 != nil {
		panic("failed to get database config: " + err3.Error())
	}
	dbConfig.SetMaxOpenConns(200)
	dbConfig.SetMaxIdleConns(200)
	dbConfig.SetConnMaxLifetime(time.Hour)
}
