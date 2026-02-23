package session

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/queue"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
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

// PostSessionRequest covers all fields used across session POST actions.
type PostSessionRequest struct {
	Action   string   `json:"action"`
	Email    string   `json:"email"`
	Password string   `json:"password"`
	U        uint64   `json:"u"`
	K        string   `json:"k"`
	Userlist []uint64 `json:"userlist"`
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
		return handleForget(c)
	case "Related":
		return handleRelated(c, req.Userlist)
	default:
		// No action means login attempt.
		if req.Email != "" && req.Password != "" {
			return handleEmailPasswordLogin(c, req.Email, req.Password)
		}
		if req.U > 0 && req.K != "" {
			return handleLinkLogin(c, req.U, req.K)
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
		// PHP returns ret=2 for unknown email. Match that behaviour.
		return c.JSON(fiber.Map{
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
		log.Printf("Failed to queue forgot-password email for user %d: %v", userID, err)
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
		log.Printf("Failed to queue unsubscribe email for user %d: %v", userID, err)
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

// createSessionAndJWT creates a sessions row and returns the persistent token data and a JWT.
func createSessionAndJWT(userID uint64) (map[string]interface{}, string, error) {
	db := database.DBConn

	series := utils.RandomHex(16)
	token := utils.RandomHex(16)

	db.Exec("INSERT INTO sessions (userid, series, token, date, lastactive) VALUES (?, ?, ?, NOW(), NOW())",
		userID, series, token)

	var sessionID uint64
	db.Raw("SELECT id FROM sessions WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&sessionID)

	if sessionID == 0 {
		return nil, "", fmt.Errorf("failed to create session")
	}

	persistent := map[string]interface{}{
		"id":     sessionID,
		"series": series,
		"token":  token,
		"userid": userID,
	}

	// Generate JWT.
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":        strconv.FormatUint(userID, 10),
		"sessionid": strconv.FormatUint(sessionID, 10),
		"exp":       time.Now().Unix() + 30*24*60*60, // 30 days
	})

	secret := os.Getenv("JWT_SECRET")
	jwtString, err := jwtToken.SignedString([]byte(secret))
	if err != nil {
		return nil, "", err
	}

	return persistent, jwtString, nil
}

// handleEmailPasswordLogin authenticates via email and bcrypt password.
func handleEmailPasswordLogin(c *fiber.Ctx, email string, password string) error {
	db := database.DBConn

	// Find user by email (must not be deleted).
	var userID uint64
	db.Raw("SELECT u.id FROM users u "+
		"JOIN users_emails ue ON ue.userid = u.id "+
		"WHERE ue.email = ? AND u.deleted IS NULL "+
		"LIMIT 1", email).Scan(&userID)

	if userID == 0 {
		return c.JSON(fiber.Map{
			"ret":    2,
			"status": "We don't know that email address.",
		})
	}

	// Verify password.
	var hashedPassword string
	db.Raw("SELECT credentials FROM users_logins WHERE userid = ? AND type = 'Native' LIMIT 1", userID).Scan(&hashedPassword)

	if hashedPassword == "" || bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password)) != nil {
		return c.JSON(fiber.Map{
			"ret":    3,
			"status": "The password is wrong.",
		})
	}

	persistent, jwtString, err := createSessionAndJWT(userID)
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
		return c.JSON(fiber.Map{
			"ret":    2,
			"status": "Unknown user.",
		})
	}

	// Verify the link key.
	var storedKey string
	db.Raw("SELECT credentials FROM users_logins WHERE userid = ? AND type = 'Link' LIMIT 1", uid).Scan(&storedKey)

	if storedKey == "" || subtle.ConstantTimeCompare([]byte(storedKey), []byte(key)) != 1 {
		return c.JSON(fiber.Map{
			"ret":    3,
			"status": "Invalid key.",
		})
	}

	persistent, jwtString, err := createSessionAndJWT(uid)
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

// handleForget marks the current user's account as deleted ("forget me").
func handleForget(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	// Check user is not a moderator.
	var modRole string
	db.Raw("SELECT role FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner') LIMIT 1", myid).Scan(&modRole)

	if modRole != "" {
		return c.JSON(fiber.Map{
			"ret":    2,
			"status": "Please demote yourself to a member first",
		})
	}

	// Signal the auth middleware to skip the post-handler session check.
	c.Locals("skipPostAuthCheck", true)

	// Set user as deleted.
	db.Exec("UPDATE users SET deleted = NOW() WHERE id = ?", myid)

	// Destroy session.
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

// GetSession returns current session info for the logged-in user.
//
// @Summary Get current session
// @Tags session
// @Router /session [get]
func GetSession(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{
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
		Trustlevel    *string         `json:"trustlevel"`
		Permissions   *string         `json:"permissions"`
	}

	type EmailRow struct {
		ID        uint64 `json:"id"`
		Email     string `json:"email"`
		Preferred int    `json:"preferred"`
		Validated *int   `json:"validated"`
	}

	type MembershipRow struct {
		Groupid             uint64  `json:"groupid"`
		Role                string  `json:"role"`
		Emailfrequency      int     `json:"emailfrequency"`
		Eventsallowed       int     `json:"eventsallowed"`
		Volunteeringallowed int     `json:"volunteeringallowed"`
		Configid            *uint64 `json:"configid"`
		Nameshort           string  `json:"nameshort"`
		Namefull            string  `json:"-"`
		Namedisplay         string  `json:"namedisplay" gorm:"-"`
		Type                string  `json:"type"`
		Region              string  `json:"region"`
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

	var wg sync.WaitGroup
	var userRow UserRow
	var emails []EmailRow
	var memberships []MembershipRow
	var sessionRow SessionRow

	wg.Add(4)
	go func() {
		defer wg.Done()
		db.Raw("SELECT id, fullname, firstname, lastname, systemrole, settings, lastaccess, added, lastlocation, onholidaytill, source, deleted, trustlevel, permissions FROM users WHERE id = ?", myid).Scan(&userRow)
	}()
	go func() {
		defer wg.Done()
		db.Raw("SELECT id, email, preferred, validated FROM users_emails WHERE userid = ? ORDER BY preferred DESC", myid).Scan(&emails)
	}()
	go func() {
		defer wg.Done()
		db.Raw("SELECT m.groupid, m.role, m.emailfrequency, m.eventsallowed, m.volunteeringallowed, m.configid, g.nameshort, g.namefull, g.type, g.region "+
			"FROM memberships m JOIN `groups` g ON g.id = m.groupid "+
			"WHERE m.userid = ? AND m.collection = 'Approved' ORDER BY COALESCE(NULLIF(g.namefull, ''), g.nameshort)", myid).Scan(&memberships)
	}()
	go func() {
		defer wg.Done()
		db.Raw("SELECT id, series, token FROM sessions WHERE userid = ? LIMIT 1", myid).Scan(&sessionRow)
	}()
	wg.Wait()

	// Compute namedisplay from namefull/nameshort (namedisplay is not a real DB column).
	for i := range memberships {
		if memberships[i].Namefull != "" {
			memberships[i].Namedisplay = memberships[i].Namefull
		} else {
			memberships[i].Namedisplay = memberships[i].Nameshort
		}
	}

	// Compute work counts and discourse stats for moderators (depends on memberships).
	var work fiber.Map
	var discourse fiber.Map

	// Collect group IDs where user is a moderator or owner.
	var modGroupIDs []uint64
	isFreegleMod := false
	for _, m := range memberships {
		if m.Role == "Owner" || m.Role == "Moderator" {
			modGroupIDs = append(modGroupIDs, m.Groupid)
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
		var pending, spam, pendingmembers, spammembers, pendingevents, pendingadmins, editreview int64
		var pendingvolunteering, spammerpendingadd, spammerpendingremove, stories int64

		var wg2 sync.WaitGroup

		wg2.Add(1)
		go func() {
			defer wg2.Done()
			// Pending messages across moderated groups.
			db.Raw("SELECT COUNT(*) FROM messages_groups mg "+
				"INNER JOIN messages m ON m.id = mg.msgid "+
				"WHERE mg.groupid IN ? AND mg.collection = 'Pending' AND mg.deleted = 0 AND m.fromuser IS NOT NULL",
				modGroupIDs).Scan(&pending)
			// Spam messages across moderated groups.
			db.Raw("SELECT COUNT(*) FROM messages_groups mg "+
				"INNER JOIN messages m ON m.id = mg.msgid "+
				"WHERE mg.groupid IN ? AND mg.collection = 'Spam' AND mg.deleted = 0 AND m.fromuser IS NOT NULL",
				modGroupIDs).Scan(&spam)
		}()
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			// Pending members.
			db.Raw("SELECT COUNT(*) FROM memberships WHERE groupid IN ? AND collection = 'Pending'",
				modGroupIDs).Scan(&pendingmembers)
			// Spam members.
			db.Raw("SELECT COUNT(*) FROM memberships WHERE groupid IN ? AND collection = 'Spam'",
				modGroupIDs).Scan(&spammembers)
		}()
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			// Pending community events.
			db.Raw("SELECT COUNT(DISTINCT ce.id) FROM communityevents ce "+
				"INNER JOIN communityevents_groups ceg ON ceg.eventid = ce.id "+
				"INNER JOIN communityevents_dates ced ON ced.eventid = ce.id "+
				"WHERE ceg.groupid IN ? AND ce.pending = 1 AND ce.deleted = 0 AND ced.end >= NOW()",
				modGroupIDs).Scan(&pendingevents)
		}()
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			// Pending admin applications.
			db.Raw("SELECT COUNT(*) FROM admins WHERE groupid IN ? AND complete IS NULL AND pending = 1 AND heldby IS NULL",
				modGroupIDs).Scan(&pendingadmins)
		}()
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			// Edit reviews.
			db.Raw("SELECT COUNT(*) FROM messages_edits me "+
				"INNER JOIN messages_groups mg ON mg.msgid = me.msgid "+
				"WHERE mg.groupid IN ? AND me.reviewrequired = 1 AND me.timestamp > DATE_SUB(NOW(), INTERVAL 7 DAY)",
				modGroupIDs).Scan(&editreview)
		}()
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			// Pending volunteering scoped to moderator's groups.
			db.Raw("SELECT COUNT(DISTINCT v.id) FROM volunteering v "+
				"INNER JOIN volunteering_groups vg ON vg.volunteeringid = v.id "+
				"INNER JOIN volunteering_dates vd ON vd.volunteeringid = v.id "+
				"WHERE vg.groupid IN ? AND v.pending = 1 AND v.deleted = 0 AND v.expired = 0 AND vd.end >= NOW()",
				modGroupIDs).Scan(&pendingvolunteering)
		}()
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			// Stories pending review.
			db.Raw("SELECT COUNT(*) FROM users_stories WHERE reviewed = 0 AND deleted = 0").Scan(&stories)
		}()
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			// Spammer pending counts (system-wide, for Admin/Support users).
			if userRow.Systemrole == "Admin" || userRow.Systemrole == "Support" {
				db.Raw("SELECT COUNT(*) FROM spam_users WHERE collection = 'PendingAdd'").Scan(&spammerpendingadd)
				db.Raw("SELECT COUNT(*) FROM spam_users WHERE collection = 'PendingRemove'").Scan(&spammerpendingremove)
			}
		}()

		wg2.Wait()

		total := pending + spam + pendingmembers + spammembers + pendingevents +
			pendingadmins + editreview + pendingvolunteering + stories +
			spammerpendingadd + spammerpendingremove

		work = fiber.Map{
			"pending":              pending,
			"spam":                 spam,
			"pendingmembers":       pendingmembers,
			"spammembers":          spammembers,
			"pendingevents":        pendingevents,
			"pendingadmins":        pendingadmins,
			"editreview":           editreview,
			"pendingvolunteering":  pendingvolunteering,
			"stories":             stories,
			"spammerpendingadd":    spammerpendingadd,
			"spammerpendingremove": spammerpendingremove,
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
			log.Printf("Failed to sign JWT for user %d: %v", myid, jwtErr)
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

	// Build the me object.
	me := fiber.Map{
		"id":         userRow.ID,
		"fullname":   userRow.Fullname,
		"firstname":  userRow.Firstname,
		"lastname":   userRow.Lastname,
		"systemrole": userRow.Systemrole,
		"settings":   userRow.Settings,
		"lastaccess": userRow.Lastaccess,
		"added":      userRow.Added,
		"source":     userRow.Source,
		"deleted":    userRow.Deleted,
		"trustlevel": userRow.Trustlevel,
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
			log.Printf("Failed to update user %d: %v", myid, result.Error)
		}
	}

	// Run non-users-table operations in parallel (different tables, no conflicts).
	var wg sync.WaitGroup

	if req.Password != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hashedPassword, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
			if err != nil {
				log.Printf("Failed to hash password for user %d: %v", myid, err)
				return
			}
			uid := strconv.FormatUint(myid, 10)
			db.Exec("INSERT INTO users_logins (userid, type, uid, credentials) VALUES (?, 'Native', ?, ?) "+
				"ON DUPLICATE KEY UPDATE credentials = ?",
				myid, uid, string(hashedPassword), string(hashedPassword))
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
				log.Printf("Failed to queue email verify for user %d: %v", myid, err)
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
