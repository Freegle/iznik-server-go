package test

import (
	json2 "encoding/json"
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
	user, token := GetUserWithToken(t)

	// Get the logged in user.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var user2 user2.User
	json2.Unmarshal(rsp(resp), &user2)

	// Should match the user we tried to log in as.
	assert.Equal(t, user2.ID, user.ID)

	// Should see memberships.
	assert.Greater(t, len(user2.Memberships), 0)
}

func TestPersistent(t *testing.T) {
	// This is the old-style persistent token used by the PHP API.
	token := GetPersistentToken()

	// Get the logged in user.
	req := httptest.NewRequest("GET", "/api/user", nil)
	req.Header.Set("Authorization2", token)
	resp, _ := getApp().Test(req, 60000)
	assert.Equal(t, 200, resp.StatusCode)
	var user2 user2.User
	json2.Unmarshal(rsp(resp), &user2)
	assert.Greater(t, user2.ID, uint64(0))
}

func TestSearches(t *testing.T) {
	user, token := GetUserWithToken(t)

	// Get the logged in user.
	id := strconv.FormatUint(user.ID, 10)
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/"+id+"/search?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	id = strconv.FormatUint(0, 10)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/user/"+id+"/search?jwt="+token, nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestPublicLocation(t *testing.T) {
	user, token := GetUserWithToken(t)

	// Get the logged in user.
	id := strconv.FormatUint(user.ID, 10)
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/"+id+"/publiclocation?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var location user2.Publiclocation
	json2.Unmarshal(rsp(resp), &location)
	assert.Greater(t, len(location.Location), 0)
}

func TestExpiredJWT(t *testing.T) {
	user, _ := GetUserWithToken(t)
	id := strconv.FormatUint(user.ID, 10)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":  id,
		"exp": time.Date(2015, 10, 10, 12, 0, 0, 0, time.UTC).Unix(),
	})

	// Sign and get the complete encoded token as a string using the secret
	tokenString, _ := token.SignedString([]byte(os.Getenv("JWT_SECRET")))

	// Expired token is ignored.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/"+id+"/publiclocation?jwt="+tokenString, nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestValidJWTInvalidUser(t *testing.T) {
	user, _ := GetUserWithToken(t)
	id := strconv.FormatUint(user.ID+1, 10)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":  id,
		"exp": time.Date(2050, 10, 10, 12, 0, 0, 0, time.UTC).Unix(),
	})

	// Sign and get the complete encoded token as a string using the secret
	tokenString, _ := token.SignedString([]byte(os.Getenv("JWT_SECRET")))

	// Expired token is ignored.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/"+id+"/publiclocation?jwt="+tokenString, nil))
	assert.Equal(t, 401, resp.StatusCode)
}
