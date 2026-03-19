package session

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	stdlog "log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/freegle/iznik-server-go/auth"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/queue"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
)

// fetchDiscourseStats fetches notification and topic counts from the Discourse API.
// Returns nil if Discourse is not configured or the API call fails.
func fetchDiscourseStats(myid uint64) fiber.Map {
	discourseAPI := os.Getenv("DISCOURSE_API")
	discourseKey := os.Getenv("DISCOURSE_APIKEY")
	if discourseAPI == "" || discourseKey == "" {
		return nil
	}

	client := &http.Client{Timeout: 2 * time.Second}

	// Look up the user's Discourse username by external ID.
	req, err := http.NewRequest("GET", discourseAPI+"/users/by-external/"+strconv.FormatUint(myid, 10)+".json", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Api-Key", discourseKey)
	req.Header.Set("Api-Username", "system")

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var userResp struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &userResp); err != nil || userResp.User.Username == "" {
		return nil
	}

	username := userResp.User.Username

	// Fetch counts in parallel.
	var notifications, newtopics, unreadtopics int64
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		req, err := http.NewRequest("GET", discourseAPI+"/session/current.json", nil)
		if err != nil {
			return
		}
		req.Header.Set("Api-Key", discourseKey)
		req.Header.Set("Api-Username", username)
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != 200 {
			if resp != nil {
				resp.Body.Close()
			}
			return
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var sr struct {
			CurrentUser struct {
				UnreadNotifications int64 `json:"unread_notifications"`
			} `json:"current_user"`
		}
		json.Unmarshal(body, &sr)
		notifications = sr.CurrentUser.UnreadNotifications
	}()

	go func() {
		defer wg.Done()
		req, err := http.NewRequest("GET", discourseAPI+"/new.json", nil)
		if err != nil {
			return
		}
		req.Header.Set("Api-Key", discourseKey)
		req.Header.Set("Api-Username", username)
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != 200 {
			if resp != nil {
				resp.Body.Close()
			}
			return
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var tr struct {
			TopicList struct {
				Topics []interface{} `json:"topics"`
			} `json:"topic_list"`
		}
		json.Unmarshal(body, &tr)
		newtopics = int64(len(tr.TopicList.Topics))
	}()

	go func() {
		defer wg.Done()
		req, err := http.NewRequest("GET", discourseAPI+"/unread.json", nil)
		if err != nil {
			return
		}
		req.Header.Set("Api-Key", discourseKey)
		req.Header.Set("Api-Username", username)
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != 200 {
			if resp != nil {
				resp.Body.Close()
			}
			return
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var tr struct {
			TopicList struct {
				Topics []interface{} `json:"topics"`
			} `json:"topic_list"`
		}
		json.Unmarshal(body, &tr)
		unreadtopics = int64(len(tr.TopicList.Topics))
	}()

	wg.Wait()

	return fiber.Map{
		"notifications": notifications,
		"newtopics":     newtopics,
		"unreadtopics":  unreadtopics,
		"timestamp":     time.Now().Unix(),
	}
}

// FlexUint64 accepts both numeric and string JSON values.
type FlexUint64 uint64

func (f *FlexUint64) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), "\"")
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return err
	}
	*f = FlexUint64(v)
	return nil
}

// PostSessionRequest covers all fields used across session POST actions.
type PostSessionRequest struct {
	Action   string     `json:"action"`
	Email    string     `json:"email"`
	Password string     `json:"password"`
	U        FlexUint64 `json:"u"`
	K        string     `json:"k"`
	Userlist []uint64   `json:"userlist"`
	Partner  string     `json:"partner"`
	ID       uint64     `json:"id"`
}

// PostSession dispatches session write actions.
//
// @Summary Session actions (LostPassword, Unsubscribe, Login, Forget, Related)
// @Tags session
// @Router /session [post]
func PostSession(c *fiber.Ctx) error {
	var req PostSessionRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	switch req.Action {
	case "LostPassword":
		return handleLostPassword(c, req.Email)
	case "Unsubscribe":
		return handleUnsubscribe(c, req.Email)
	case "Forget":
		return handleForget(c, req.Partner, req.ID)
	case "Related":
		return handleRelated(c, req.Userlist)
	default:
		// No action means login attempt.
		if req.Email != "" && req.Password != "" {
			return handleEmailPasswordLogin(c, req.Email, req.Password)
		}
		if uint64(req.U) > 0 && req.K != "" {
			return handleLinkLogin(c, uint64(req.U), req.K)
		}

		// If we get here with a non-empty action we don't recognise, error.
		if req.Action != "" {
			return fiber.NewError(fiber.StatusBadRequest, "Unsupported action")
		}

		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}
}

// handleLostPassword finds the user by email and queues a forgot-password email.
func handleLostPassword(c *fiber.Ctx, email string) error {
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email parameter required")
	}

	db := database.DBConn

	// Find user by email (must not be deleted).
	var userID uint64
	db.Raw("SELECT users.id FROM users "+
		"INNER JOIN users_emails ON users_emails.userid = users.id "+
		"WHERE users_emails.email = ? AND users.deleted IS NULL "+
		"LIMIT 1", email).Scan(&userID)

	if userID == 0 {
		// Return ret=2 for unknown email.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"ret":    2,
			"status": "We don't know that email address.",
		})
	}

	// Get or create the auto-login key for this user.
	key, err := getOrCreateLoginKey(userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to generate login key")
	}

	// Build the auto-login URL: /settings?u={id}&k={key}&src=forgotpass
	userSite := os.Getenv("USER_SITE")
	resetURL := fmt.Sprintf("https://%s/settings?u=%d&k=%s&src=forgotpass", userSite, userID, key)

	// Get user's preferred email for sending.
	var preferredEmail string
	db.Raw("SELECT email FROM users_emails WHERE userid = ? ORDER BY preferred DESC, id ASC LIMIT 1", userID).Scan(&preferredEmail)

	if preferredEmail == "" {
		preferredEmail = email
	}

	// Queue the forgot-password email.
	if err := queue.QueueTask(queue.TaskEmailForgotPassword, map[string]interface{}{
		"user_id":   userID,
		"email":     preferredEmail,
		"reset_url": resetURL,
	}); err != nil {
		stdlog.Printf("Failed to queue forgot-password email for user %d: %v", userID, err)
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
	})
}

