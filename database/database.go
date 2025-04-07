package database

// This rocketlaunchr/mysql-go package allows genuine cancellation of the MySQL query, which doesn't happen in
// the standard library. See https://medium.com/@rocketlaunchr.cloud/canceling-mysql-in-go-827ed8f83b30.
//
// We use this in cases where we want to be able to cancel long-running queries.

import (
	"fmt"
	sentrylogpackage "github.com/freegle/iznik-server-go/sentrylog"
	sql "github.com/rocketlaunchr/mysql-go"
	"golang.org/x/crypto/ssh"
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

	newLogger := sentrylogpackage.New(
		sentrylogpackage.Config(logger.Config{
			SlowThreshold:             time.Second * 30,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true, // Can validly happen for us.
		}),
	)

	privateKey := os.Getenv("MYSQL_PRIVATE_KEY")
	publicKey := os.Getenv("MYSQL_PUBLIC_KEY")
	sqlHost := os.Getenv("MYSQL_HOST")

	privateKeyBytes := []byte(privateKey)
	publicKeyBytes := []byte(publicKey)

	if privateKey != "" && publicKey != "" {
		fmt.Println("Using private key and public key")

		// Create the Signer for this private key.
		signer, err := ssh.ParsePrivateKey(privateKeyBytes)
		if err != nil {
			panic(fmt.Sprintf("unable to parse private key: %v", err))
		}

		pk, _, _, _, err := ssh.ParseAuthorizedKey(publicKeyBytes)
		if err != nil {
			panic(fmt.Sprintf("unable to parse public key: %v", err))
		}

		certSigner, err := ssh.NewCertSigner(pk.(*ssh.Certificate), signer)
		if err != nil {
			panic(fmt.Sprintf("failed to create cert signer: %v", err))
		}

		config := &ssh.ClientConfig{
			User: "user",
			Auth: []ssh.AuthMethod{
				// Use the PublicKeys method for remote authentication.
				ssh.PublicKeys(certSigner),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}

		// Connect to the remote server and perform the SSH handshake.
		client, err := ssh.Dial("tcp", sqlHost+":22", config)
		if err != nil {
			panic(fmt.Sprintf("unable to connect: %v", err))
		}
		defer client.Close()
	} else {
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

		DBConn, err = gorm.Open(mysql.Open(mysqlCredentials), &gorm.Config{
			Logger: newLogger,
		})

		Pool, err2 = sql.Open(mysqlCredentials)
	}

	// We don't have any retrying of DB errors, such as may happen if a cluster member misbehaves.  We expect the
	// client to handle any retries required.
	if err != nil || err2 != nil {
		panic("failed to connect database")
	}

	// We want lots of connections for parallelisation.
	dbConfig, _ := DBConn.DB()
	dbConfig.SetMaxIdleConns(1000)
	dbConfig.SetConnMaxLifetime(time.Hour)
}
