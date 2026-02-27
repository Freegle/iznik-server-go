package user

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/freegle/iznik-server-go/auth"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/queue"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
)

// UserPutRequest is the body for PUT /user (signup).
type UserPutRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	Firstname   string `json:"firstname"`
	Lastname    string `json:"lastname"`
	Displayname string `json:"displayname"`
	GroupID     uint64 `json:"groupid"`
}

// UserPatchRequest is the body for PATCH /user (profile update).
type UserPatchRequest struct {
	ID                  uint64           `json:"id"`
	Displayname         *string          `json:"displayname,omitempty"`
	Settings            *json.RawMessage `json:"settings,omitempty"`
	Onholidaytill       *string          `json:"onholidaytill,omitempty"`
	Relevantallowed     *int             `json:"relevantallowed,omitempty"`
	Newslettersallowed  *int             `json:"newslettersallowed,omitempty"`
	Aboutme             *string          `json:"aboutme,omitempty"`
	Newsfeedmodstatus   *string          `json:"newsfeedmodstatus,omitempty"`
	Email               *string          `json:"email,omitempty"`
	Source              *string          `json:"source,omitempty"`
}

// UserDeleteRequest is the body for DELETE /user.
type UserDeleteRequest struct {
	ID uint64 `json:"id"`
}

// PutUser creates a new user (signup).
//
// @Summary Create/signup a new user
// @Tags user
// @Accept json
// @Produce json
// @Param body body UserPutRequest true "Signup details"
// @Success 200 {object} map[string]interface{}
// @Router /user [put]
func PutUser(c *fiber.Ctx) error {
	var req UserPutRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "email is required")
	}

	email := strings.TrimSpace(req.Email)
	db := database.DBConn

	// Check if email already exists.
	var existingUID uint64
	db.Raw("SELECT userid FROM users_emails WHERE email = ? LIMIT 1", email).Scan(&existingUID)

	if existingUID > 0 {
		// If they provided a correct password, treat signup as login — avoids
		// forcing users to switch to the login screen and re-enter credentials.
		if req.Password != "" && auth.VerifyPassword(existingUID, req.Password) {
			persistent, jwtString, err := auth.CreateSessionAndJWT(existingUID)
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "Failed to create session")
			}
			return c.JSON(fiber.Map{
				"ret":        0,
				"status":     "Success",
				"id":         existingUID,
				"persistent": persistent,
				"jwt":        jwtString,
			})
		}

		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"ret":    2,
			"status": "That email is already in use",
		})
	}

	// Build display name from parts.
	fullname := strings.TrimSpace(req.Displayname)
	if fullname == "" {
		parts := []string{}
		if req.Firstname != "" {
			parts = append(parts, req.Firstname)
		}
		if req.Lastname != "" {
			parts = append(parts, req.Lastname)
		}
		fullname = strings.Join(parts, " ")
	}

	var firstname *string
	var lastname *string
	if req.Firstname != "" {
		firstname = &req.Firstname
	}
	if req.Lastname != "" {
		lastname = &req.Lastname
	}

	// Create user.
	result := db.Exec("INSERT INTO users (fullname, firstname, lastname, added) VALUES (?, ?, ?, NOW())",
		fullname, firstname, lastname)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create user")
	}

	// LAST_INSERT_ID() is per-connection and safe for sequential calls.
	// No better alternative exists here since the email hasn't been inserted yet.
	var newUserID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&newUserID)

	if newUserID == 0 {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to get new user ID")
	}

	// Add email.
	canon := CanonicalizeEmail(email)
	db.Exec("INSERT INTO users_emails (userid, email, preferred, validated, canon) VALUES (?, ?, 1, NOW(), ?)",
		newUserID, email, canon)

	// Generate random password if none provided (matches PHP behavior for email-only signup).
	// The client shows this to the user in the welcome modal.
	password := req.Password
	if password == "" {
		password = utils.RandomHex(4) // 8 char random hex password
	}

	// Hash with sha1+salt (matching PHP) and store.
	salt := os.Getenv("PASSWORD_SALT")
	if salt == "" {
		salt = "zzzz"
	}
	h := sha1.New()
	h.Write([]byte(password + salt))
	hashed := hex.EncodeToString(h.Sum(nil))
	db.Exec("INSERT INTO users_logins (userid, type, uid, credentials, salt) VALUES (?, 'Native', ?, ?, ?)",
		newUserID, email, hashed, salt)

	// If groupid provided, add membership.
	if req.GroupID > 0 {
		db.Exec("INSERT INTO memberships (userid, groupid, role, collection) VALUES (?, ?, 'Member', 'Approved')",
			newUserID, req.GroupID)
	}

	// Create a session. Series is a numeric value; token is a random string.
	token := utils.RandomHex(16)
	db.Exec("INSERT INTO sessions (userid, series, token, lastactive) VALUES (?, ?, ?, NOW())",
		newUserID, newUserID, token)

	var sessionID uint64
	db.Raw("SELECT id FROM sessions WHERE userid = ? ORDER BY id DESC LIMIT 1", newUserID).Scan(&sessionID)

	// Generate JWT.
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":        fmt.Sprint(newUserID),
		"sessionid": fmt.Sprint(sessionID),
		"exp":       time.Now().Unix() + 30*24*60*60, // 30 days
	})

	jwtString, err := jwtToken.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to generate JWT")
	}

	resp := fiber.Map{
		"ret":    0,
		"status": "Success",
		"id":     newUserID,
		"persistent": fiber.Map{
			"id":     sessionID,
			"series": newUserID,
			"token":  token,
			"userid": newUserID,
		},
		"jwt": jwtString,
	}

	// Return the generated password so the client can show it in the welcome modal.
	if req.Password == "" {
		resp["password"] = password
	}

	return c.JSON(resp)
}

