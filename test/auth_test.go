package test

import (
	json2 "encoding/json"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/auth"
	user2 "github.com/freegle/iznik-server-go/user"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestAuth(t *testing.T) {
	// Create a full test user with all relationships
	prefix := uniquePrefix("auth")
	userID, token := CreateFullTestUser(t, prefix)

	// Get the logged in user - use 60s timeout since /api/user is a complex endpoint
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user?jwt="+token, nil), 60000)
	assert.Equal(t, 200, resp.StatusCode)
	var user user2.User
	json2.Unmarshal(rsp(resp), &user)

	// Should match the user we tried to log in as
	assert.Equal(t, user.ID, userID)

	// Should see memberships
	assert.Greater(t, len(user.Memberships), 0)
}

func TestPersistent(t *testing.T) {
	// Create a user and session for this test
	prefix := uniquePrefix("persistent")
	userID := CreateTestUser(t, prefix, "User")
	sessionID, _ := CreateTestSession(t, userID)

	// Create the old-style persistent token used by the PHP API
	token := CreatePersistentToken(t, userID, sessionID)

	// Get the logged in user
	req := httptest.NewRequest("GET", "/api/user", nil)
	req.Header.Set("Authorization2", token)
	resp, _ := getApp().Test(req, 60000)
	assert.Equal(t, 200, resp.StatusCode)
	var user user2.User
	json2.Unmarshal(rsp(resp), &user)
	assert.Equal(t, user.ID, userID)
}

func TestSearches(t *testing.T) {
	// Create a full test user
	prefix := uniquePrefix("searches")
	userID, token := CreateFullTestUser(t, prefix)

	// Get the logged in user's searches
	id := strconv.FormatUint(userID, 10)
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/"+id+"/search?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Non-existent user should return 404
	id = strconv.FormatUint(0, 10)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/user/"+id+"/search?jwt="+token, nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestPublicLocation(t *testing.T) {
	// Create a full test user with location
	prefix := uniquePrefix("publoc")
	userID, token := CreateFullTestUser(t, prefix)

	// Get the user's public location
	id := strconv.FormatUint(userID, 10)
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/"+id+"/publiclocation?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var location user2.Publiclocation
	json2.Unmarshal(rsp(resp), &location)
	assert.Greater(t, len(location.Location), 0)
}

func TestExpiredJWT(t *testing.T) {
	// Create a user for this test
	prefix := uniquePrefix("expired")
	userID, _ := CreateFullTestUser(t, prefix)
	id := strconv.FormatUint(userID, 10)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":  id,
		"exp": time.Date(2015, 10, 10, 12, 0, 0, 0, time.UTC).Unix(),
	})

	// Sign and get the complete encoded token as a string using the secret
	tokenString, _ := token.SignedString([]byte(os.Getenv("JWT_SECRET")))

	// Expired token is ignored
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/"+id+"/publiclocation?jwt="+tokenString, nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestValidJWTInvalidUser(t *testing.T) {
	// Create a real user and token, then delete the user so the JWT points
	// to a non-existent user. The middleware's post-check verifies the
	// user+session still exists in DB and returns 401 when it doesn't.
	uid := CreateTestUser(t, uniquePrefix("invaliduser"), "User")
	token := getToken(t, uid)

	db := database.DBConn
	db.Exec("DELETE FROM users WHERE id = ?", uid)

	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, nil)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestHasPermission(t *testing.T) {
	prefix := uniquePrefix("hasperm")
	db := database.DBConn

	// User with no permissions
	userID := CreateTestUser(t, prefix+"_none", "User")
	assert.False(t, auth.HasPermission(userID, auth.PERM_GIFTAID))

	// User with GiftAid permission
	userGiftAid := CreateTestUser(t, prefix+"_ga", "User")
	db.Exec("UPDATE users SET permissions = 'GiftAid' WHERE id = ?", userGiftAid)
	assert.True(t, auth.HasPermission(userGiftAid, auth.PERM_GIFTAID))
	assert.False(t, auth.HasPermission(userGiftAid, auth.PERM_NEWSLETTER))

	// User with multiple permissions
	userMulti := CreateTestUser(t, prefix+"_multi", "User")
	db.Exec("UPDATE users SET permissions = 'Newsletter,GiftAid,SpamAdmin' WHERE id = ?", userMulti)
	assert.True(t, auth.HasPermission(userMulti, auth.PERM_GIFTAID))
	assert.True(t, auth.HasPermission(userMulti, auth.PERM_NEWSLETTER))
	assert.True(t, auth.HasPermission(userMulti, auth.PERM_SPAM_ADMIN))
	assert.False(t, auth.HasPermission(userMulti, auth.PERM_TEAMS))
}
