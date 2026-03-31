package test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// Google Login Tests
// =============================================================================

func newGoogleMockServer(sub, email, givenName, familyName, name, aud string, statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if statusCode != 200 {
			w.WriteHeader(statusCode)
			w.Write([]byte(`{"error": "invalid_token"}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]string{
			"sub":         sub,
			"email":       email,
			"given_name":  givenName,
			"family_name": familyName,
			"name":        name,
			"aud":         aud,
		})
	}))
}

func TestGoogleLoginNewUser(t *testing.T) {
	prefix := uniquePrefix("google-new")
	email := fmt.Sprintf("%s@gmail.com", prefix)
	clientID := "test-google-client-id"

	server := newGoogleMockServer("google-uid-"+prefix, email, "Google", "User", "Google User", clientID, 200)
	defer server.Close()

	os.Setenv("GOOGLE_TOKENINFO_URL", server.URL)
	os.Setenv("GOOGLE_CLIENT_ID", clientID)
	defer os.Unsetenv("GOOGLE_TOKENINFO_URL")
	defer os.Unsetenv("GOOGLE_CLIENT_ID")

	body := `{"googlelogin":true,"googlejwt":"fake-jwt-token"}`
	resp := postSession(body)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])
	assert.NotNil(t, result["jwt"])
	assert.NotNil(t, result["persistent"])

	// Verify user was created with the login record.
	db := database.DBConn
	var userID uint64
	db.Raw("SELECT userid FROM users_logins WHERE type = 'Google' AND uid = ?", "google-uid-"+prefix).Scan(&userID)
	assert.NotEqual(t, uint64(0), userID)

	// Cleanup.
	db.Exec("DELETE FROM users_logins WHERE userid = ?", userID)
	db.Exec("DELETE FROM users_emails WHERE userid = ?", userID)
	db.Exec("DELETE FROM sessions WHERE userid = ?", userID)
	db.Exec("DELETE FROM users WHERE id = ?", userID)
}

func TestGoogleLoginExistingUserByEmail(t *testing.T) {
	prefix := uniquePrefix("google-email")
	email := fmt.Sprintf("%s@test.com", prefix)
	userID := CreateTestUser(t, prefix, "User")
	clientID := "test-google-client-id"

	server := newGoogleMockServer("google-uid-"+prefix, email, "Test", prefix, "Test "+prefix, clientID, 200)
	defer server.Close()

	os.Setenv("GOOGLE_TOKENINFO_URL", server.URL)
	os.Setenv("GOOGLE_CLIENT_ID", clientID)
	defer os.Unsetenv("GOOGLE_TOKENINFO_URL")
	defer os.Unsetenv("GOOGLE_CLIENT_ID")

	body := `{"googlelogin":true,"googlejwt":"fake-jwt-token"}`
	resp := postSession(body)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])

	// Verify the Google login was linked to the existing user.
	db := database.DBConn
	var loginUserID uint64
	db.Raw("SELECT userid FROM users_logins WHERE type = 'Google' AND uid = ?", "google-uid-"+prefix).Scan(&loginUserID)
	assert.Equal(t, userID, loginUserID)

	// Cleanup.
	db.Exec("DELETE FROM users_logins WHERE userid = ? AND type = 'Google'", userID)
	db.Exec("DELETE FROM sessions WHERE userid = ?", userID)
}

func TestGoogleLoginExistingUserByGoogleUID(t *testing.T) {
	prefix := uniquePrefix("google-uid")
	userID := CreateTestUser(t, prefix, "User")
	clientID := "test-google-client-id"
	uid := "google-uid-" + prefix

	// Pre-create the Google login record.
	db := database.DBConn
	db.Exec("INSERT INTO users_logins (userid, type, uid) VALUES (?, 'Google', ?)", userID, uid)

	server := newGoogleMockServer(uid, "other-email@gmail.com", "Test", prefix, "Test "+prefix, clientID, 200)
	defer server.Close()

	os.Setenv("GOOGLE_TOKENINFO_URL", server.URL)
	os.Setenv("GOOGLE_CLIENT_ID", clientID)
	defer os.Unsetenv("GOOGLE_TOKENINFO_URL")
	defer os.Unsetenv("GOOGLE_CLIENT_ID")

	body := `{"googlelogin":true,"googlejwt":"fake-jwt-token"}`
	resp := postSession(body)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])

	// Cleanup.
	db.Exec("DELETE FROM users_logins WHERE userid = ? AND type = 'Google'", userID)
	db.Exec("DELETE FROM sessions WHERE userid = ?", userID)
}

func TestGoogleLoginInvalidJWT(t *testing.T) {
	clientID := "test-google-client-id"

	server := newGoogleMockServer("", "", "", "", "", "", 400)
	defer server.Close()

	os.Setenv("GOOGLE_TOKENINFO_URL", server.URL)
	os.Setenv("GOOGLE_CLIENT_ID", clientID)
	defer os.Unsetenv("GOOGLE_TOKENINFO_URL")
	defer os.Unsetenv("GOOGLE_CLIENT_ID")

	body := `{"googlelogin":true,"googlejwt":"invalid-token"}`
	resp := postSession(body)
	assert.Equal(t, 401, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestGoogleLoginMissingJWT(t *testing.T) {
	// googlelogin=true but no googlejwt should fall through to "Invalid request body".
	body := `{"googlelogin":true,"googlejwt":""}`
	resp := postSession(body)
	assert.Equal(t, 400, resp.StatusCode)
}

// =============================================================================
// Facebook Login Tests
// =============================================================================

func newFacebookMockServer(id, email, firstName, lastName, name string, statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if statusCode != 200 {
			w.WriteHeader(statusCode)
			w.Write([]byte(`{"error": {"message": "Invalid OAuth access token."}}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]string{
			"id":         id,
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"name":       name,
		})
	}))
}