// PatchUser updates user profile fields.
//
// @Summary Update user profile
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /user [patch]
func PatchUser(c *fiber.Ctx) error {
	myid := WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req UserPatchRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	db := database.DBConn

	// Handle newsfeedmodstatus for another user (mod action).
	if req.Newsfeedmodstatus != nil && req.ID > 0 && req.ID != myid {
		// Verify caller is admin/support or mod of a shared group.
		var isSupport bool
		db.Raw("SELECT systemrole IN ('Support', 'Admin') FROM users WHERE id = ?", myid).Scan(&isSupport)

		if !isSupport {
			// Check if they share a group where the caller is a mod.
			var sharedModGroup int64
			db.Raw("SELECT COUNT(*) FROM memberships m1 "+
				"INNER JOIN memberships m2 ON m1.groupid = m2.groupid "+
				"WHERE m1.userid = ? AND m2.userid = ? AND m1.role IN ('Owner', 'Moderator')",
				myid, req.ID).Scan(&sharedModGroup)

			if sharedModGroup == 0 {
				return fiber.NewError(fiber.StatusForbidden, "Not authorized to moderate this user")
			}
		}

		db.Exec("UPDATE users SET newsfeedmodstatus = ? WHERE id = ?", *req.Newsfeedmodstatus, req.ID)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
	}

	// All other updates apply to the logged-in user.
	if req.Displayname != nil {
		db.Exec("UPDATE users SET fullname = ?, firstname = NULL, lastname = NULL WHERE id = ?",
			*req.Displayname, myid)
	}

	if req.Settings != nil {
		settingsJSON, err := json.Marshal(req.Settings)
		if err == nil {
			db.Exec("UPDATE users SET settings = ? WHERE id = ?", string(settingsJSON), myid)
		}
	}

	if req.Onholidaytill != nil {
		if *req.Onholidaytill == "" {
			db.Exec("UPDATE users SET onholidaytill = NULL WHERE id = ?", myid)
		} else {
			db.Exec("UPDATE users SET onholidaytill = ? WHERE id = ?", *req.Onholidaytill, myid)
		}
	}

	if req.Relevantallowed != nil {
		db.Exec("UPDATE users SET relevantallowed = ? WHERE id = ?", *req.Relevantallowed, myid)
	}

	if req.Newslettersallowed != nil {
		db.Exec("UPDATE users SET newslettersallowed = ? WHERE id = ?", *req.Newslettersallowed, myid)
	}

	if req.Aboutme != nil {
		// Insert a new aboutme entry. The most recent is fetched via ORDER BY timestamp DESC LIMIT 1.
		db.Exec("INSERT INTO users_aboutme (userid, text, timestamp) VALUES (?, ?, NOW())", myid, *req.Aboutme)
	}

	if req.Newsfeedmodstatus != nil {
		// Self-update (no req.ID or req.ID == myid).
		db.Exec("UPDATE users SET newsfeedmodstatus = ? WHERE id = ?", *req.Newsfeedmodstatus, myid)
	}

	if req.Email != nil && *req.Email != "" {
		// Queue email verification rather than adding directly.
		// New addresses must be verified before being linked to the account.
		if err := queue.QueueTask(queue.TaskEmailVerify, map[string]interface{}{
			"user_id": myid,
			"email":   strings.TrimSpace(*req.Email),
		}); err != nil {
			// Log but don't fail the whole request.
			fmt.Printf("Failed to queue email verify for user %d: %v\n", myid, err)
		}
	}

	if req.Source != nil {
		db.Exec("UPDATE users SET source = ? WHERE id = ?", *req.Source, myid)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// DeleteUser purges/deletes a user.
//
// @Summary Delete/purge a user
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /user [delete]
func DeleteUser(c *fiber.Ctx) error {
	myid := WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	// Parse the target user ID from body or query.
	var req UserDeleteRequest
	_ = c.BodyParser(&req) // Ignore parse errors - body is optional, query param fallback below.

	if req.ID == 0 {
		// Try query parameter.
		if idStr := c.Query("id"); idStr != "" {
			fmt.Sscanf(idStr, "%d", &req.ID)
		}
	}

	targetID := req.ID
	if targetID == 0 {
		// Self-delete.
		targetID = myid
	}

	if targetID != myid {
		// Deleting another user requires admin/support.
		var systemrole string
		db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)

		if systemrole != "Admin" && systemrole != "Support" {
			return fiber.NewError(fiber.StatusForbidden, "Only admin/support can delete other users")
		}

		// Cannot delete moderators/owners — they must demote themselves first.
		var targetModRole string
		db.Raw("SELECT role FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner') LIMIT 1", targetID).Scan(&targetModRole)

		if targetModRole != "" {
			return fiber.NewError(fiber.StatusForbidden, "Cannot delete a moderator/owner — they must demote first")
		}
	} else {
		// Self-delete checks: moderators must demote first, spammers cannot self-delete.
		var modRole string
		db.Raw("SELECT role FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner') LIMIT 1", myid).Scan(&modRole)

		if modRole != "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"ret":    2,
				"status": "Please demote yourself to a member first",
			})
		}

		var spammerCount int64
		db.Raw("SELECT COUNT(*) FROM spam_users WHERE userid = ? AND collection IN ('Spammer', 'PendingAdd')", myid).Scan(&spammerCount)

		if spammerCount > 0 {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"ret":    3,
				"status": "We can't do this.",
			})
		}
	}

	db.Exec("UPDATE users SET deleted = NOW() WHERE id = ?", targetID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleUnbounce resets the bouncing flag on a user. Admin/Support only.
func handleUnbounce(c *fiber.Ctx, myid uint64, req UserPostRequest) error {
	db := database.DBConn

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	// Require admin/support.
	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)

	if systemrole != "Admin" && systemrole != "Support" {
		return fiber.NewError(fiber.StatusForbidden, "Only admin/support can unbounce users")
	}

	db.Exec("UPDATE users SET bouncing = 0 WHERE id = ?", req.ID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleMerge merges user id2 into user id1. Admin/Support only.
func handleMerge(c *fiber.Ctx, myid uint64, req UserPostRequest) error {
	db := database.DBConn

	if req.ID1 == 0 || req.ID2 == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id1 and id2 are required")
	}

	if req.ID1 == req.ID2 {
		return fiber.NewError(fiber.StatusBadRequest, "Cannot merge a user with themselves")
	}

	// Require admin/support.
	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)

	if systemrole != "Admin" && systemrole != "Support" {
		return fiber.NewError(fiber.StatusForbidden, "Only admin/support can merge users")
	}

	// Move references from id2 to id1 - run independent writes in parallel.
	var wg sync.WaitGroup
	wg.Add(5)

	go func() {
		defer wg.Done()
		db.Exec("UPDATE messages SET fromuser = ? WHERE fromuser = ?", req.ID1, req.ID2)
	}()
	go func() {
		defer wg.Done()
		db.Exec("UPDATE chat_rooms SET user1 = ? WHERE user1 = ?", req.ID1, req.ID2)
	}()
	go func() {
		defer wg.Done()
		db.Exec("UPDATE chat_rooms SET user2 = ? WHERE user2 = ?", req.ID1, req.ID2)
	}()
	go func() {
		defer wg.Done()
		db.Exec("UPDATE chat_messages SET userid = ? WHERE userid = ?", req.ID1, req.ID2)
	}()
	go func() {
		defer wg.Done()
		db.Exec("UPDATE users_emails SET userid = ? WHERE userid = ?", req.ID1, req.ID2)
	}()

	wg.Wait()

	// Memberships must be sequential: move non-duplicates, then delete remaining, then mark deleted.
	db.Exec("UPDATE IGNORE memberships SET userid = ? WHERE userid = ?", req.ID1, req.ID2)
	db.Exec("DELETE FROM memberships WHERE userid = ?", req.ID2)
	db.Exec("UPDATE users SET deleted = NOW() WHERE id = ?", req.ID2)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
