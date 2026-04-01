package sso

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
)

// ForumSSO handles the Forum SSO login flow.
// This is the Go equivalent of iznik-server/http/forum_sso.php.
//
// Similar to DiscourseSSO but:
// - Uses FORUM_SECRET (not DISCOURSE_SECRET)
// - Uses Iznik-Forum-SSO cookie (not Iznik-Discourse-SSO)
// - Does NOT require moderator — any logged-in user can use it
// - Bio says "Freegle Volunteer" for mods, "Member" for regular users
//
// @Summary Forum SSO login
// @Tags sso
// @Param sso query string true "Base64-encoded SSO payload"
// @Param sig query string true "HMAC-SHA256 signature"
// @Success 302
// @Router /forum_sso [get]
func ForumSSO(c *fiber.Ctx) error {
	ssoPayload := c.Query("sso")
	sig := c.Query("sig")

	log.Printf("[ForumSSO] Received SSO request")

	secret := os.Getenv("FORUM_SECRET")
	if secret == "" {
		log.Printf("[ForumSSO] FORUM_SECRET not set")
		return c.Status(fiber.StatusInternalServerError).SendString("SSO not configured")
	}

	// Validate the HMAC-SHA256 signature.
	if !validateHMAC(ssoPayload, sig, secret) {
		log.Printf("[ForumSSO] Invalid signature")
		return c.Status(fiber.StatusForbidden).SendString("Invalid signature")
	}

	log.Printf("[ForumSSO] Signature validated")

	// Decode the payload to get the nonce.
	nonce, err := extractNonce(ssoPayload)
	if err != nil {
		log.Printf("[ForumSSO] Failed to extract nonce: %v", err)
		return c.Status(fiber.StatusBadRequest).SendString("Invalid SSO payload")
	}

	// Look up user from the Iznik-Forum-SSO cookie.
	cookieValue := c.Cookies("Iznik-Forum-SSO")
	if cookieValue == "" {
		log.Printf("[ForumSSO] No cookie, redirecting to login")
		return c.Redirect("http://ilovefreegle.org/forum", fiber.StatusFound)
	}

	// Cookie value may be URL-encoded (browsers sometimes encode JSON cookies).
	if decoded, err2 := url.QueryUnescape(cookieValue); err2 == nil {
		cookieValue = decoded
	}

	session, err := validateForumSession(cookieValue)
	if err != nil {
		log.Printf("[ForumSSO] Session validation failed: %v", err)
		return c.Redirect("http://ilovefreegle.org/forum", fiber.StatusFound)
	}

	// Build the SSO response.
	responsePayload := buildForumSSOResponse(nonce, session)
	encodedPayload := base64.StdEncoding.EncodeToString([]byte(responsePayload))
	responseSig := computeHMAC(encodedPayload, secret)

	redirectURL := fmt.Sprintf("https://forum.ilovefreegle.org/session/sso_login?sso=%s&sig=%s",
		url.QueryEscape(encodedPayload), url.QueryEscape(responseSig))

	log.Printf("[ForumSSO] Logged in %s, redirecting to Forum", session.Name)
	return c.Redirect(redirectURL, fiber.StatusFound)
}

// validateForumSession validates the cookie against the sessions table.
// Any logged-in user is allowed (no moderator requirement).
func validateForumSession(cookieValue string) (*ssoSession, error) {
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

	// Look up session — any valid session is accepted.
	type SessionRow struct {
		UserID uint64 `gorm:"column:userid"`
	}

	var sessions []SessionRow
	db.Raw(`SELECT userid FROM sessions
		WHERE id = ? AND series = ? AND token = ?`,
		cookie.ID, cookie.Series, cookie.Token).Scan(&sessions)

	if len(sessions) == 0 {
		return nil, fmt.Errorf("no valid session found")
	}

	userID := sessions[0].UserID

	// Get user details.
	var fullname string
	db.Raw("SELECT COALESCE(fullname, '') FROM users WHERE id = ?", userID).Scan(&fullname)

	var email string
	db.Raw("SELECT email FROM users_emails WHERE userid = ? ORDER BY preferred DESC LIMIT 1", userID).Scan(&email)

	var profileURL string
	db.Raw("SELECT url FROM users_images WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&profileURL)

	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", userID).Scan(&systemrole)
	isAdmin := systemrole == "Admin"

	// Check if user is a moderator on any Freegle group.
	var modGroupCount int64
	db.Raw(`SELECT COUNT(*) FROM memberships
		INNER JOIN `+"`groups`"+` ON memberships.groupid = `+"`groups`"+`.id
		WHERE memberships.userid = ? AND memberships.role IN ('Owner', 'Moderator')
		AND `+"`groups`"+`.type = 'Freegle'`, userID).Scan(&modGroupCount)
	isMod := modGroupCount > 0

	// Get group list — all Freegle memberships.
	groupList := getForumGroupList(userID)

	return &ssoSession{
		UserID:    userID,
		Name:      fullname,
		AvatarURL: profileURL,
		Admin:     isAdmin,
		Email:     email,
		GroupList: groupList,
		IsMod:     isMod,
	}, nil
}

// getForumGroupList returns a comma-separated list of all Freegle group display names for a user.
func getForumGroupList(userID uint64) string {
	db := database.DBConn

	type GroupName struct {
		NameDisplay string `gorm:"column:namedisplay"`
	}

	var groups []GroupName
	db.Raw(`SELECT COALESCE(namefull, nameshort) AS namedisplay FROM `+"`groups`"+`
		INNER JOIN memberships ON memberships.groupid = `+"`groups`"+`.id
		WHERE memberships.userid = ?
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

// buildForumSSOResponse builds the query string for the Forum SSO response.
func buildForumSSOResponse(nonce string, session *ssoSession) string {
	var bio string
	if session.IsMod {
		bio = "Freegle Volunteer on " + session.GroupList
	} else {
		bio = "Member on " + session.GroupList
	}

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
