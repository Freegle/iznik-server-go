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
	"github.com/stretchr/testify/assert"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func rsp(response *http.Response) []byte {
	buf := new(strings.Builder)
	io.Copy(buf, response.Body)
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

func GetUserWithToken(t *testing.T) (user2.User, string) {
	db := database.DBConn

	// Find a user with:
	// - an isochrone
	// - an address
	// - a user chat
	// - a mod chat
	// - a group membership
	//
	// This should have been set up in testenv.php.
	var user user2.User
	start := time.Now().AddDate(0, 0, -utils.CHAT_ACTIVE_LIMIT).Format("2006-01-02")

	db.Raw("SELECT users.* FROM users "+
		"INNER JOIN isochrones_users ON isochrones_users.userid = users.id "+
		"INNER JOIN chat_messages ON chat_messages.userid = users.id AND chat_messages.message IS NOT NULL "+
		"INNER JOIN chat_rooms c1 ON c1.user1 = users.id AND c1.chattype = ? AND c1.latestmessage > ? "+
		"INNER JOIN chat_rooms c2 ON c2.user1 = users.id AND c2.chattype = ? AND c2.latestmessage > ? "+
		"INNER JOIN users_addresses ON users_addresses.userid = users.id "+
		"INNER JOIN memberships ON memberships.userid = users.id "+
		"LIMIT 1", utils.CHAT_TYPE_USER2USER, start, utils.CHAT_TYPE_USER2MOD, start).Scan(&user)

	// Get their JWT. This matches the PHP code.
	assert.Greater(t, user.ID, uint64(0))
	token := GetToken(user.ID)
	assert.Greater(t, len(token), 0)

	return user, token
}

func GetPersistentToken() string {
	db := database.DBConn

	var t user2.PersistentToken

	db.Raw("SELECT id, series, token FROM sessions ORDER BY lastactive DESC LIMIT 1").Scan(&t)

	enc, _ := json2.Marshal(t)

	return string(enc)
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

func GetUserWithMessage(t *testing.T) uint64 {
	db := database.DBConn

	type users struct {
		Fromuser uint64
	}

	var u []users

	db.Raw("SELECT fromuser FROM messages_groups INNER JOIN messages ON messages.id = messages_groups.msgid WHERE fromuser IS NOT NULL AND fromuser > 0 ORDER BY messages.id DESC LIMIT 1").Scan(&u)

	return u[0].Fromuser
}
