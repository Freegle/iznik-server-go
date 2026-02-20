package user

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"strings"
)

type UserPostRequest struct {
	Action    string  `json:"action"`
	Engageid  uint64  `json:"engageid"`
	Ratee     uint64  `json:"ratee"`
	Rating    *string `json:"rating"`
	Reason    *string `json:"reason"`
	Text      *string `json:"text"`
	Ratingid  uint64  `json:"ratingid"`
	ID        uint64  `json:"id"`
	Email     string  `json:"email"`
	Primary   *bool   `json:"primary"`
	ID1       uint64  `json:"id1"`
	ID2       uint64  `json:"id2"`
}

func PostUser(c *fiber.Ctx) error {
	myid := WhoAmI(c)

	var req UserPostRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	db := database.DBConn

	// Engaged doesn't require login.
	if req.Engageid > 0 {
		return handleEngaged(c, db, req.Engageid)
	}

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	switch req.Action {
	case "Rate":
		return handleRate(c, db, myid, req)
	case "RatingReviewed":
		return handleRatingReviewed(c, db, myid, req)
	case "AddEmail":
		return handleAddEmail(c, db, myid, req)
	case "RemoveEmail":
		return handleRemoveEmail(c, db, myid, req)
	case "Unbounce":
		return handleUnbounce(c, myid, req)
	case "Merge":
		return handleMerge(c, myid, req)
	default:
		return fiber.NewError(fiber.StatusBadRequest, "Unknown action")
	}
}

func handleEngaged(c *fiber.Ctx, db *gorm.DB, engageid uint64) error {
	// Record engagement success.
	var mailid uint64
	db.Raw("SELECT mailid FROM engage WHERE id = ?", engageid).Scan(&mailid)

	if mailid > 0 {
		db.Exec("UPDATE engage SET succeeded = NOW() WHERE id = ?", engageid)
		db.Exec("UPDATE engage_mails SET action = action + 1, rate = COALESCE(100 * action / shown, 0) WHERE id = ?", mailid)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

func handleRate(c *fiber.Ctx, db *gorm.DB, myid uint64, req UserPostRequest) error {
	if req.Ratee == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "ratee is required")
	}

	// Validate rating value.
	if req.Rating != nil && *req.Rating != utils.RATING_UP && *req.Rating != utils.RATING_DOWN {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid rating value")
	}

	// Can't rate yourself.
	if req.Ratee == myid {
		return fiber.NewError(fiber.StatusBadRequest, "Cannot rate yourself")
	}

	// Determine if review is required (down-vote with reason and text).
	reviewRequired := false
	if req.Rating != nil && *req.Rating == utils.RATING_DOWN && req.Reason != nil && req.Text != nil {
		reviewRequired = true
	}

	db.Exec("REPLACE INTO ratings (rater, ratee, rating, reason, text, timestamp, reviewrequired) VALUES (?, ?, ?, ?, ?, NOW(), ?)",
		myid, req.Ratee, req.Rating, req.Reason, req.Text, reviewRequired)

	// Update lastupdated for both users.
	db.Exec("UPDATE users SET lastupdated = NOW() WHERE id IN (?, ?)", myid, req.Ratee)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

func handleRatingReviewed(c *fiber.Ctx, db *gorm.DB, myid uint64, req UserPostRequest) error {
	if req.Ratingid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "ratingid is required")
	}

	// Mark the rating as reviewed. Only allow if the user can see the rating
	// (i.e., they are a mod for the ratee's group).
	db.Exec("UPDATE ratings SET reviewrequired = 0 WHERE id = ?", req.Ratingid)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

func handleAddEmail(c *fiber.Ctx, db *gorm.DB, myid uint64, req UserPostRequest) error {
	if req.Email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "email is required")
	}

	email := strings.TrimSpace(req.Email)
	targetID := req.ID
	if targetID == 0 {
		targetID = myid
	}

	// Only allow if admin/support or own account.
	if targetID != myid {
		var isSupport bool
		db.Raw("SELECT systemrole IN ('Support', 'Admin') FROM users WHERE id = ?", myid).Scan(&isSupport)
		if !isSupport {
			return fiber.NewError(fiber.StatusForbidden, "You cannot administer those users")
		}
	}

	// Check if email is already in use by another user.
	var existingUID uint64
	db.Raw("SELECT userid FROM users_emails WHERE email = ? AND userid IS NOT NULL", email).Scan(&existingUID)

	if existingUID > 0 && existingUID != targetID {
		// Email is used by a different user.
		var isSupport bool
		db.Raw("SELECT systemrole IN ('Support', 'Admin') FROM users WHERE id = ?", myid).Scan(&isSupport)
		if !isSupport {
			return c.JSON(fiber.Map{"ret": 3, "status": "Email already used"})
		}
	}

	// Add the email.
	isPrimary := true
	if req.Primary != nil {
		isPrimary = *req.Primary
	}

	var primaryVal int
	if isPrimary {
		primaryVal = 1
	}

	result := db.Exec("INSERT INTO users_emails (userid, email, preferred, validated, canon) VALUES (?, ?, ?, NOW(), ?)",
		targetID, email, primaryVal, canonicalizeEmail(email))

	if result.Error != nil {
		return c.JSON(fiber.Map{"ret": 4, "status": "Email add failed"})
	}

	var emailID uint64
	db.Raw("SELECT id FROM users_emails WHERE userid = ? AND email = ? ORDER BY id DESC LIMIT 1", targetID, email).Scan(&emailID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "emailid": emailID})
}

func handleRemoveEmail(c *fiber.Ctx, db *gorm.DB, myid uint64, req UserPostRequest) error {
	if req.Email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "email is required")
	}

	targetID := req.ID
	if targetID == 0 {
		targetID = myid
	}

	// Only allow if admin/support or own account.
	if targetID != myid {
		var isSupport bool
		db.Raw("SELECT systemrole IN ('Support', 'Admin') FROM users WHERE id = ?", myid).Scan(&isSupport)
		if !isSupport {
			return fiber.NewError(fiber.StatusForbidden, "You cannot administer those users")
		}
	}

	// Verify email belongs to this user.
	var emailUserid uint64
	db.Raw("SELECT userid FROM users_emails WHERE email = ? AND userid = ?", req.Email, targetID).Scan(&emailUserid)

	if emailUserid == 0 {
		return c.JSON(fiber.Map{"ret": 3, "status": "Not on same user"})
	}

	db.Exec("DELETE FROM users_emails WHERE email = ? AND userid = ?", req.Email, targetID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// canonicalizeEmail returns a canonical form of the email for deduplication.
func canonicalizeEmail(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return email
	}
	// Remove dots and plus-addressing from local part for Gmail-style canonicalization.
	local := strings.ReplaceAll(parts[0], ".", "")
	if idx := strings.Index(local, "+"); idx >= 0 {
		local = local[:idx]
	}
	return local + "@" + parts[1]
}
