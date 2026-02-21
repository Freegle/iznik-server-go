package merge

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/queue"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)


// obfuscateEmail replaces the middle characters of the local part with stars.
// e.g. "test@example.com" -> "t***@example.com"
func obfuscateEmail(email string) string {
	if email == "" {
		return ""
	}

	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return email
	}

	local := parts[0]
	domain := parts[1]

	if len(local) <= 1 {
		return local + "***@" + domain
	}

	return string(local[0]) + strings.Repeat("*", len(local)-1) + "@" + domain
}

// generateUID generates a random 32-character hex string.
func generateUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// GetMerge handles GET /merge - fetch merge by id and uid (public access with uid).
//
// @Summary Get merge by ID and UID
// @Tags merge
// @Produce json
// @Param id query integer true "Merge ID"
// @Param uid query string true "Merge UID (secret link)"
// @Success 200 {object} map[string]interface{}
// @Router /api/merge [get]
func GetMerge(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)
	uid := c.Query("uid", "")

	if id == 0 || uid == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Missing id or uid")
	}

	db := database.DBConn

	type MergeRow struct {
		ID       uint64  `json:"id"`
		User1    uint64  `json:"user1"`
		User2    uint64  `json:"user2"`
		UID      string  `json:"uid"`
		Accepted *string `json:"accepted"`
		Rejected *string `json:"rejected"`
	}

	var m MergeRow
	db.Raw("SELECT id, user1, user2, uid, accepted, rejected FROM merges WHERE id = ? AND uid = ?", id, uid).Scan(&m)

	if m.ID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Not found")
	}

	// Get user info for both users in parallel.
	var name1, email1, name2, email2 string
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		db.Raw("SELECT COALESCE(fullname, 'A freegler') FROM users WHERE id = ?", m.User1).Scan(&name1)
	}()
	go func() {
		defer wg.Done()
		db.Raw("SELECT COALESCE(email, '') FROM users_emails WHERE userid = ? ORDER BY preferred DESC LIMIT 1", m.User1).Scan(&email1)
	}()
	go func() {
		defer wg.Done()
		db.Raw("SELECT COALESCE(fullname, 'A freegler') FROM users WHERE id = ?", m.User2).Scan(&name2)
	}()
	go func() {
		defer wg.Done()
		db.Raw("SELECT COALESCE(email, '') FROM users_emails WHERE userid = ? ORDER BY preferred DESC LIMIT 1", m.User2).Scan(&email2)
	}()
	wg.Wait()

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"merge": fiber.Map{
			"id":  m.ID,
			"uid": m.UID,
			"user1": fiber.Map{
				"id":    m.User1,
				"name":  name1,
				"email": obfuscateEmail(email1),
			},
			"user2": fiber.Map{
				"id":    m.User2,
				"name":  name2,
				"email": obfuscateEmail(email2),
			},
			"accepted": m.Accepted,
			"rejected": m.Rejected,
		},
	})
}

// CreateMerge handles PUT /merge - mod creates a merge offer.
//
// @Summary Create merge offer
// @Tags merge
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/merge [put]
func CreateMerge(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	if !user.IsModOfAnyGroup(myid) {
		return fiber.NewError(fiber.StatusForbidden, "Permission denied")
	}

	type CreateRequest struct {
		User1 uint64 `json:"user1"`
		User2 uint64 `json:"user2"`
		Email *bool  `json:"email"`
	}

	var req CreateRequest
	if c.Get("Content-Type") == "application/json" {
		c.BodyParser(&req)
	}
	if req.User1 == 0 {
		req.User1, _ = strconv.ParseUint(c.FormValue("user1", c.Query("user1", "0")), 10, 64)
	}
	if req.User2 == 0 {
		req.User2, _ = strconv.ParseUint(c.FormValue("user2", c.Query("user2", "0")), 10, 64)
	}

	if req.User1 == 0 || req.User2 == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid parameters")
	}

	uid := generateUID()

	db := database.DBConn
	result := db.Exec("INSERT INTO merges (user1, user2, offeredby, uid) VALUES (?, ?, ?, ?)",
		req.User1, req.User2, myid, uid)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Create failed")
	}

	var newID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&newID)

	// Queue email to both users (default true).
	sendEmail := true
	if req.Email != nil {
		sendEmail = *req.Email
	}

	if sendEmail {
		if err := queue.QueueTask(queue.TaskEmailMerge, map[string]interface{}{
			"merge_id": newID,
			"uid":      uid,
			"user1":    req.User1,
			"user2":    req.User2,
		}); err != nil {
			log.Printf("Failed to queue merge email for merge %d: %v", newID, err)
		}
	}

	// Flag related users as notified.
	db.Exec("UPDATE users_related SET notified = 1 WHERE (user1 = ? AND user2 = ?) OR (user1 = ? AND user2 = ?)",
		req.User1, req.User2, req.User2, req.User1)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": newID, "uid": uid})
}