func TestFacebookLoginNewUser(t *testing.T) {
	prefix := uniquePrefix("fb-new")
	email := fmt.Sprintf("%s@facebook.com", prefix)
	fbID := "fb-uid-" + prefix

	server := newFacebookMockServer(fbID, email, "FB", "User", "FB User", 200)
	defer server.Close()

	os.Setenv("FACEBOOK_GRAPH_URL", server.URL)
	defer os.Unsetenv("FACEBOOK_GRAPH_URL")

	body := `{"fblogin":1,"fbaccesstoken":"fake-access-token"}`
	resp := postSession(body)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])
	assert.NotNil(t, result["jwt"])

	// Verify user was created.
	db := database.DBConn
	var userID uint64
	db.Raw("SELECT userid FROM users_logins WHERE type = 'Facebook' AND uid = ?", fbID).Scan(&userID)
	assert.NotEqual(t, uint64(0), userID)

	// Cleanup.
	db.Exec("DELETE FROM users_logins WHERE userid = ?", userID)
	db.Exec("DELETE FROM users_emails WHERE userid = ?", userID)
	db.Exec("DELETE FROM sessions WHERE userid = ?", userID)
	db.Exec("DELETE FROM users WHERE id = ?", userID)
}

func TestFacebookLoginExistingUserByEmail(t *testing.T) {
	prefix := uniquePrefix("fb-email")
	email := fmt.Sprintf("%s@test.com", prefix)
	userID := CreateTestUser(t, prefix, "User")
	fbID := "fb-uid-" + prefix

	server := newFacebookMockServer(fbID, email, "Test", prefix, "Test "+prefix, 200)
	defer server.Close()

	os.Setenv("FACEBOOK_GRAPH_URL", server.URL)
	defer os.Unsetenv("FACEBOOK_GRAPH_URL")

	body := `{"fblogin":true,"fbaccesstoken":"fake-access-token"}`
	resp := postSession(body)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify Facebook login linked to existing user.
	db := database.DBConn
	var loginUserID uint64
	db.Raw("SELECT userid FROM users_logins WHERE type = 'Facebook' AND uid = ?", fbID).Scan(&loginUserID)
	assert.Equal(t, userID, loginUserID)

	// Cleanup.
	db.Exec("DELETE FROM users_logins WHERE userid = ? AND type = 'Facebook'", userID)
	db.Exec("DELETE FROM sessions WHERE userid = ?", userID)
}

func TestFacebookLoginExistingUserByFBUID(t *testing.T) {
	prefix := uniquePrefix("fb-uid")
	userID := CreateTestUser(t, prefix, "User")
	fbID := "fb-uid-" + prefix

	db := database.DBConn
	db.Exec("INSERT INTO users_logins (userid, type, uid) VALUES (?, 'Facebook', ?)", userID, fbID)

	server := newFacebookMockServer(fbID, "other@facebook.com", "Test", prefix, "Test "+prefix, 200)
	defer server.Close()

	os.Setenv("FACEBOOK_GRAPH_URL", server.URL)
	defer os.Unsetenv("FACEBOOK_GRAPH_URL")

	body := `{"fblogin":"1","fbaccesstoken":"fake-access-token"}`
	resp := postSession(body)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])

	// Cleanup.
	db.Exec("DELETE FROM users_logins WHERE userid = ? AND type = 'Facebook'", userID)
	db.Exec("DELETE FROM sessions WHERE userid = ?", userID)
}

