package microvolunteering

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// Version matches the PHP MicroVolunteering::VERSION constant
const Version = 4

// PostResponseRequest represents the body for POST /microvolunteering
type PostResponseRequest struct {
	Msgid       uint64  `json:"msgid"`
	MsgCategory *string `json:"msgcategory,omitempty"`
	Response    *string `json:"response,omitempty"`
	Comments    *string `json:"comments,omitempty"`
	Searchterm1 uint64  `json:"searchterm1"`
	Searchterm2 uint64  `json:"searchterm2"`
	Facebook    uint64  `json:"facebook"`
	Photoid     uint64  `json:"photoid"`
	Invite      bool    `json:"invite"`
	Deg         int     `json:"deg"`
}

// PostResponse records a user's response to a micro-volunteering challenge
// @Summary Submit micro-volunteering response
// @Description Records the user's response to a micro-volunteering challenge
// @Tags microvolunteering
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} fiber.Map
// @Failure 400 {object} fiber.Error "Invalid parameters"
// @Failure 401 {object} fiber.Error "Not logged in"
// @Router /microvolunteering [post]
func PostResponse(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PostResponseRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	db := database.DBConn

	if req.Msgid > 0 && req.Response != nil {
		// Response to a CheckMessage challenge
		response := *req.Response

		if response == "Approve" || response == "Reject" {
			// Mark any notifications regarding this message as read
			db.Exec(`UPDATE users_notifications SET seen = 1
				WHERE touser = ? AND url LIKE CONCAT('/microvolunteering/message/', ?) AND type = 'Exhort'`,
				myid, req.Msgid)

			// Record the response - insert or update
			var msgcategory interface{}
			if req.MsgCategory != nil {
				msgcategory = *req.MsgCategory
			}

			var comments interface{}
			if req.Comments != nil {
				comments = *req.Comments
			}

			db.Exec(`INSERT INTO microactions (actiontype, userid, msgid, result, msgcategory, comments, version)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON DUPLICATE KEY UPDATE result = ?, comments = ?, version = ?, msgcategory = ?`,
				ChallengeCheckMessage, myid, req.Msgid, response, msgcategory, comments, Version,
				response, comments, Version, msgcategory)

			// If rejection, check if we have quorum to send for review
			if response == "Reject" {
				var rejectCount int64
				db.Raw(`SELECT COUNT(*) FROM microactions
					WHERE msgid = ? AND result = 'Reject' AND comments IS NOT NULL
					AND (msgcategory IS NULL OR msgcategory = 'ShouldntBeHere')`,
					req.Msgid).Scan(&rejectCount)

				if rejectCount >= int64(ApprovalQuorum) {
					// Quorum reached - the batch process will handle sending for review
					// We don't replicate the PHP Message->sendForReview() here
				}
			}
		}

		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	} else if req.Searchterm1 > 0 && req.Searchterm2 > 0 {
		// Response to a SearchTerm challenge
		db.Exec(`INSERT INTO microactions (actiontype, userid, item1, item2, version)
			VALUES (?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE userid = userid, version = ?`,
			ChallengeSearchTerm, myid, req.Searchterm1, req.Searchterm2, Version, Version)

		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	} else if req.Facebook > 0 {
		// Response to a Facebook share challenge.
		// The result column is enum('Approve','Reject') NOT NULL - the actual response
		// (e.g. "Shared") can't be stored there. PHP uses INSERT IGNORE which silently
		// truncates. We omit result and let MySQL default to the first enum value.
		db.Exec(`INSERT IGNORE INTO microactions (actiontype, userid, facebook_post, version)
			VALUES (?, ?, ?, ?)`,
			ChallengeFacebookShare, myid, req.Facebook, Version)

		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	} else if req.Photoid > 0 {
		// Response to a PhotoRotate challenge
		var response interface{}
		if req.Response != nil {
			response = *req.Response
		}

		db.Exec(`INSERT IGNORE INTO microactions (actiontype, userid, rotatedimage, result, version)
			VALUES (?, ?, ?, ?, ?)`,
			ChallengePhotoRotate, myid, req.Photoid, response, Version)

		// Check if we have enough votes to rotate the photo
		rotated := false
		if req.Response != nil && *req.Response == "Reject" {
			var voteCount int64
			db.Raw("SELECT COUNT(*) FROM microactions WHERE rotatedimage = ? AND result = 'Reject'",
				req.Photoid).Scan(&voteCount)

			if voteCount >= int64(ApprovalQuorum) {
				// Enough votes - the batch process handles the actual rotation
				rotated = true
			}
		}

		return c.JSON(fiber.Map{"ret": 0, "status": "Success", "rotated": rotated})

	} else if req.Invite {
		// Response to an Invite challenge.
		// The result column is enum('Approve','Reject') NOT NULL. Set to 'Approve' as
		// the default value since invite responses don't map to approve/reject.
		db.Exec(`INSERT IGNORE INTO microactions (actiontype, userid, version, result)
			VALUES (?, ?, ?, 'Approve')`,
			ChallengeInvite, myid, Version)

		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
	}

	return fiber.NewError(fiber.StatusBadRequest, "Invalid parameters")
}