// PostMerge handles POST /merge - accept or reject a merge (public with uid).
//
// @Summary Accept or reject merge
// @Tags merge
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/merge [post]
func PostMerge(c *fiber.Ctx) error {
	type ActionRequest struct {
		ID     uint64 `json:"id"`
		UID    string `json:"uid"`
		User1  uint64 `json:"user1"`
		User2  uint64 `json:"user2"`
		Action string `json:"action"`
	}

	var req ActionRequest
	if c.Get("Content-Type") == "application/json" {
		c.BodyParser(&req)
	}
	if req.ID == 0 {
		req.ID, _ = strconv.ParseUint(c.FormValue("id", c.Query("id", "0")), 10, 64)
	}
	if req.UID == "" {
		req.UID = c.FormValue("uid", c.Query("uid", ""))
	}
	if req.Action == "" {
		req.Action = c.FormValue("action", c.Query("action", ""))
	}

	if req.ID == 0 || req.UID == "" || req.Action == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Missing parameters")
	}

	db := database.DBConn

	// Validate merge exists and uid matches.
	type MergeRow struct {
		ID    uint64 `json:"id"`
		User1 uint64 `json:"user1"`
		User2 uint64 `json:"user2"`
		UID   string `json:"uid"`
	}

	var m MergeRow
	db.Raw("SELECT id, user1, user2, uid FROM merges WHERE id = ? AND uid = ?", req.ID, req.UID).Scan(&m)

	if m.ID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Not found")
	}

	// Validate user1/user2 match the merge record (either order).
	if req.User1 > 0 && req.User2 > 0 {
		valid := (req.User1 == m.User1 && req.User2 == m.User2) ||
			(req.User1 == m.User2 && req.User2 == m.User1)
		if !valid {
			return fiber.NewError(fiber.StatusForbidden, "User mismatch")
		}
	}

	switch req.Action {
	case "Accept":
		db.Exec("UPDATE merges SET accepted = NOW() WHERE id = ?", req.ID)
	case "Reject":
		db.Exec("UPDATE merges SET rejected = NOW() WHERE id = ?", req.ID)
	default:
		return fiber.NewError(fiber.StatusBadRequest, "Invalid action")
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// DeleteMerge handles DELETE /merge - mod marks related users as notified.
//
// @Summary Delete merge / mark related notified
// @Tags merge
// @Produce json
// @Param user1 query integer true "User 1 ID"
// @Param user2 query integer true "User 2 ID"
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/merge [delete]
func DeleteMerge(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	if !user.IsModOfAnyGroup(myid) {
		return fiber.NewError(fiber.StatusForbidden, "Permission denied")
	}

	type DeleteRequest struct {
		User1 uint64 `json:"user1"`
		User2 uint64 `json:"user2"`
	}

	var req DeleteRequest
	if c.Get("Content-Type") == "application/json" {
		c.BodyParser(&req)
	}
	if req.User1 == 0 {
		req.User1, _ = strconv.ParseUint(c.FormValue("user1", c.Query("user1", "0")), 10, 64)
	}
	if req.User2 == 0 {
		req.User2, _ = strconv.ParseUint(c.FormValue("user2", c.Query("user2", "0")), 10, 64)
	}

	if req.User1 == 0 || req.User2 == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid parameters")
	}

	db := database.DBConn
	db.Exec("UPDATE users_related SET notified = 1 WHERE (user1 = ? AND user2 = ?) OR (user1 = ? AND user2 = ?)",
		req.User1, req.User2, req.User2, req.User1)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