func TestFacebookLoginInvalidToken(t *testing.T) {
	server := newFacebookMockServer("", "", "", "", "", 400)
	defer server.Close()

	os.Setenv("FACEBOOK_GRAPH_URL", server.URL)
	defer os.Unsetenv("FACEBOOK_GRAPH_URL")

	body := `{"fblogin":1,"fbaccesstoken":"invalid-token"}`
	resp := postSession(body)
	assert.Equal(t, 401, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestFacebookLimitedLoginNewUser(t *testing.T) {
	prefix := uniquePrefix("fb-limited")
	email := fmt.Sprintf("%s@facebook.com", prefix)
	fbSub := "fb-limited-uid-" + prefix

	// Generate an RSA key pair for signing.
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)

	kid := "test-kid-1"

	// Create a JWKS mock server.
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nBytes := privateKey.PublicKey.N.Bytes()
		eBytes := big.NewInt(int64(privateKey.PublicKey.E)).Bytes()
		jwks := map[string]interface{}{
			"keys": []map[string]string{
				{
					"kty": "RSA",
					"kid": kid,
					"use": "sig",
					"alg": "RS256",
					"n":   base64.RawURLEncoding.EncodeToString(nBytes),
					"e":   base64.RawURLEncoding.EncodeToString(eBytes),
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))
	defer jwksServer.Close()

	// Create a signed JWT.
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub":         fbSub,
		"email":       email,
		"given_name":  "FB",
		"family_name": "Limited",
		"name":        "FB Limited",
		"iat":         time.Now().Unix(),
		"exp":         time.Now().Add(time.Hour).Unix(),
	})
	token.Header["kid"] = kid
	signedToken, err := token.SignedString(privateKey)
	assert.NoError(t, err)

	os.Setenv("FACEBOOK_JWKS_URL", jwksServer.URL)
	defer os.Unsetenv("FACEBOOK_JWKS_URL")

	body := fmt.Sprintf(`{"fblogin":1,"fbaccesstoken":"%s","fblimited":1}`, signedToken)
	resp := postSession(body)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])
	assert.NotNil(t, result["jwt"])

	// Verify user was created.
	db := database.DBConn
	var userID uint64
	db.Raw("SELECT userid FROM users_logins WHERE type = 'Facebook' AND uid = ?", fbSub).Scan(&userID)
	assert.NotEqual(t, uint64(0), userID)

	// Cleanup.
	db.Exec("DELETE FROM users_logins WHERE userid = ?", userID)
	db.Exec("DELETE FROM users_emails WHERE userid = ?", userID)
	db.Exec("DELETE FROM sessions WHERE userid = ?", userID)
	db.Exec("DELETE FROM users WHERE id = ?", userID)
}

// =============================================================================
// Social Auth Helper Tests
// =============================================================================

func TestSocialMatchOrCreateNewUser(t *testing.T) {
	prefix := uniquePrefix("social-new")
	email := fmt.Sprintf("%s@social.com", prefix)
	uid := "social-uid-" + prefix

	// Call via a Google login through the API to exercise the full path.
	clientID := "test-social-client-id"
	server := newGoogleMockServer(uid, email, "Social", "New", "Social New", clientID, 200)
	defer server.Close()

	os.Setenv("GOOGLE_TOKENINFO_URL", server.URL)
	os.Setenv("GOOGLE_CLIENT_ID", clientID)
	defer os.Unsetenv("GOOGLE_TOKENINFO_URL")
	defer os.Unsetenv("GOOGLE_CLIENT_ID")

	body := `{"googlelogin":true,"googlejwt":"test-token"}`
	resp := postSession(body)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify user and records.
	db := database.DBConn
	var userID uint64
	db.Raw("SELECT userid FROM users_logins WHERE type = 'Google' AND uid = ?", uid).Scan(&userID)
	assert.NotEqual(t, uint64(0), userID)

	var emailCount int64
	db.Raw("SELECT COUNT(*) FROM users_emails WHERE userid = ? AND email = ?", userID, email).Scan(&emailCount)
	assert.Equal(t, int64(1), emailCount)

	// Cleanup.
	db.Exec("DELETE FROM users_logins WHERE userid = ?", userID)
	db.Exec("DELETE FROM users_emails WHERE userid = ?", userID)
	db.Exec("DELETE FROM sessions WHERE userid = ?", userID)
	db.Exec("DELETE FROM users WHERE id = ?", userID)
}