// handleUnsubscribe finds the user by email and queues an unsubscribe confirmation email.
func handleUnsubscribe(c *fiber.Ctx, email string) error {
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email parameter required")
	}

	db := database.DBConn

	// Find user by email (must not be deleted).
	var userID uint64
	db.Raw("SELECT users.id FROM users "+
		"INNER JOIN users_emails ON users_emails.userid = users.id "+
		"WHERE users_emails.email = ? AND users.deleted IS NULL "+
		"LIMIT 1", email).Scan(&userID)

	if userID == 0 {
		// Return success even for unknown emails to prevent email enumeration.
		return c.JSON(fiber.Map{
			"ret":       0,
			"status":    "Success",
			"emailsent": true,
		})
	}

	// Get or create the auto-login key.
	key, err := getOrCreateLoginKey(userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to generate login key")
	}

	// Build the unsubscribe URL: /unsubscribe/{id}?u={id}&k={key}&confirm=1
	userSite := os.Getenv("USER_SITE")
	unsubURL := fmt.Sprintf("https://%s/unsubscribe/%d?u=%d&k=%s&confirm=1", userSite, userID, userID, key)

	// Get user's preferred email.
	var preferredEmail string
	db.Raw("SELECT email FROM users_emails WHERE userid = ? ORDER BY preferred DESC, id ASC LIMIT 1", userID).Scan(&preferredEmail)

	if preferredEmail == "" {
		preferredEmail = email
	}

	// Queue the unsubscribe confirmation email.
	if err := queue.QueueTask(queue.TaskEmailUnsubscribe, map[string]interface{}{
		"user_id":    userID,
		"email":      preferredEmail,
		"unsub_url":  unsubURL,
	}); err != nil {
		stdlog.Printf("Failed to queue unsubscribe email for user %d: %v", userID, err)
	}

	return c.JSON(fiber.Map{
		"ret":       0,
		"status":    "Success",
		"emailsent": true,
	})
}

// getOrCreateLoginKey retrieves or creates a 32-char hex auto-login key
// stored in users_logins with type='Link'.
func getOrCreateLoginKey(userID uint64) (string, error) {
	db := database.DBConn

	// Check for existing key.
	var existingKey string
	db.Raw("SELECT credentials FROM users_logins WHERE userid = ? AND type = 'Link' LIMIT 1", userID).Scan(&existingKey)

	if existingKey != "" {
		return existingKey, nil
	}

	// Generate a new 32-char hex key (16 random bytes → 32 hex chars).
	newKey := utils.RandomHex(16)

	// Insert the login key. Use uid=userid as a unique identifier.
	db.Exec("INSERT INTO users_logins (userid, type, uid, credentials) VALUES (?, 'Link', ?, ?)",
		userID, fmt.Sprintf("%d", userID), newKey)

	return newKey, nil
}

// Delegated to auth package to break circular dependency with user package.

// handleEmailPasswordLogin authenticates via email and sha1-hashed password.
func handleEmailPasswordLogin(c *fiber.Ctx, email string, password string) error {
	db := database.DBConn

	// Find user by email (must not be deleted).
	var userID uint64
	db.Raw("SELECT u.id FROM users u "+
		"JOIN users_emails ue ON ue.userid = u.id "+
		"WHERE ue.email = ? AND u.deleted IS NULL "+
		"LIMIT 1", email).Scan(&userID)

	if userID == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"ret":    2,
			"status": "We don't know that email address.",
		})
	}

	if !auth.VerifyPassword(userID, password) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"ret":    3,
			"status": "The password is wrong.",
		})
	}

	persistent, jwtString, err := auth.CreateSessionAndJWT(userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create session")
	}

	return c.JSON(fiber.Map{
		"ret":        0,
		"status":     "Success",
		"persistent": persistent,
		"jwt":        jwtString,
	})
}

// handleLinkLogin authenticates via userid + link key.
func handleLinkLogin(c *fiber.Ctx, uid uint64, key string) error {
	db := database.DBConn

	// Verify the user exists and is not deleted.
	var exists uint64
	db.Raw("SELECT id FROM users WHERE id = ? AND deleted IS NULL LIMIT 1", uid).Scan(&exists)

	if exists == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"ret":    2,
			"status": "Unknown user.",
		})
	}

	// Verify the link key.
	var storedKey string
	db.Raw("SELECT credentials FROM users_logins WHERE userid = ? AND type = 'Link' LIMIT 1", uid).Scan(&storedKey)

	if storedKey == "" || subtle.ConstantTimeCompare([]byte(storedKey), []byte(key)) != 1 {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"ret":    3,
			"status": "Invalid key.",
		})
	}

	persistent, jwtString, err := auth.CreateSessionAndJWT(uid)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create session")
	}

	return c.JSON(fiber.Map{
		"ret":        0,
		"status":     "Success",
		"persistent": persistent,
		"jwt":        jwtString,
	})
}

