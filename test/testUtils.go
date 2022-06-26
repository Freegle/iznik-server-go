package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/group"
	"github.com/freegle/iznik-server-go/router"
	user2 "github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"io"
	"net/http"
	"net/http/httptest"
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

	// Find a user with:
	// - an isochrone
	// - a chat
	var user user2.User
	start := time.Now().AddDate(0, 0, -utils.CHAT_ACTIVE_LIMIT).Format("2006-01-02")

	db.Raw("SELECT users.* FROM users "+
		"INNER JOIN isochrones_users ON isochrones_users.userid = users.id "+
		"INNER JOIN chat_messages ON chat_messages.userid = users.id "+
		"INNER JOIN chat_rooms c1 ON c1.user1 = users.id AND c1.chattype = ? AND c1.latestmessage > ? "+
		"INNER JOIN chat_rooms c2 ON c2.user1 = users.id AND c2.chattype = ? AND c2.latestmessage > ?  "+
		"LIMIT 1", utils.CHAT_TYPE_USER2USER, start, utils.CHAT_TYPE_USER2MOD, start).Scan(&user)

	// Get their JWT. This matches the PHP code.
	token := GetToken(user.ID)

	return user, token
}

func GetGroup(name string) group.GroupEntry {
	app := fiber.New()
	database.InitDatabase()
	router.SetupRoutes(app)

	resp, _ := app.Test(httptest.NewRequest("GET", "/api/group", nil))

	var groups []group.GroupEntry
	json2.Unmarshal(rsp(resp), &groups)

	// Get the playground
	gix := 0

	for ix, g := range groups {
		if g.Nameshort == name {
			gix = ix
		}
	}

	return groups[gix]
}
