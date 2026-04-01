package sso

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
)

// ssoSession holds the user data we need after session validation.
type ssoSession struct {
	UserID    uint64
	Name      string
	AvatarURL string
	Admin     bool
	Email     string
	GroupList string
	IsMod     bool
}

// DiscourseSSO handles the Discourse SSO login flow.
// This is the Go equivalent of iznik-server/http/discourse_sso.php.
//
// Discourse sends GET with `sso` and `sig` query params. We validate the signature,
// look up the user from the Iznik-Discourse-SSO cookie, verify they are a Freegle moderator,
// and redirect back to Discourse with the signed SSO response.
//
// @Summary Discourse SSO login
// @Tags sso
// @Param sso query string true "Base64-encoded SSO payload"
// @Param sig query string true "HMAC-SHA256 signature"
// @Success 302
// @Router /discourse_sso [get]
func DiscourseSSO(c *fiber.Ctx) error {
	ssoPayload := c.Query("sso")
	sig := c.Query("sig")

	log.Printf("[DiscourseSSO] Received SSO request")

	secret := os.Getenv("DISCOURSE_SECRET")
	if secret == "" {
		log.Printf("[DiscourseSSO] DISCOURSE_SECRET not set")
		return c.Status(fiber.StatusInternalServerError).SendString("SSO not configured")
	}

	// Validate the HMAC-SHA256 signature.
	if !validateHMAC(ssoPayload, sig, secret) {
		log.Printf("[DiscourseSSO] Invalid signature")
		return c.Status(fiber.StatusForbidden).SendString("Invalid signature")
	}

	log.Printf("[DiscourseSSO] Signature validated")

	// Decode the payload to get the nonce.
	nonce, err := extractNonce(ssoPayload)
	if err != nil {
		log.Printf("[DiscourseSSO] Failed to extract nonce: %v", err)
		return c.Status(fiber.StatusBadRequest).SendString("Invalid SSO payload")
	}

	// Look up user from the Iznik-Discourse-SSO cookie.
	cookieValue := c.Cookies("Iznik-Discourse-SSO")
	if cookieValue == "" {
		log.Printf("[DiscourseSSO] No cookie, redirecting to login")
		return c.Redirect("https://modtools.org/discourse", fiber.StatusFound)
	}

	// Cookie value may be URL-encoded (browsers sometimes encode JSON cookies).
	if decoded, err2 := url.QueryUnescape(cookieValue); err2 == nil {
		cookieValue = decoded
	}

	session, err := validateDiscourseSession(cookieValue)
	if err != nil {
		log.Printf("[DiscourseSSO] Session validation failed: %v", err)
		return c.Redirect("https://modtools.org/discourse", fiber.StatusFound)
	}

	// Build the SSO response.
	responsePayload := buildSSOResponse(nonce, session)
	encodedPayload := base64.StdEncoding.EncodeToString([]byte(responsePayload))
	responseSig := computeHMAC(encodedPayload, secret)

	redirectURL := fmt.Sprintf("https://discourse.ilovefreegle.org/session/sso_login?sso=%s&sig=%s",
		url.QueryEscape(encodedPayload), url.QueryEscape(responseSig))

	log.Printf("[DiscourseSSO] Logged in %s, redirecting to Discourse", session.Name)
	return c.Redirect(redirectURL, fiber.StatusFound)
}