func TestSocialMatchOrCreateExistingEmail(t *testing.T) {
	prefix := uniquePrefix("social-email")
	email := fmt.Sprintf("%s@test.com", prefix)
	userID := CreateTestUser(t, prefix, "User")

	clientID := "test-social-client-id"
	server := newGoogleMockServer("social-uid-"+prefix, email, "Test", prefix, "Test "+prefix, clientID, 200)
	defer server.Close()

	os.Setenv("GOOGLE_TOKENINFO_URL", server.URL)
	os.Setenv("GOOGLE_CLIENT_ID", clientID)
	defer os.Unsetenv("GOOGLE_TOKENINFO_URL")
	defer os.Unsetenv("GOOGLE_CLIENT_ID")

	body := `{"googlelogin":true,"googlejwt":"test-token"}`
	resp := postSession(body)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify the login was linked to the existing user, not a new one.
	db := database.DBConn
	var loginUserID uint64
	db.Raw("SELECT userid FROM users_logins WHERE type = 'Google' AND uid = ?", "social-uid-"+prefix).Scan(&loginUserID)
	assert.Equal(t, userID, loginUserID)

	// Cleanup.
	db.Exec("DELETE FROM users_logins WHERE userid = ? AND type = 'Google'", userID)
	db.Exec("DELETE FROM sessions WHERE userid = ?", userID)
}

func TestSocialMatchOrCreateExistingLogin(t *testing.T) {
	prefix := uniquePrefix("social-login")
	userID := CreateTestUser(t, prefix, "User")
	uid := "social-uid-" + prefix

	db := database.DBConn
	db.Exec("INSERT INTO users_logins (userid, type, uid) VALUES (?, 'Google', ?)", userID, uid)

	clientID := "test-social-client-id"
	server := newGoogleMockServer(uid, "newemail@gmail.com", "Test", prefix, "Test "+prefix, clientID, 200)
	defer server.Close()

	os.Setenv("GOOGLE_TOKENINFO_URL", server.URL)
	os.Setenv("GOOGLE_CLIENT_ID", clientID)
	defer os.Unsetenv("GOOGLE_TOKENINFO_URL")
	defer os.Unsetenv("GOOGLE_CLIENT_ID")

	body := `{"googlelogin":true,"googlejwt":"test-token"}`
	resp := postSession(body)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Cleanup.
	db.Exec("DELETE FROM users_logins WHERE userid = ? AND type = 'Google'", userID)
	db.Exec("DELETE FROM sessions WHERE userid = ?", userID)
}

func TestSocialMatchOrCreateTNUser(t *testing.T) {
	prefix := uniquePrefix("social-tn")
	email := fmt.Sprintf("%s@test.com", prefix)

	// Create user with tnuserid set.
	db := database.DBConn
	fullname := fmt.Sprintf("Test User %s", prefix)
	db.Exec("INSERT INTO users (firstname, lastname, fullname, systemrole, tnuserid) VALUES ('Test', ?, ?, 'User', 12345)",
		prefix, fullname)

	var userID uint64
	db.Raw("SELECT id FROM users WHERE fullname = ? ORDER BY id DESC LIMIT 1", fullname).Scan(&userID)
	if userID == 0 {
		t.Fatalf("Failed to create TN test user")
	}

	db.Exec("INSERT INTO users_emails (userid, email) VALUES (?, ?)", userID, email)

	clientID := "test-social-client-id"
	server := newGoogleMockServer("tn-google-uid-"+prefix, email, "TN", "User", "TN User", clientID, 200)
	defer server.Close()

	os.Setenv("GOOGLE_TOKENINFO_URL", server.URL)
	os.Setenv("GOOGLE_CLIENT_ID", clientID)
	defer os.Unsetenv("GOOGLE_TOKENINFO_URL")
	defer os.Unsetenv("GOOGLE_CLIENT_ID")

	body := `{"googlelogin":true,"googlejwt":"test-token"}`
	resp := postSession(body)

	// Should fail for TN users.
	assert.Equal(t, 403, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(3), result["ret"])
	assert.True(t, strings.Contains(result["status"].(string), "TN user"))

	// Cleanup.
	db.Exec("DELETE FROM users_emails WHERE userid = ?", userID)
	db.Exec("DELETE FROM sessions WHERE userid = ?", userID)
	db.Exec("DELETE FROM users WHERE id = ?", userID)
}
