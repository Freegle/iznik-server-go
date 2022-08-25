package database

import (
	"fmt"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
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

	DBConn, err = gorm.Open("mysql", mysqlCredentials)

	// We want lots of connections for parallelisation.
	DBConn.DB().SetMaxIdleConns(1000)
	DBConn.DB().SetConnMaxLifetime(time.Hour)

	if err != nil {
		panic("failed to connect database")
	}
}