// handleForget puts a user into "limbo" — soft-deleted but recoverable for ~14 days.
// Supports two flows: partner-authenticated (for integrated services) and self-service.
func handleForget(c *fiber.Ctx, partner string, targetID uint64) error {
	db := database.DBConn

	if partner != "" {
		// Partner flow: a partner service can delete users it manages.
		var partnerID uint64
		db.Raw("SELECT id FROM partners_keys WHERE `key` = ?", partner).Scan(&partnerID)

		if partnerID == 0 {
			return fiber.NewError(fiber.StatusForbidden, "Invalid partner key")
		}

		if targetID == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "id is required for partner forget")
		}

		// Only allow for users linked via partner (ljuserid set).
		var ljuserid *uint64
		db.Raw("SELECT ljuserid FROM users WHERE id = ?", targetID).Scan(&ljuserid)

		if ljuserid == nil || *ljuserid == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "User is not partner-linked")
		}

		db.Exec("UPDATE users SET deleted = NOW() WHERE id = ?", targetID)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
	}

	// Self-service flow: logged-in user deletes their own account.
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	// Moderators must demote themselves first to avoid accidental deletion.
	var modRole string
	db.Raw("SELECT role FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner') LIMIT 1", myid).Scan(&modRole)

	if modRole != "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ret":    2,
			"status": "Please demote yourself to a member first",
		})
	}

	// Spammers cannot delete their own accounts (prevents evasion of tracking).
	var spammerCount int64
	db.Raw("SELECT COUNT(*) FROM spam_users WHERE userid = ? AND collection IN ('Spammer', 'PendingAdd')", myid).Scan(&spammerCount)

	if spammerCount > 0 {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"ret":    3,
			"status": "We can't do this.",
		})
	}

	// Signal the auth middleware to skip the post-handler session check.
	c.Locals("skipPostAuthCheck", true)

	// Soft-delete: user can recover by logging back in within ~14 days.
	db.Exec("UPDATE users SET deleted = NOW() WHERE id = ?", myid)

	// Destroy session so the user is logged out.
	db.Exec("DELETE FROM sessions WHERE userid = ?", myid)

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
	})
}

// handleRelated records related users.
func handleRelated(c *fiber.Ctx, userlist []uint64) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	// Insert related records for each pair.
	for _, otherID := range userlist {
		if otherID != myid && otherID > 0 {
			db.Exec("INSERT IGNORE INTO users_related (user1, user2) VALUES (?, ?)", myid, otherID)
		}
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
	})
}

// isActiveModForGroup checks the membership settings JSON to determine if the
// moderator is actively moderating this group. Defaults to active=1, then checks
// the 'active' key in the JSON settings, falling back to the legacy 'showmessages' key.
func isActiveModForGroup(settingsJSON *string) bool {
	if settingsJSON == nil || *settingsJSON == "" {
		return true // default to active when no settings are present
	}
	var settings map[string]interface{}
	if err := json.Unmarshal([]byte(*settingsJSON), &settings); err != nil {
		return true
	}
	if active, ok := settings["active"]; ok {
		switch v := active.(type) {
		case bool:
			return v
		case float64:
			return v != 0
		}
	}
	// Fallback to legacy showmessages flag (default true if absent).
	if sm, ok := settings["showmessages"]; ok {
		switch v := sm.(type) {
		case bool:
			return v
		case float64:
			return v != 0
		}
	}
	return true
}

