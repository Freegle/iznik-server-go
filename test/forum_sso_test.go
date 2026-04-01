package test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

const testForumSecret = "test_forum_secret_456"

func makeForumSSO(nonce string, secret string) (string, string) {
	payload := "nonce=" + url.QueryEscape(nonce)
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(encoded))
	sig := hex.EncodeToString(mac.Sum(nil))

	return encoded, sig
}

func TestForumSSO_ValidFlow(t *testing.T) {
	prefix := uniquePrefix("forumssoval")
	db := database.DBConn

	os.Setenv("FORUM_SECRET", testForumSecret)
	defer os.Unsetenv("FORUM_SECRET")

	// Create a regular user (not a mod).
	userID := CreateTestUser(t, prefix+"_user", "User")
	email := prefix + "_user@test.com"

	// Create a session.
	sessionID, _ := CreateTestSession(t, userID)

	var series uint64
	var token string
	db.Raw("SELECT series, token FROM sessions WHERE id = ?", sessionID).Row().Scan(&series, &token)

	cookieData, _ := json.Marshal(map[string]interface{}{
		"id":     sessionID,
		"series": series,
		"token":  token,
	})

	ssoPayload, sig := makeForumSSO("test_nonce_"+prefix, testForumSecret)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/forum_sso?sso=%s&sig=%s",
		url.QueryEscape(ssoPayload), url.QueryEscape(sig)), nil)
	req.Header.Set("Cookie", "Iznik-Forum-SSO="+url.QueryEscape(string(cookieData)))

	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)

	location := resp.Header.Get("Location")
	assert.Contains(t, location, "forum.ilovefreegle.org/session/sso_login",
		"Should redirect to Forum SSO login")

	// Verify the response payload.
	parsedURL, err := url.Parse(location)
	assert.NoError(t, err)
	ssoResp := parsedURL.Query().Get("sso")
	decoded, err := base64.StdEncoding.DecodeString(ssoResp)
	assert.NoError(t, err)
	values, err := url.ParseQuery(string(decoded))
	assert.NoError(t, err)
	assert.Equal(t, email, values.Get("email"))
	assert.Equal(t, fmt.Sprint(userID), values.Get("external_id"))

	// Regular user bio should say "Member on".
	assert.Contains(t, values.Get("bio"), "Member on")
}

func TestForumSSO_NonModAllowed(t *testing.T) {
	prefix := uniquePrefix("forumssomod")
	db := database.DBConn

	os.Setenv("FORUM_SECRET", testForumSecret)
	defer os.Unsetenv("FORUM_SECRET")

	// Create a regular user — should still be allowed (unlike Discourse SSO).
	userID := CreateTestUser(t, prefix+"_regular", "User")

	sessionID, _ := CreateTestSession(t, userID)

	var series uint64
	var token string
	db.Raw("SELECT series, token FROM sessions WHERE id = ?", sessionID).Row().Scan(&series, &token)

	cookieData, _ := json.Marshal(map[string]interface{}{
		"id":     sessionID,
		"series": series,
		"token":  token,
	})

	ssoPayload, sig := makeForumSSO("test_nonce_"+prefix, testForumSecret)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/forum_sso?sso=%s&sig=%s",
		url.QueryEscape(ssoPayload), url.QueryEscape(sig)), nil)
	req.Header.Set("Cookie", "Iznik-Forum-SSO="+url.QueryEscape(string(cookieData)))

	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode, "Non-mod users should be allowed for Forum SSO")

	location := resp.Header.Get("Location")
	assert.Contains(t, location, "forum.ilovefreegle.org/session/sso_login")
}

func TestForumSSO_MissingCookie(t *testing.T) {
	os.Setenv("FORUM_SECRET", testForumSecret)
	defer os.Unsetenv("FORUM_SECRET")

	ssoPayload, sig := makeForumSSO("test_nonce", testForumSecret)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/forum_sso?sso=%s&sig=%s",
		url.QueryEscape(ssoPayload), url.QueryEscape(sig)), nil)

	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)

	location := resp.Header.Get("Location")
	assert.Contains(t, location, "ilovefreegle.org/forum",
		"Should redirect to forum login when no cookie")
}