// validateDiscourseSession validates the cookie against the sessions table.
// The user must be a Freegle moderator.
func validateDiscourseSession(cookieValue string) (*ssoSession, error) {
	db := database.DBConn

	var cookie struct {
		ID     uint64 `json:"id"`
		Series uint64 `json:"series"`
		Token  string `json:"token"`
	}

	if err := json.Unmarshal([]byte(cookieValue), &cookie); err != nil {
		return nil, fmt.Errorf("invalid cookie JSON: %w", err)
	}

	if cookie.ID == 0 || cookie.Series == 0 || cookie.Token == "" {
		return nil, fmt.Errorf("incomplete cookie data")
	}

	// Look up session — user must have a moderator/admin/support systemrole.
	type SessionRow struct {
		UserID uint64 `gorm:"column:userid"`
	}

	var sessions []SessionRow
	db.Raw(`SELECT sessions.userid FROM sessions
		INNER JOIN users ON sessions.userid = users.id
		WHERE users.systemrole IN ('Admin', 'Support', 'Moderator')
		AND sessions.id = ? AND sessions.series = ? AND sessions.token = ?`,
		cookie.ID, cookie.Series, cookie.Token).Scan(&sessions)

	if len(sessions) == 0 {
		return nil, fmt.Errorf("no valid moderator session found")
	}

	userID := sessions[0].UserID

	// Check they are a mod on a Freegle group.
	var freegleGroupCount int64
	db.Raw(`SELECT COUNT(*) FROM memberships
		INNER JOIN ` + "`groups`" + ` ON memberships.groupid = ` + "`groups`" + `.id
		WHERE memberships.userid = ? AND memberships.role IN ('Owner', 'Moderator')
		AND ` + "`groups`" + `.type = 'Freegle'`, userID).Scan(&freegleGroupCount)

	if freegleGroupCount == 0 {
		return nil, fmt.Errorf("user %d is not a moderator of a Freegle group", userID)
	}

	// Get user details.
	var fullname string
	db.Raw("SELECT COALESCE(fullname, '') FROM users WHERE id = ?", userID).Scan(&fullname)

	var email string
	db.Raw("SELECT email FROM users_emails WHERE userid = ? ORDER BY preferred DESC LIMIT 1", userID).Scan(&email)

	var profileURL string
	db.Raw("SELECT url FROM users_images WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&profileURL)

	var isAdmin bool
	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", userID).Scan(&systemrole)
	isAdmin = systemrole == "Admin"

	// Get group list — try active mod groups first, fall back to all moderatorships.
	groupList := getModGroupList(userID)

	return &ssoSession{
		UserID:    userID,
		Name:      fullname,
		AvatarURL: profileURL,
		Admin:     isAdmin,
		Email:     email,
		GroupList: groupList,
		IsMod:     true,
	}, nil
}

// getModGroupList returns a comma-separated list of group display names for a moderator.
func getModGroupList(userID uint64) string {
	db := database.DBConn

	type GroupName struct {
		NameDisplay string `gorm:"column:namedisplay"`
	}

	var groups []GroupName
	db.Raw(`SELECT COALESCE(namefull, nameshort) AS namedisplay FROM `+"`groups`"+`
		INNER JOIN memberships ON memberships.groupid = `+"`groups`"+`.id
		WHERE memberships.userid = ? AND memberships.role IN ('Owner', 'Moderator')
		AND `+"`groups`"+`.type = 'Freegle'`, userID).Scan(&groups)

	names := make([]string, 0, len(groups))
	for _, g := range groups {
		names = append(names, g.NameDisplay)
	}

	result := strings.Join(names, ",")
	if len(result) > 1000 {
		result = result[:1000]
	}

	return result
}

// validateHMAC checks that the HMAC-SHA256 of the payload matches the signature.
func validateHMAC(payload, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// computeHMAC returns the hex-encoded HMAC-SHA256 of the data.
func computeHMAC(data, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// extractNonce decodes the base64 SSO payload and extracts the nonce value.
func extractNonce(ssoPayload string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(ssoPayload)
	if err != nil {
		return "", fmt.Errorf("base64 decode failed: %w", err)
	}

	values, err := url.ParseQuery(string(decoded))
	if err != nil {
		return "", fmt.Errorf("query parse failed: %w", err)
	}

	nonce := values.Get("nonce")
	if nonce == "" {
		return "", fmt.Errorf("no nonce in payload")
	}

	return nonce, nil
}

// buildSSOResponse builds the query string for the Discourse SSO response.
func buildSSOResponse(nonce string, session *ssoSession) string {
	bio := session.Email + " \r\n\r\nis a mod on " + session.GroupList

	params := url.Values{}
	params.Set("nonce", nonce)
	params.Set("email", session.Email)
	params.Set("external_id", fmt.Sprint(session.UserID))
	params.Set("username", session.Name)
	params.Set("name", session.Name)
	params.Set("avatar_url", session.AvatarURL)
	if session.Admin {
		params.Set("admin", "true")
	} else {
		params.Set("admin", "false")
	}
	params.Set("bio", bio)

	return params.Encode()
}
