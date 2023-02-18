package database

import (
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"os"
	"time"
)

var (
	DBConn *gorm.DB
)

func InitDatabase() {
	var err error
	mysqlCredentials := fmt.Sprintf(
		"%s:%s@%s(%s:%s)/%s?charset=utf8&parseTime=True&loc=Local&interpolateParams=true",
		os.Getenv("MYSQL_USER"),
		os.Getenv("MYSQL_PASSWORD"),
		os.Getenv("MYSQL_PROTOCOL"),
		os.Getenv("MYSQL_HOST"),
		os.Getenv("MYSQL_PORT"),
		os.Getenv("MYSQL_DBNAME"),
	)

	DBConn, err = gorm.Open(mysql.Open(mysqlCredentials))

	// We don't have any retrying of DB errors, such as may happen if a cluster member misbehaves.  We expect the
	// client to handle any retries required.
	if err != nil {
		panic("failed to connect database")
	}

	// We want lots of connections for parallelisation.
	dbConfig, _ := DBConn.DB()
	dbConfig.SetMaxIdleConns(1000)
	dbConfig.SetConnMaxLifetime(time.Hour)
}
