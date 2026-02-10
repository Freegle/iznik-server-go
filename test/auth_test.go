package test

import (
	json2 "encoding/json"
	"github.com/freegle/iznik-server-go/database"
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
	// Get max id in users table and add 1 to make it invalid
	uid := uint64(0)
	db := database.DBConn
	db.Raw("SELECT MAX(id) + 1 FROM users").Scan(&uid)

	idstr := strconv.FormatUint(uid, 10)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":        idstr,
		"sessionid": "1234",
		"exp":       time.Date(2050, 10, 10, 12, 0, 0, 0, time.UTC).Unix(),
	})

	// Sign and get the complete encoded token as a string using the secret
	tokenString, _ := token.SignedString([]byte(os.Getenv("JWT_SECRET")))

	// Invalid user returns 401
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/job?lat=52.5833189&lng=-2.0455619&jwt="+tokenString, nil))
	assert.Equal(t, 401, resp.StatusCode)
}