// GetSession returns current session info for the logged-in user.
//
// @Summary Get current session
// @Tags session
// @Router /session [get]
func GetSession(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ret":    1,
			"status": "Not logged in",
		})
	}

	db := database.DBConn

	// Parallel fetches for user data.
	type UserRow struct {
		ID            uint64          `json:"id"`
		Fullname      *string         `json:"fullname"`
		Firstname     *string         `json:"firstname"`
		Lastname      *string         `json:"lastname"`
		Systemrole    string          `json:"systemrole"`
		Settings      json.RawMessage `json:"settings"`
		Lastaccess    *time.Time      `json:"lastaccess"`
		Added         *time.Time      `json:"added"`
		Lastlocation  *uint64         `json:"lastlocation"`
		Onholidaytill *string         `json:"onholidaytill"`
		Source        *string         `json:"source"`
		Deleted       *time.Time      `json:"deleted"`
		Trustlevel       *string         `json:"trustlevel"`
		Permissions      *string         `json:"permissions"`
		Marketingconsent bool            `json:"marketingconsent"`
		Bouncing         int             `json:"bouncing"`
	}

	type EmailRow struct {
		ID        uint64 `json:"id"`
		Email     string `json:"email"`
		Preferred int    `json:"preferred"`
		Validated *time.Time   `json:"validated"`
	}

	type MembershipRow struct {
		Groupid             uint64  `json:"groupid"`
		Role                string  `json:"role"`
		Emailfrequency      int     `json:"emailfrequency"`
		Eventsallowed       int     `json:"eventsallowed"`
		Volunteeringallowed int     `json:"volunteeringallowed"`
		Configid            *uint64 `json:"configid"`
		Active              int     `json:"active"`  // 1=active mod, 0=backup mod
		Type                string  `json:"-"`        // Used server-side for moderator detection, not returned to client
		Settings            *string `json:"-"`        // Per-group membership settings JSON, used to determine active/inactive
	}

	type LocationRow struct {
		Name string  `json:"name"`
		Lat  float64 `json:"lat"`
		Lng  float64 `json:"lng"`
	}

	type ProfileRow struct {
		ID           uint64          `json:"id"`
		Externaluid  string          `json:"externaluid"`
		Externalmods json.RawMessage `json:"externalmods"`
	}

	type SessionRow struct {
		ID     uint64 `json:"id"`
		Series string `json:"series"`
		Token  string `json:"token"`
	}

	type AboutmeRow struct {
		Text      string    `json:"text"`
		Timestamp time.Time `json:"timestamp"`
	}

	var wg sync.WaitGroup
	var userRow UserRow
	var emails []EmailRow
	var memberships []MembershipRow
	var sessionRow SessionRow
	var aboutme AboutmeRow

	wg.Add(5)
	go func() {
		defer wg.Done()
		db.Raw("SELECT id, fullname, firstname, lastname, systemrole, settings, lastaccess, added, lastlocation, onholidaytill, source, deleted, trustlevel, permissions, marketingconsent, bouncing FROM users WHERE id = ?", myid).Scan(&userRow)
	}()
	go func() {
		defer wg.Done()
		db.Raw("SELECT id, email, preferred, validated FROM users_emails WHERE userid = ? ORDER BY preferred DESC", myid).Scan(&emails)
	}()
	go func() {
		defer wg.Done()
		db.Raw("SELECT m.groupid, m.role, m.emailfrequency, m.eventsallowed, m.volunteeringallowed, m.configid, g.type, m.settings "+
			"FROM memberships m JOIN `groups` g ON g.id = m.groupid "+
			"WHERE m.userid = ? AND m.collection = 'Approved' ORDER BY LOWER(CASE WHEN g.namefull IS NOT NULL THEN g.namefull ELSE g.nameshort END)", myid).Scan(&memberships)
	}()
	go func() {
		defer wg.Done()
		db.Raw("SELECT id, series, token FROM sessions WHERE userid = ? LIMIT 1", myid).Scan(&sessionRow)
	}()
	go func() {
		defer wg.Done()
		db.Raw("SELECT text, timestamp FROM users_aboutme WHERE userid = ? ORDER BY timestamp DESC LIMIT 1", myid).Scan(&aboutme)
	}()
	wg.Wait()

	// Populate the Active field on each membership from the settings JSON.
	for i := range memberships {
		if isActiveModForGroup(memberships[i].Settings) {
			memberships[i].Active = 1
		} else {
			memberships[i].Active = 0
		}
	}

	// Compute work counts and discourse stats for moderators (depends on memberships).
	var work fiber.Map
	var discourse fiber.Map

	// Collect group IDs where user is a moderator or owner, split by active/inactive.
	// The memberships.settings JSON 'active' flag determines if a mod is actively
	// moderating a group. Inactive groups' work counts show as blue (info) badges instead
	// of red (danger) badges. Default is active.
	var modGroupIDs, activeGroupIDs, inactiveGroupIDs []uint64
	isFreegleMod := false
	for _, m := range memberships {
		if m.Role == "Owner" || m.Role == "Moderator" {
			modGroupIDs = append(modGroupIDs, m.Groupid)
			if m.Active == 1 {
				activeGroupIDs = append(activeGroupIDs, m.Groupid)
			} else {
				inactiveGroupIDs = append(inactiveGroupIDs, m.Groupid)
			}
			if m.Type == "Freegle" {
				isFreegleMod = true
			}
		}
	}

	// Start discourse fetch in parallel with work counts (only for Freegle moderators).
	var discourseWg sync.WaitGroup
	if isFreegleMod {
		discourseWg.Add(1)
		go func() {
			defer discourseWg.Done()
			discourse = fetchDiscourseStats(myid)
		}()
	}

	if len(modGroupIDs) > 0 {
		// Work counts are split by active/inactive group status.
		// Active groups → primary fields (red/danger badges in UI).
		// Inactive groups → "other" fields (blue/info badges in UI).
		// Counts that only appear for active groups: spam, pendingevents, pendingvolunteering,
		// pendingadmins, editreview, happiness, relatedmembers.
		// Counts split by active/inactive: pending/pendingother, spammembers/spammembersother,
		// chatreview/chatreviewother.
		var pending, pendingother, spam int64
		var pendingmembers, spammembers, spammembersother int64
		var pendingevents, pendingadmins, editreview int64
		var pendingvolunteering, spammerpendingadd, spammerpendingremove, stories int64
		var chatreview, chatreviewother, newsletterstories, giftaid, happiness, relatedmembers int64

		var wg2 sync.WaitGroup

		// --- Pending messages: active groups split by held, inactive all → pendingother ---
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			if len(activeGroupIDs) > 0 {
				// Unheld pending in active groups → pending (red).
				db.Raw("SELECT COUNT(*) FROM messages_groups mg "+
					"INNER JOIN messages m ON m.id = mg.msgid "+
					"WHERE mg.groupid IN ? AND mg.collection = 'Pending' AND mg.deleted = 0 "+
					"AND m.fromuser IS NOT NULL AND m.heldby IS NULL",
					activeGroupIDs).Scan(&pending)
				// Held pending in active groups → pendingother (blue).
				var heldActive int64
				db.Raw("SELECT COUNT(*) FROM messages_groups mg "+
					"INNER JOIN messages m ON m.id = mg.msgid "+
					"WHERE mg.groupid IN ? AND mg.collection = 'Pending' AND mg.deleted = 0 "+
					"AND m.fromuser IS NOT NULL AND m.heldby IS NOT NULL",
					activeGroupIDs).Scan(&heldActive)
				pendingother += heldActive
			}
			if len(inactiveGroupIDs) > 0 {
				// All pending in inactive groups → pendingother (blue).
				var inact int64
				db.Raw("SELECT COUNT(*) FROM messages_groups mg "+
					"INNER JOIN messages m ON m.id = mg.msgid "+
					"WHERE mg.groupid IN ? AND mg.collection = 'Pending' AND mg.deleted = 0 "+
					"AND m.fromuser IS NOT NULL",
					inactiveGroupIDs).Scan(&inact)
				pendingother += inact
			}
		}()

		// --- Spam messages (only for active groups) ---
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			if len(activeGroupIDs) > 0 {
				db.Raw("SELECT COUNT(*) FROM messages_groups mg "+
					"INNER JOIN messages m ON m.id = mg.msgid "+
					"WHERE mg.groupid IN ? AND mg.collection = 'Spam' AND mg.deleted = 0 AND m.fromuser IS NOT NULL",
					activeGroupIDs).Scan(&spam)
			}
		}()

		// --- Pending members (all groups, no active/inactive split) ---
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			db.Raw("SELECT COUNT(*) FROM memberships WHERE groupid IN ? AND collection = 'Pending'",
				modGroupIDs).Scan(&pendingmembers)
		}()

		// --- Spam members: active split by held, inactive all → spammembersother ---
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			if len(activeGroupIDs) > 0 {
				// Unheld spam members in active groups → spammembers (red).
				db.Raw("SELECT COUNT(*) FROM memberships "+
					"WHERE groupid IN ? AND (reviewrequestedat IS NOT NULL AND "+
					"(reviewedat IS NULL OR DATE(reviewedat) < DATE_SUB(NOW(), INTERVAL 31 DAY))) "+
					"AND heldby IS NULL",
					activeGroupIDs).Scan(&spammembers)
				// Held spam members in active groups → spammembersother (blue).
				var heldActive int64
				db.Raw("SELECT COUNT(*) FROM memberships "+
					"WHERE groupid IN ? AND (reviewrequestedat IS NOT NULL AND "+
					"(reviewedat IS NULL OR DATE(reviewedat) < DATE_SUB(NOW(), INTERVAL 31 DAY))) "+
					"AND heldby IS NOT NULL",
					activeGroupIDs).Scan(&heldActive)
				spammembersother += heldActive
			}
			if len(inactiveGroupIDs) > 0 {
				// All spam members in inactive groups → spammembersother (blue).
				var inact int64
				db.Raw("SELECT COUNT(*) FROM memberships "+
					"WHERE groupid IN ? AND (reviewrequestedat IS NOT NULL AND "+
					"(reviewedat IS NULL OR DATE(reviewedat) < DATE_SUB(NOW(), INTERVAL 31 DAY)))",
					inactiveGroupIDs).Scan(&inact)
				spammembersother += inact
			}
		}()

		// --- Pending community events (only active groups) ---
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			if len(activeGroupIDs) > 0 {
				db.Raw("SELECT COUNT(DISTINCT ce.id) FROM communityevents ce "+
					"INNER JOIN communityevents_groups ceg ON ceg.eventid = ce.id "+
					"INNER JOIN communityevents_dates ced ON ced.eventid = ce.id "+
					"WHERE ceg.groupid IN ? AND ce.pending = 1 AND ce.deleted = 0 AND ced.end >= NOW()",
					activeGroupIDs).Scan(&pendingevents)
			}
		}()

		// --- Pending admin applications (only active groups) ---
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			if len(activeGroupIDs) > 0 {
				db.Raw("SELECT COUNT(*) FROM admins WHERE groupid IN ? AND complete IS NULL AND pending = 1 AND heldby IS NULL",
					activeGroupIDs).Scan(&pendingadmins)
			}
		}()

		// --- Edit reviews (only active groups) ---
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			if len(activeGroupIDs) > 0 {
				db.Raw("SELECT COUNT(DISTINCT me.msgid) FROM messages_edits me "+
					"INNER JOIN messages_groups mg ON mg.msgid = me.msgid AND mg.deleted = 0 "+
					"WHERE mg.groupid IN ? AND me.reviewrequired = 1 AND me.approvedat IS NULL AND me.revertedat IS NULL AND me.timestamp > DATE_SUB(NOW(), INTERVAL 7 DAY)",
					activeGroupIDs).Scan(&editreview)
			}
		}()

		// --- Pending volunteering (only active groups) ---
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			if len(activeGroupIDs) > 0 {
				db.Raw("SELECT COUNT(DISTINCT v.id) FROM volunteering v "+
					"INNER JOIN volunteering_groups vg ON vg.volunteeringid = v.id "+
					"LEFT JOIN volunteering_dates vd ON vd.volunteeringid = v.id "+
					"WHERE vg.groupid IN ? AND v.pending = 1 AND v.deleted = 0 AND v.expired = 0 AND (vd.end IS NULL OR vd.end >= NOW())",
					activeGroupIDs).Scan(&pendingvolunteering)
			}
		}()

		// --- Stories (all mod groups, no active/inactive split) ---
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			storyCutoff := time.Now().AddDate(0, 0, -31).Format("2006-01-02")
			db.Raw("SELECT COUNT(DISTINCT us.id) FROM users_stories us "+
				"INNER JOIN memberships m ON m.userid = us.userid "+
				"WHERE m.groupid IN ? AND us.date > ? AND us.reviewed = 0",
				modGroupIDs, storyCutoff).Scan(&stories)
		}()

		// --- Spammer pending counts (system-wide, Admin/Support only) ---
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			if userRow.Systemrole == "Admin" || userRow.Systemrole == "Support" {
				db.Raw("SELECT COUNT(*) FROM spam_users WHERE collection = 'PendingAdd'").Scan(&spammerpendingadd)
				db.Raw("SELECT COUNT(*) FROM spam_users WHERE collection = 'PendingRemove'").Scan(&spammerpendingremove)
			}
		}()

		// --- Chat review: RECIPIENT matching + active/inactive split ---
		// Review counts are based on the RECIPIENT's group membership (not either participant).
		// Active groups: not-held -> chatreview, held -> chatreviewother.
		// Inactive groups: all -> chatreviewother.
		//
		// The chat review SQL uses CASE WHEN to find the recipient:
		//   CASE WHEN cm.userid = cr.user1 THEN cr.user2 ELSE cr.user1 END
		// Primary: recipient IS a member of a Freegle group.
		// Secondary: recipient is NOT a member → use sender's group instead.
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			chatCutoff := time.Now().AddDate(0, 0, -utils.CHAT_ACTIVE_LIMIT).Format("2006-01-02")

			// Helper SQL for recipient-based chat review counting.
			// Count chat messages pending review. Must match the logic in
			// chatmessage_review.go getReviewQueue() so the sidebar count
			// equals the number of displayed messages.
			chatReviewSQL := func(groupIDs []uint64, heldFilter string) int64 {
				if len(groupIDs) == 0 {
					return 0
				}
				var count int64
				db.Raw("SELECT COUNT(DISTINCT cm.id) FROM chat_messages cm "+
					"INNER JOIN chat_rooms cr ON cr.id = cm.chatid "+
					"LEFT JOIN chat_messages_held cmh ON cmh.msgid = cm.id "+
					"WHERE cm.reviewrequired = 1 AND cm.reviewrejected = 0 "+
					"AND cm.date >= ? "+heldFilter+" "+
					"AND ("+
					// User2Mod: chat belongs to one of the mod's groups.
					"  (cr.chattype = ? AND cr.groupid IN ?) "+
					"  OR "+
					// User2User: either participant is a member of one of the mod's groups.
					"  (cr.chattype = ? AND ("+
					"    EXISTS (SELECT 1 FROM memberships m "+
					"      INNER JOIN `groups` g ON m.groupid = g.id AND g.type = 'Freegle' "+
					"      WHERE m.userid = cr.user1 AND m.groupid IN ?) "+
					"    OR EXISTS (SELECT 1 FROM memberships m "+
					"      INNER JOIN `groups` g ON m.groupid = g.id AND g.type = 'Freegle' "+
					"      WHERE m.userid = cr.user2 AND m.groupid IN ?)))"+
					")",
					chatCutoff, utils.CHAT_TYPE_USER2MOD, groupIDs,
					utils.CHAT_TYPE_USER2USER, groupIDs, groupIDs).Scan(&count)
				return count
			}

			// Active groups: not held → chatreview (red), held → chatreviewother (blue).
			chatreview = chatReviewSQL(activeGroupIDs, "AND cmh.userid IS NULL")
			chatreviewother = chatReviewSQL(activeGroupIDs, "AND cmh.userid IS NOT NULL")
			// Inactive groups: all → chatreviewother (blue).
			chatreviewother += chatReviewSQL(inactiveGroupIDs, "AND cmh.userid IS NULL")
			chatreviewother += chatReviewSQL(inactiveGroupIDs, "AND cmh.userid IS NOT NULL")

			// Wider chat review: unheld messages from groups with widerchatreview=1.
			// These go into chatreviewother (blue badge).
			if user.HasWiderReview(myid) {
				var widerCount int64
				db.Raw("SELECT COUNT(DISTINCT cm.id) FROM chat_messages cm "+
					"INNER JOIN chat_rooms cr ON cr.id = cm.chatid "+
					"LEFT JOIN chat_messages_held cmh ON cmh.msgid = cm.id "+
					"INNER JOIN memberships m ON m.userid = (CASE WHEN cm.userid = cr.user1 THEN cr.user2 ELSE cr.user1 END) "+
					"INNER JOIN `groups` g ON m.groupid = g.id AND g.type = 'Freegle' "+
					"WHERE cm.reviewrequired = 1 AND cm.reviewrejected = 0 "+
					"AND cm.date >= ? AND cmh.id IS NULL "+
					"AND JSON_EXTRACT(g.settings, '$.widerchatreview') = 1 "+
					"AND (cm.reportreason IS NULL OR cm.reportreason != 'User')",
					chatCutoff).Scan(&widerCount)
				chatreviewother += widerCount
			}
		}()

		// --- Newsletter stories (global, no group scope) ---
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			db.Raw("SELECT COUNT(*) FROM users_stories "+
				"WHERE reviewed = 1 AND public = 1 AND newsletterreviewed = 0").Scan(&newsletterstories)
		}()

		// --- Gift aid (global) ---
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			db.Raw("SELECT COUNT(*) FROM giftaid WHERE reviewed IS NULL AND deleted IS NULL AND period != 'Declined'").Scan(&giftaid)
		}()

		// --- Happiness (only active groups) ---
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			if len(activeGroupIDs) > 0 {
				hapCutoff := time.Now().AddDate(0, 0, -utils.CHAT_ACTIVE_LIMIT).Format("2006-01-02")
				db.Raw("SELECT COUNT(DISTINCT mo.id) FROM messages_outcomes mo "+
					"INNER JOIN messages_groups mg ON mg.msgid = mo.msgid "+
					"WHERE mo.timestamp >= ? AND mg.arrival >= ? "+
					"AND mg.groupid IN ? "+
					"AND mo.comments IS NOT NULL "+
					"AND mo.comments != 'Sorry, this is no longer available.' "+
					"AND mo.comments != 'Thanks, this has now been taken.' "+
					"AND mo.comments != 'Thanks, I''m no longer looking for this.' "+
					"AND mo.comments != 'Sorry, this has now been taken.' "+
					"AND mo.comments != 'Thanks for the interest, but this has now been taken.' "+
					"AND mo.comments != 'Thanks, these have now been taken.' "+
					"AND mo.comments != 'Thanks, this has now been received.' "+
					"AND mo.comments != 'Withdrawn on user unsubscribe' "+
					"AND mo.comments != 'Auto-Expired' "+
					"AND (mo.happiness = 'Happy' OR mo.happiness IS NULL) "+
					"AND mo.reviewed = 0",
					hapCutoff, hapCutoff, activeGroupIDs).Scan(&happiness)
			}
		}()

		// --- Related members (only active groups) ---
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			if len(activeGroupIDs) > 0 {
				db.Raw("SELECT COUNT(*) FROM ("+
					"SELECT ur.user1 FROM users_related ur "+
					"INNER JOIN memberships m ON m.userid = ur.user1 "+
					"INNER JOIN users u1 ON ur.user1 = u1.id AND u1.deleted IS NULL AND u1.systemrole = 'User' "+
					"INNER JOIN users u2 ON ur.user2 = u2.id AND u2.deleted IS NULL AND u2.systemrole = 'User' "+
					"WHERE ur.user1 < ur.user2 AND ur.notified = 0 AND m.groupid IN ? "+
					"UNION "+
					"SELECT ur.user1 FROM users_related ur "+
					"INNER JOIN memberships m ON m.userid = ur.user2 "+
					"INNER JOIN users u1 ON ur.user1 = u1.id AND u1.deleted IS NULL AND u1.systemrole = 'User' "+
					"INNER JOIN users u2 ON ur.user2 = u2.id AND u2.deleted IS NULL AND u2.systemrole = 'User' "+
					"WHERE ur.user1 < ur.user2 AND ur.notified = 0 AND m.groupid IN ? "+
					") t", activeGroupIDs, activeGroupIDs).Scan(&relatedmembers)
			}
		}()

		wg2.Wait()

		// Total only includes actionable work items (primary/red badge counts),
		// not informational ones (other/blue badge counts like chatreviewother, happiness, giftaid, pendingother).
		total := pending + spam + pendingmembers + spammembers + pendingevents +
			pendingadmins + editreview + pendingvolunteering + stories +
			spammerpendingadd + spammerpendingremove +
			chatreview + newsletterstories + relatedmembers

		work = fiber.Map{
			"pending":              pending,
			"pendingother":         pendingother,
			"spam":                 spam,
			"pendingmembers":       pendingmembers,
			"spammembers":          spammembers,
			"spammembersother":     spammembersother,
			"pendingevents":        pendingevents,
			"pendingadmins":        pendingadmins,
			"editreview":           editreview,
			"pendingvolunteering":  pendingvolunteering,
			"stories":             stories,
			"spammerpendingadd":    spammerpendingadd,
			"spammerpendingremove": spammerpendingremove,
			"chatreview":          chatreview,
			"chatreviewother":     chatreviewother,
			"newsletterstories":   newsletterstories,
			"giftaid":             giftaid,
			"happiness":           happiness,
			"relatedmembers":      relatedmembers,
			"total":               total,
		}
	}

	// Wait for discourse fetch to complete.
	discourseWg.Wait()

	// Fetch location if available (depends on userRow).
	var loc *LocationRow
	if userRow.Lastlocation != nil && *userRow.Lastlocation > 0 {
		var locRow LocationRow
		db.Raw("SELECT name, lat, lng FROM locations WHERE id = ?", *userRow.Lastlocation).Scan(&locRow)
		if locRow.Name != "" {
			loc = &locRow
		}
	}

	// Fetch profile.
	var profile *ProfileRow
	var profRow ProfileRow
	db.Raw("SELECT id, externaluid, externalmods FROM users_images WHERE userid = ? ORDER BY id DESC LIMIT 1", myid).Scan(&profRow)
	if profRow.ID > 0 {
		profile = &profRow
	}

	// Build JWT from session.
	var jwtString string
	if sessionRow.ID > 0 {
		jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"id":        strconv.FormatUint(myid, 10),
			"sessionid": strconv.FormatUint(sessionRow.ID, 10),
			"exp":       time.Now().Unix() + 30*24*60*60,
		})
		secret := os.Getenv("JWT_SECRET")
		var jwtErr error
		jwtString, jwtErr = jwtToken.SignedString([]byte(secret))
		if jwtErr != nil {
			stdlog.Printf("Failed to sign JWT for user %d: %v", myid, jwtErr)
		}
	}

	// Build persistent token.
	var persistent interface{}
	if sessionRow.ID > 0 {
		persistent = fiber.Map{
			"id":     sessionRow.ID,
			"series": sessionRow.Series,
			"token":  sessionRow.Token,
			"userid": myid,
		}
	}

	// Compute displayname from fullname/firstname/lastname (matching GetUserById logic).
	displayname := ""
	if userRow.Fullname != nil && *userRow.Fullname != "" {
		displayname = *userRow.Fullname
	} else {
		if userRow.Firstname != nil {
			displayname = *userRow.Firstname
			if userRow.Lastname != nil {
				displayname += " " + *userRow.Lastname
			}
		} else if userRow.Lastname != nil {
			displayname = *userRow.Lastname
		}
	}
	displayname = utils.TidyName(displayname)

	// Build the me object.
	me := fiber.Map{
		"id":               userRow.ID,
		"displayname":      displayname,
		"fullname":         userRow.Fullname,
		"firstname":        userRow.Firstname,
		"lastname":         userRow.Lastname,
		"systemrole":       userRow.Systemrole,
		"settings":         userRow.Settings,
		"lastaccess":       userRow.Lastaccess,
		"added":            userRow.Added,
		"source":           userRow.Source,
		"deleted":          userRow.Deleted,
		"trustlevel":       userRow.Trustlevel,
		"marketingconsent": userRow.Marketingconsent,
		"bouncing":         userRow.Bouncing,
		"aboutme":          aboutme,
	}

	if userRow.Onholidaytill != nil {
		me["onholidaytill"] = *userRow.Onholidaytill
	}

	if loc != nil {
		me["city"] = loc.Name
		me["lat"] = loc.Lat
		me["lng"] = loc.Lng
	}

	if profile != nil {
		me["profile"] = profile
	}

	// Parse permissions from comma-separated string into array.
	if userRow.Permissions != nil && *userRow.Permissions != "" {
		perms := strings.Split(*userRow.Permissions, ",")
		for i := range perms {
			perms[i] = strings.TrimSpace(perms[i])
		}
		me["permissions"] = perms
	}

	if emails == nil {
		emails = make([]EmailRow, 0)
	}

	// Add primary email to the me object (first non-internal-domain email).
	for _, email := range emails {
		if utils.OurDomain(email.Email) == 0 {
			me["email"] = email.Email
			break
		}
	}

	if memberships == nil {
		memberships = make([]MembershipRow, 0)
	}

	resp := fiber.Map{
		"ret":        0,
		"status":     "Success",
		"me":         me,
		"groups":     memberships,
		"emails":     emails,
		"persistent": persistent,
		"jwt":        jwtString,
	}

	if work != nil {
		resp["work"] = work
	}

	if discourse != nil {
		resp["discourse"] = discourse
	}

	return c.JSON(resp)
}

