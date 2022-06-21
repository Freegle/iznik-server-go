package test

import (
	"github.com/freegle/iznik-server-go/database"
	"os"
	"testing"
)

func TestFail(t *testing.T) {
	// Change the env far so that the connection will fail; should panic.
	user :=
		os.Getenv("MYSQL_USER")

	defer func() {
		os.Setenv("MYSQL_USER", user)
		recover()
	}()

	database.InitDatabase()
}
