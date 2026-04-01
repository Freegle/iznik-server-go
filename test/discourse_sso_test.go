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

const testDiscourseSecret = "test_discourse_secret_123"

func makeDiscourseSSO(nonce string, secret string) (string, string) {
	payload := "nonce=" + url.QueryEscape(nonce)
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(encoded))
	sig := hex.EncodeToString(mac.Sum(nil))

	return encoded, sig
}

func TestDiscourseSSO_ValidFlow(t *testing.T) {
	prefix := uniquePrefix("discoursesso")
	db := database.DBConn

	os.Setenv("DISCOURSE_SECRET", testDiscourseSecret)
	defer os.Unsetenv("DISCOURSE_SECRET")

	// Create a moderator user on a Freegle group.
	userID := CreateTestUser(t, prefix+"_mod", "Moderator")
	email := prefix + "_mod@test.com"
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Moderator")

	// Create a session.
	sessionID, _ := CreateTestSession(t, userID)

	// Get session details for cookie.
	var series uint64
	var token string
	db.Raw("SELECT series, token FROM sessions WHERE id = ?", sessionID).Row().Scan(&series, &token)

	cookieData, _ := json.Marshal(map[string]interface{}{
		"id":     sessionID,
		"series": series,
		"token":  token,
	})

	// Build SSO request.
	ssoPayload, sig := makeDiscourseSSO("test_nonce_"+prefix, testDiscourseSecret)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/discourse_sso?sso=%s&sig=%s",
		url.QueryEscape(ssoPayload), url.QueryEscape(sig)), nil)
	req.Header.Set("Cookie", "Iznik-Discourse-SSO="+url.QueryEscape(string(cookieData)))

	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)

	location := resp.Header.Get("Location")
	assert.Contains(t, location, "discourse.ilovefreegle.org/session/sso_login",
		"Should redirect to Discourse SSO login")
	assert.Contains(t, location, "sso=", "Should have sso parameter")
	assert.Contains(t, location, "sig=", "Should have sig parameter")

	// Verify the response payload contains the user's email.
	parsedURL, err := url.Parse(location)
	assert.NoError(t, err)
	ssoResp := parsedURL.Query().Get("sso")
	decoded, err := base64.StdEncoding.DecodeString(ssoResp)
	assert.NoError(t, err)
	values, err := url.ParseQuery(string(decoded))
	assert.NoError(t, err)
	assert.Equal(t, email, values.Get("email"))
	assert.Equal(t, fmt.Sprint(userID), values.Get("external_id"))
}

func TestDiscourseSSO_InvalidSignature(t *testing.T) {
	os.Setenv("DISCOURSE_SECRET", testDiscourseSecret)
	defer os.Unsetenv("DISCOURSE_SECRET")

	ssoPayload, _ := makeDiscourseSSO("test_nonce", testDiscourseSecret)

	// Use a wrong signature.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/discourse_sso?sso=%s&sig=invalidsig",
		url.QueryEscape(ssoPayload)), nil)

	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestDiscourseSSO_MissingCookie(t *testing.T) {
	os.Setenv("DISCOURSE_SECRET", testDiscourseSecret)
	defer os.Unsetenv("DISCOURSE_SECRET")

	ssoPayload, sig := makeDiscourseSSO("test_nonce", testDiscourseSecret)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/discourse_sso?sso=%s&sig=%s",
		url.QueryEscape(ssoPayload), url.QueryEscape(sig)), nil)

	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)

	location := resp.Header.Get("Location")
	assert.Contains(t, location, "modtools.org/discourse",
		"Should redirect to modtools login when no cookie")
}