// PatchSession updates session/user settings for the logged-in user.
//
// @Summary Update session/user settings
// @Tags session
// @Router /session [patch]
func PatchSession(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	type PatchNotifications struct {
		Push *json.RawMessage `json:"push,omitempty"`
	}

	type PatchRequest struct {
		Displayname        *string             `json:"displayname,omitempty"`
		Firstname          *string             `json:"firstname,omitempty"`
		Lastname           *string             `json:"lastname,omitempty"`
		Settings           *json.RawMessage    `json:"settings,omitempty"`
		Password           *string             `json:"password,omitempty"`
		Onholidaytill      *string             `json:"onholidaytill,omitempty"`
		Relevantallowed    *int                `json:"relevantallowed,omitempty"`
		Newslettersallowed *int                `json:"newslettersallowed,omitempty"`
		Aboutme            *string             `json:"aboutme,omitempty"`
		Notifications      *PatchNotifications `json:"notifications,omitempty"`
		Email              *string             `json:"email,omitempty"`
		Source             *string             `json:"source,omitempty"`
		Deleted            *json.RawMessage    `json:"deleted,omitempty"`
		Marketingconsent   *bool               `json:"marketingconsent,omitempty"`
	}

	var req PatchRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	db := database.DBConn

	// Build a single UPDATE for all users table fields to avoid race conditions
	// between concurrent goroutines writing conflicting values to the same row.
	// For example, displayname sets firstname=NULL while a concurrent firstname
	// goroutine sets firstname to a value — the outcome was non-deterministic.
	var setClauses []string
	var setArgs []interface{}

	if req.Displayname != nil {
		setClauses = append(setClauses, "fullname = ?")
		setArgs = append(setArgs, *req.Displayname)
		// Clear first/last unless explicitly provided in the same request.
		if req.Firstname == nil {
			setClauses = append(setClauses, "firstname = NULL")
		}
		if req.Lastname == nil {
			setClauses = append(setClauses, "lastname = NULL")
		}
	}

	if req.Firstname != nil {
		setClauses = append(setClauses, "firstname = ?")
		setArgs = append(setArgs, *req.Firstname)
	}

	if req.Lastname != nil {
		setClauses = append(setClauses, "lastname = ?")
		setArgs = append(setArgs, *req.Lastname)
	}

	if req.Settings != nil {
		setClauses = append(setClauses, "settings = ?")
		setArgs = append(setArgs, string(*req.Settings))
	}

	if req.Onholidaytill != nil {
		setClauses = append(setClauses, "onholidaytill = ?")
		setArgs = append(setArgs, *req.Onholidaytill)
	}

	if req.Relevantallowed != nil {
		setClauses = append(setClauses, "relevantallowed = ?")
		setArgs = append(setArgs, *req.Relevantallowed)
	}

	if req.Newslettersallowed != nil {
		setClauses = append(setClauses, "newslettersallowed = ?")
		setArgs = append(setArgs, *req.Newslettersallowed)
	}

	if req.Source != nil {
		setClauses = append(setClauses, "source = ?")
		setArgs = append(setArgs, *req.Source)
	}

	if req.Deleted != nil {
		rawStr := string(*req.Deleted)
		if rawStr == "null" {
			setClauses = append(setClauses, "deleted = NULL")
		}
	}

	if req.Marketingconsent != nil {
		mc := 0
		if *req.Marketingconsent {
			mc = 1
		}
		setClauses = append(setClauses, "marketingconsent = ?")
		setArgs = append(setArgs, mc)
	}

	// Execute single users table UPDATE if there are any changes.
	if len(setClauses) > 0 {
		setArgs = append(setArgs, myid)
		if result := db.Exec("UPDATE users SET "+strings.Join(setClauses, ", ")+" WHERE id = ?", setArgs...); result.Error != nil {
			stdlog.Printf("Failed to update user %d: %v", myid, result.Error)
		}
	}

	// Run non-users-table operations in parallel (different tables, no conflicts).
	var wg sync.WaitGroup

	if req.Password != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			salt := auth.GetPasswordSalt()
			hashed := auth.HashPassword(*req.Password, salt)
			uid := strconv.FormatUint(myid, 10)
			db.Exec("INSERT INTO users_logins (userid, type, uid, credentials, salt) VALUES (?, 'Native', ?, ?, ?) "+
				"ON DUPLICATE KEY UPDATE credentials = ?, salt = ?",
				myid, uid, hashed, salt, hashed, salt)
		}()
	}

	if req.Aboutme != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db.Exec("INSERT INTO users_aboutme (userid, text, timestamp) VALUES (?, ?, NOW())", myid, *req.Aboutme)
		}()
	}

	if req.Notifications != nil && req.Notifications.Push != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			type PushSub struct {
				Type         string `json:"type"`
				Subscription string `json:"subscription"`
			}
			var pushSub PushSub
			if err := json.Unmarshal(*req.Notifications.Push, &pushSub); err == nil && pushSub.Type != "" {
				db.Exec("INSERT INTO users_push_notifications (userid, type, subscription) VALUES (?, ?, ?) "+
					"ON DUPLICATE KEY UPDATE subscription = ?",
					myid, pushSub.Type, pushSub.Subscription, pushSub.Subscription)
			}
		}()
	}

	if req.Email != nil && *req.Email != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := queue.QueueTask(queue.TaskEmailVerify, map[string]interface{}{
				"user_id": myid,
				"email":   *req.Email,
			}); err != nil {
				stdlog.Printf("Failed to queue email verify for user %d: %v", myid, err)
			}
		}()
	}

	wg.Wait()

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
	})
}

// DeleteSession logs the user out by destroying their session.
//
// @Summary Logout
// @Tags session
// @Router /session [delete]
func DeleteSession(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	// Signal the auth middleware to skip the post-handler session check.
	// Without this, there's a race condition: the middleware's goroutine checks
	// that the session exists in DB, but the handler deletes the session before
	// the goroutine completes, causing a spurious 401.
	c.Locals("skipPostAuthCheck", true)

	if myid > 0 {
		db := database.DBConn
		db.Exec("DELETE FROM sessions WHERE userid = ?", myid)
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
	})
}
