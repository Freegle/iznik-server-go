package invitation

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/queue"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"log"
	"time"
)

const (
	OutcomePending  = "Pending"
	OutcomeAccepted = "Accepted"
	OutcomeDeclined = "Declined"
)

type Invitation struct {
	ID               uint64  `json:"id"`
	Email            string  `json:"email"`
	Date             string  `json:"date"`
	Outcome          string  `json:"outcome"`
	OutcomeTimestamp *string `json:"outcometimestamp"`
}

// ListInvitations returns the logged-in user's invitations from the last 30 days.
//
// @Summary List user invitations
// @Tags Invitation
// @Router /api/invitation [get]
func ListInvitations(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	db := database.DBConn
	since := time.Now().AddDate(0, 0, -30).Format("2006-01-02")

	var invitations []Invitation
	db.Raw("SELECT id, email, date, outcome, outcometimestamp FROM users_invitations WHERE userid = ? AND date > ? ORDER BY date DESC",
		myid, since).Scan(&invitations)

	if invitations == nil {
		invitations = []Invitation{}
	}

	// Convert dates to ISO format.
	for i := range invitations {
		if t, err := time.Parse("2006-01-02 15:04:05", invitations[i].Date); err == nil {
			invitations[i].Date = t.Format(time.RFC3339)
		}
		if invitations[i].OutcomeTimestamp != nil {
			if t, err := time.Parse("2006-01-02 15:04:05", *invitations[i].OutcomeTimestamp); err == nil {
				iso := t.Format(time.RFC3339)
				invitations[i].OutcomeTimestamp = &iso
			}
		}
	}

	return c.JSON(fiber.Map{
		"ret":         0,
		"status":      "Success",
		"invitations": invitations,
	})
}

// CreateInvitation sends an invitation email to the specified address.
// Always returns success to prevent abuse detection.
//
// @Summary Send invitation email
// @Tags Invitation
// @Router /api/invitation [put]
func CreateInvitation(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	type CreateRequest struct {
		Email string `json:"email"`
	}
	var req CreateRequest
	if err := c.BodyParser(&req); err != nil || req.Email == "" {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	db := database.DBConn

	// Check invitesleft quota.
	var invitesLeft int
	db.Raw("SELECT invitesleft FROM users WHERE id = ?", myid).Scan(&invitesLeft)
	if invitesLeft <= 0 {
		// Always return success to prevent abuse detection.
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
	}

	// Check for previously declined invitation to this email.
	var declinedCount int64
	db.Raw("SELECT COUNT(*) FROM users_invitations WHERE email = ? AND outcome = ?",
		req.Email, OutcomeDeclined).Scan(&declinedCount)
	if declinedCount > 0 {
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
	}

	// Try to insert - unique key (userid, email) prevents duplicates.
	result := db.Exec("INSERT INTO users_invitations (userid, email) VALUES (?, ?)", myid, req.Email)
	if result.Error != nil {
		// Probably a duplicate - return success silently.
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
	}

	// Get the invitation ID.
	var inviteID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&inviteID)

	// Decrement invitesleft.
	db.Exec("UPDATE users SET invitesleft = invitesleft - 1 WHERE id = ?", myid)

	// Get sender details for the email.
	var userName string
	var userEmail string
	db.Raw("SELECT fullname FROM users WHERE id = ?", myid).Scan(&userName)
	if userName == "" {
		userName = "A Freegle user"
	}

	db.Raw(`SELECT email FROM users_emails WHERE userid = ? AND preferred = 1 LIMIT 1`, myid).Scan(&userEmail)
	if userEmail == "" {
		db.Raw(`SELECT email FROM users_emails WHERE userid = ? ORDER BY added DESC LIMIT 1`, myid).Scan(&userEmail)
	}

	// Queue the invitation email.
	err := queue.QueueTask(queue.TaskEmailInvitation, map[string]interface{}{
		"invite_id":    inviteID,
		"sender_name":  userName,
		"sender_email": userEmail,
		"to_email":     req.Email,
	})
	if err != nil {
		log.Printf("Failed to queue invitation email for invite %d: %v", inviteID, err)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// UpdateOutcome updates the outcome of an invitation (Accepted/Declined).
//
// @Summary Update invitation outcome
// @Tags Invitation
// @Router /api/invitation [patch]
func UpdateOutcome(c *fiber.Ctx) error {
	type OutcomeRequest struct {
		ID      uint64 `json:"id"`
		Outcome string `json:"outcome"`
	}
	var req OutcomeRequest
	if err := c.BodyParser(&req); err != nil || req.ID == 0 {
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
	}

	if req.Outcome == "" {
		req.Outcome = OutcomeAccepted
	}

	db := database.DBConn

	// Get the current invitation.
	type inviteRow struct {
		Outcome string
		Userid  uint64
	}
	var invite inviteRow
	db.Raw("SELECT outcome, userid FROM users_invitations WHERE id = ?", req.ID).Scan(&invite)

	if invite.Userid == 0 {
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
	}

	// Only update if currently Pending.
	if invite.Outcome != OutcomePending {
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
	}

	db.Exec("UPDATE users_invitations SET outcome = ?, outcometimestamp = NOW() WHERE id = ?",
		req.Outcome, req.ID)

	// If accepted, give the sender 2 more invites.
	if req.Outcome == OutcomeAccepted {
		db.Exec("UPDATE users SET invitesleft = invitesleft + 2 WHERE id = ?", invite.Userid)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
