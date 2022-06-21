package test

import (
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	user2 "github.com/freegle/iznik-server-go/user"
	"github.com/golang-jwt/jwt/v4"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func rsp(response *http.Response) []byte {
	buf := new(strings.Builder)
	io.Copy(buf, response.Body)
	//fmt.Println(buf.String())
	return []byte(buf.String())
}

func GetToken(id uint64) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":  fmt.Sprint(id),
		"exp": time.Now().Unix() + 30*60,
	})

	// Sign and get the complete encoded token as a string using the secret
	tokenString, _ := token.SignedString([]byte(os.Getenv("JWT_SECRET")))

	return tokenString
}

func GetUserWithToken() (user2.User, string) {
	db := database.DBConn
	// Find a user.
	var user user2.User
	db.First(&user)

	// Get their JWT. This matches the PHP code.
	token := GetToken(user.ID)

	return user, token
}
