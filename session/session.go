package session

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
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
	queue.QueueTask(queue.TaskEmailForgotPassword, map[string]interface{}{
		"user_id":   userID,
		"email":     preferredEmail,
		"reset_url": resetURL,
	})

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
	queue.QueueTask(queue.TaskEmailUnsubscribe, map[string]interface{}{
		"user_id":    userID,
		"email":      preferredEmail,
		"unsub_url":  unsubURL,
	})

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
	}

	type EmailRow struct {
		ID        uint64 `json:"id"`
		Email     string `json:"email"`
		Preferred int    `json:"preferred"`
		Validated *int   `json:"validated"`
	}

	type MembershipRow struct {
		Groupid             uint64 `json:"groupid"`
		Role                string `json:"role"`
		Emailfrequency      int    `json:"emailfrequency"`
		Eventsallowed       int    `json:"eventsallowed"`
		Volunteeringallowed int    `json:"volunteeringallowed"`
		Nameshort           string `json:"nameshort"`
		Namefull            string `json:"namefull"`
		Namedisplay         string `json:"namedisplay" gorm:"-"`
		Type                string `json:"type"`
		Region              string `json:"region"`
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
		db.Raw("SELECT id, fullname, firstname, lastname, systemrole, settings, lastaccess, added, lastlocation, onholidaytill, source, deleted, trustlevel FROM users WHERE id = ?", myid).Scan(&userRow)
	}()
	go func() {
		defer wg.Done()
		db.Raw("SELECT id, email, preferred, validated FROM users_emails WHERE userid = ? ORDER BY preferred DESC", myid).Scan(&emails)
	}()
	go func() {
		defer wg.Done()
		db.Raw("SELECT m.groupid, m.role, m.emailfrequency, m.eventsallowed, m.volunteeringallowed, g.nameshort, g.namefull, g.type, g.region "+
			"FROM memberships m JOIN `groups` g ON g.id = m.groupid "+
			"WHERE m.userid = ? AND m.collection = 'Approved' ORDER BY COALESCE(g.namefull, g.nameshort)", myid).Scan(&memberships)
	}()
	go func() {
		defer wg.Done()
		db.Raw("SELECT id, series, token FROM sessions WHERE userid = ? LIMIT 1", myid).Scan(&sessionRow)
	}()
	wg.Wait()

	// Compute namedisplay from namefull/nameshort.
	for ix, m := range memberships {
		if len(m.Namefull) > 0 {
			memberships[ix].Namedisplay = m.Namefull
		} else {
			memberships[ix].Namedisplay = m.Nameshort
		}
	}

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
		jwtString, _ = jwtToken.SignedString([]byte(secret))
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

	if emails == nil {
		emails = make([]EmailRow, 0)
	}

	if memberships == nil {
		memberships = make([]MembershipRow, 0)
	}

	return c.JSON(fiber.Map{
		"ret":        0,
		"status":     "Success",
		"me":         me,
		"groups":     memberships,
		"emails":     emails,
		"persistent": persistent,
		"jwt":        jwtString,
	})
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

	if req.Displayname != nil {
		db.Exec("UPDATE users SET fullname = ?, firstname = NULL, lastname = NULL WHERE id = ?", *req.Displayname, myid)
	}

	if req.Firstname != nil {
		db.Exec("UPDATE users SET firstname = ? WHERE id = ?", *req.Firstname, myid)
	}

	if req.Lastname != nil {
		db.Exec("UPDATE users SET lastname = ? WHERE id = ?", *req.Lastname, myid)
	}

	if req.Settings != nil {
		db.Exec("UPDATE users SET settings = ? WHERE id = ?", string(*req.Settings), myid)
	}

	if req.Password != nil {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to hash password")
		}
		uid := strconv.FormatUint(myid, 10)
		db.Exec("INSERT INTO users_logins (userid, type, uid, credentials) VALUES (?, 'Native', ?, ?) "+
			"ON DUPLICATE KEY UPDATE credentials = ?",
			myid, uid, string(hashedPassword), string(hashedPassword))
	}

	if req.Onholidaytill != nil {
		db.Exec("UPDATE users SET onholidaytill = ? WHERE id = ?", *req.Onholidaytill, myid)
	}

	if req.Relevantallowed != nil {
		db.Exec("UPDATE users SET relevantallowed = ? WHERE id = ?", *req.Relevantallowed, myid)
	}

	if req.Newslettersallowed != nil {
		db.Exec("UPDATE users SET newslettersallowed = ? WHERE id = ?", *req.Newslettersallowed, myid)
	}

	if req.Aboutme != nil {
		db.Exec("INSERT INTO users_aboutme (userid, text, timestamp) VALUES (?, ?, NOW())", myid, *req.Aboutme)
	}

	if req.Notifications != nil && req.Notifications.Push != nil {
		// Parse the push subscription to determine type.
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
	}

	if req.Email != nil && *req.Email != "" {
		queue.QueueTask(queue.TaskEmailVerify, map[string]interface{}{
			"user_id": myid,
			"email":   *req.Email,
		})
	}

	if req.Source != nil {
		db.Exec("UPDATE users SET source = ? WHERE id = ?", *req.Source, myid)
	}

	if req.Deleted != nil {
		// A null value for deleted means restore account.
		rawStr := string(*req.Deleted)
		if rawStr == "null" {
			db.Exec("UPDATE users SET deleted = NULL WHERE id = ?", myid)
		}
	}

	if req.Marketingconsent != nil {
		mc := 0
		if *req.Marketingconsent {
			mc = 1
		}
		db.Exec("UPDATE users SET marketingconsent = ? WHERE id = ?", mc, myid)
	}

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

	if myid > 0 {
		db := database.DBConn

		// Signal the auth middleware to skip the post-handler session check.
		// Without this, there's a race condition: the middleware's goroutine checks
		// that the session exists in DB, but the handler deletes the session before
		// the goroutine completes, causing a spurious 401.
		c.Locals("skipPostAuthCheck", true)

		db.Exec("DELETE FROM sessions WHERE userid = ?", myid)
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
	})
}
