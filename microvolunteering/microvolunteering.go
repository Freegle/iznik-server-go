package microvolunteering

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/freegle/iznik-server-go/auth"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Challenge represents a micro-volunteering challenge
type Challenge struct {
	Type   string      `json:"type"`
	Msgid  *uint64     `json:"msgid,omitempty"`
	Terms  []SearchTerm `json:"terms,omitempty"`
	Photos []Photo     `json:"photos,omitempty"`
	URL    *string     `json:"url,omitempty"`
}

// SearchTerm represents a search term for matching
type SearchTerm struct {
	ID   uint64 `json:"id"`
	Term string `json:"term"`
}

// Photo represents a photo for rotation challenge
type Photo struct {
	ID   uint64 `json:"id"`
	Path string `json:"path"`
}

// Challenge types
const (
	ChallengeCheckMessage   = "CheckMessage"
	ChallengeSearchTerm     = "SearchTerm"
	ChallengePhotoRotate    = "PhotoRotate"
	ChallengeSurvey         = "Survey2"
	ChallengeInvite         = "Invite"
)

// Trust levels
const (
	TrustExcluded = "Excluded"
	TrustDeclined = "Declined"
	TrustBasic    = "Basic"
	TrustModerate = "Moderate"
	TrustAdvanced = "Advanced"
)

// Microvolunteering quorum constants
const (
	ApprovalQuorum   = 2
	DissentingQuorum = 3
)

// GetChallenge returns a micro-volunteering challenge for the logged-in user
// @Summary Get micro-volunteering challenge
// @Description Returns a micro-volunteering challenge for the logged-in user
// @Tags microvolunteering
// @Accept json
// @Produce json
// @Param groupid query int false "Group ID to get challenges for"
// @Param types query []string false "Challenge types to include"
// @Success 200 {object} Challenge "Micro-volunteering challenge"
// @Failure 401 {object} map[string]string "Not logged in"
// @Router /microvolunteering [get]
func GetChallenge(c *fiber.Ctx) error {
	db := database.DBConn

	// Get user ID from JWT
	userID, _, _ := user.GetJWTFromRequest(c)
	if userID == 0 {
		return c.Status(401).JSON(fiber.Map{
			"error": "Not logged in",
		})
	}

	// V1 parity: when list=true, return moderator listing of microactions.
	if c.Query("list") == "true" || c.Query("list") == "1" {
		return listMicroActions(c, db, userID)
	}

	// Get parameters
	groupID := c.QueryInt("groupid", 0)
	types := c.Query("types", "")

	// Parse types if provided
	var challengeTypes []string
	if types != "" {
		// Parse comma-separated types
		// For now, default to all types
		challengeTypes = []string{
			ChallengeInvite,
			ChallengeCheckMessage,
			ChallengePhotoRotate,
		}
	} else {
		challengeTypes = []string{
			ChallengeInvite,
			ChallengeCheckMessage,
			ChallengePhotoRotate,
		}
	}

	// Get user's trust level
	var trustLevel string
	err := db.Raw(`
		SELECT COALESCE(trustlevel, ?) FROM users WHERE id = ?
	`, TrustBasic, userID).Scan(&trustLevel).Error

	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to fetch user trust level",
		})
	}

	// Don't offer challenges to declined/excluded users
	if trustLevel == TrustDeclined || trustLevel == TrustExcluded {
		return c.JSON(fiber.Map{})
	}

	// Get user's group IDs
	var groupIDs []uint64
	if groupID > 0 {
		groupIDs = []uint64{uint64(groupID)}
	} else {
		// Get all user's Freegle groups
		db.Raw(`
			SELECT groupid FROM memberships
			INNER JOIN `+"`groups`"+` ON memberships.groupid = `+"`groups`"+`.id
			WHERE userid = ? AND type = 'Freegle'
		`, userID).Scan(&groupIDs)
	}

	// Try Invite challenge first
	if contains(challengeTypes, ChallengeInvite) {
		if challenge := getInviteChallenge(db, userID); challenge != nil {
			return c.JSON(challenge)
		}
	}

	// Try pending message review for Moderate trust level users
	if trustLevel == TrustModerate && len(groupIDs) > 0 {
		if challenge := getPendingMessageChallenge(db, userID, groupIDs); challenge != nil {
			return c.JSON(challenge)
		}
	}

	// Try approved message review for all users
	if contains(challengeTypes, ChallengeCheckMessage) && len(groupIDs) > 0 {
		if challenge := getApprovedMessageChallenge(db, userID, groupIDs); challenge != nil {
			return c.JSON(challenge)
		}
	}

	// Try photo rotate challenge
	if contains(challengeTypes, ChallengePhotoRotate) && len(groupIDs) > 0 {
		if challenge := getPhotoRotateChallenge(db, userID, groupIDs); challenge != nil {
			return c.JSON(challenge)
		}
	}

	// Try search term challenge
	if contains(challengeTypes, ChallengeSearchTerm) {
		// Check if user is in a group with word matching enabled
		var enabled int
		var query string
		var params []interface{}

		if groupID > 0 {
			// Filter to specific group if provided
			query = `
				SELECT COUNT(*)
				FROM memberships
				INNER JOIN ` + "`groups`" + ` ON memberships.groupid = ` + "`groups`" + `.id
				WHERE memberships.userid = ?
				AND memberships.groupid = ?
				AND (microvolunteeringoptions IS NULL OR JSON_EXTRACT(microvolunteeringoptions, '$.wordmatch') = 1)
			`
			params = []interface{}{userID, groupID}
		} else {
			// Check all user's groups
			query = `
				SELECT COUNT(*)
				FROM memberships
				INNER JOIN ` + "`groups`" + ` ON memberships.groupid = ` + "`groups`" + `.id
				WHERE memberships.userid = ?
				AND (microvolunteeringoptions IS NULL OR JSON_EXTRACT(microvolunteeringoptions, '$.wordmatch') = 1)
			`
			params = []interface{}{userID}
		}

		db.Raw(query, params...).Scan(&enabled)

		if enabled > 0 {
			// Get 10 random popular items
			type ItemTerm struct {
				ID   uint64 `json:"id"`
				Term string `json:"term"`
			}
			var terms []ItemTerm

			db.Raw(`
				SELECT DISTINCT id, name AS term
				FROM (SELECT id, name FROM items WHERE LENGTH(name) > 2 ORDER BY popularity DESC LIMIT 300) t
				ORDER BY RAND() LIMIT 10
			`).Scan(&terms)

			if len(terms) > 0 {
				var searchTerms []SearchTerm
				for _, t := range terms {
					searchTerms = append(searchTerms, SearchTerm{
						ID:   t.ID,
						Term: t.Term,
					})
				}

				return c.JSON(Challenge{
					Type:  ChallengeSearchTerm,
					Terms: searchTerms,
				})
			}
		}
	}

	// If no challenge found, return empty object
	return c.JSON(fiber.Map{})
}

// Helper function to check if slice contains string
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// getInviteChallenge returns an invite challenge if the user hasn't been asked recently
func getInviteChallenge(db *gorm.DB, userID uint64) *Challenge {
	// Check if we've asked in the last 31 days
	var count int
	db.Raw(`
		SELECT COUNT(*) FROM microactions
		WHERE userid = ? AND actiontype = ?
		AND DATEDIFF(NOW(), timestamp) < 31
	`, userID, ChallengeInvite).Scan(&count)

	if count == 0 {
		// Record a placeholder to ensure we don't ask too often
		db.Exec(`
			INSERT INTO microactions (actiontype, userid, version, comments)
			VALUES (?, ?, 4, 'Ask to invite')
		`, ChallengeInvite, userID)

		return &Challenge{
			Type: ChallengeInvite,
		}
	}

	return nil
}

// getPendingMessageChallenge returns a pending message for moderate trust users to review
func getPendingMessageChallenge(db *gorm.DB, userID uint64, groupIDs []uint64) *Challenge {
	if len(groupIDs) == 0 {
		return nil
	}

	// Convert group IDs to comma-separated string for SQL
	groupIDStrs := make([]string, len(groupIDs))
	for i, id := range groupIDs {
		groupIDStrs[i] = fmt.Sprintf("%d", id)
	}
	groupIDsStr := strings.Join(groupIDStrs, ",")

	type MessageResult struct {
		Msgid uint64 `json:"msgid"`
	}
	var msg MessageResult

	err := db.Raw(`
		SELECT messages_groups.msgid
		FROM messages_groups
		INNER JOIN messages ON messages.id = messages_groups.msgid
		INNER JOIN `+"`groups`"+` ON groups.id = messages_groups.groupid
		LEFT JOIN microactions ON microactions.msgid = messages_groups.msgid AND microactions.userid = ?
		WHERE messages_groups.groupid IN (`+groupIDsStr+`)
			AND DATE(messages.arrival) = CURDATE()
			AND fromuser != ?
			AND microvolunteering = 1
			AND messages.deleted IS NULL
			AND microactions.id IS NULL
			AND (microvolunteeringoptions IS NULL OR JSON_EXTRACT(microvolunteeringoptions, '$.approvedmessages') = 1)
			AND collection = ?
			AND autoreposts = 0
		ORDER BY messages_groups.arrival ASC LIMIT 1
	`, userID, userID, utils.COLLECTION_PENDING).Scan(&msg).Error

	if err == nil && msg.Msgid > 0 {
		return &Challenge{
			Type:  ChallengeCheckMessage,
			Msgid: &msg.Msgid,
		}
	}

	return nil
}

// getApprovedMessageChallenge returns an approved message for any user to review
func getApprovedMessageChallenge(db *gorm.DB, userID uint64, groupIDs []uint64) *Challenge {
	if len(groupIDs) == 0 {
		return nil
	}

	// Convert group IDs to comma-separated string for SQL
	groupIDStrs := make([]string, len(groupIDs))
	for i, id := range groupIDs {
		groupIDStrs[i] = fmt.Sprintf("%d", id)
	}
	groupIDsStr := strings.Join(groupIDStrs, ",")

	type MessageResult struct {
		Msgid uint64 `json:"msgid"`
	}
	var msg MessageResult

	resultApprove := "Approve"

	err := db.Raw(`
		SELECT messages_spatial.msgid,
			(SELECT COUNT(*) AS count FROM microactions WHERE msgid = messages_spatial.msgid) AS reviewcount,
			(SELECT COUNT(*) AS count FROM microactions WHERE msgid = messages_spatial.msgid AND result = ?) AS approvalcount
		FROM messages_spatial
		INNER JOIN messages_groups ON messages_spatial.msgid = messages_groups.msgid
		INNER JOIN messages ON messages.id = messages_spatial.msgid
		INNER JOIN `+"`groups`"+` ON groups.id = messages_groups.groupid
		LEFT JOIN microactions ON microactions.msgid = messages_spatial.msgid AND microactions.userid = ?
		LEFT JOIN messages_outcomes ON messages_outcomes.msgid = messages_spatial.msgid
		WHERE messages_groups.groupid IN (`+groupIDsStr+`)
			AND DATE(messages.arrival) = CURDATE()
			AND fromuser != ?
			AND microvolunteering = 1
			AND messages_outcomes.id IS NULL
			AND messages.deleted IS NULL
			AND microactions.id IS NULL
			AND (microvolunteeringoptions IS NULL OR JSON_EXTRACT(microvolunteeringoptions, '$.approvedmessages') = 1)
			AND collection = ?
			AND autoreposts = 0
		HAVING approvalcount < ? AND reviewcount < ?
		ORDER BY messages_groups.arrival ASC LIMIT 1
	`, resultApprove, userID, userID, utils.COLLECTION_APPROVED, ApprovalQuorum, DissentingQuorum).Scan(&msg).Error

	if err == nil && msg.Msgid > 0 {
		return &Challenge{
			Type:  ChallengeCheckMessage,
			Msgid: &msg.Msgid,
		}
	}

	return nil
}

// getPhotoRotateChallenge returns photos that need rotation review
func getPhotoRotateChallenge(db *gorm.DB, userID uint64, groupIDs []uint64) *Challenge {
	if len(groupIDs) == 0 {
		return nil
	}

	// Convert group IDs to comma-separated string for SQL
	groupIDStrs := make([]string, len(groupIDs))
	for i, id := range groupIDs {
		groupIDStrs[i] = fmt.Sprintf("%d", id)
	}
	groupIDsStr := strings.Join(groupIDStrs, ",")

	type PhotoResult struct {
		ID uint64 `json:"id"`
	}
	var photos []PhotoResult

	today := time.Now().Format("2006-01-02")

	err := db.Raw(`
		SELECT messages_attachments.id,
			(SELECT COUNT(*) AS count FROM microactions WHERE rotatedimage = messages_attachments.id) AS reviewcount
		FROM messages_groups
		INNER JOIN messages_attachments ON messages_attachments.msgid = messages_groups.msgid
		LEFT JOIN microactions ON microactions.rotatedimage = messages_attachments.id AND userid = ?
		INNER JOIN `+"`groups`"+` ON groups.id = messages_groups.groupid AND microvolunteering = 1 AND (microvolunteeringoptions IS NULL OR JSON_EXTRACT(microvolunteeringoptions, '$.photorotate') = 1)
		WHERE arrival >= ? AND groupid IN (`+groupIDsStr+`) AND microactions.id IS NULL
		HAVING reviewcount < ?
		ORDER BY RAND() LIMIT 9
	`, userID, today, DissentingQuorum).Scan(&photos).Error

	if err == nil && len(photos) > 0 {
		var photoList []Photo

		// Get image domain from environment
		imageDomain := os.Getenv("IMAGE_DOMAIN")
		if imageDomain == "" {
			imageDomain = "images.ilovefreegle.org"
		}

		for _, p := range photos {
			// Construct thumbnail path similar to how message.go does it
			path := "https://" + imageDomain + "/timg_" + fmt.Sprintf("%d", p.ID) + ".jpg"

			photoList = append(photoList, Photo{
				ID:   p.ID,
				Path: path,
			})
		}

		return &Challenge{
			Type:   ChallengePhotoRotate,
			Photos: photoList,
		}
	}

	return nil
}

// Version is the current microvolunteering protocol version.
const Version = 4

// PostResponseRequest represents the body for POST /microvolunteering
type PostResponseRequest struct {
	Msgid       uint64  `json:"msgid"`
	MsgCategory *string `json:"msgcategory,omitempty"`
	Response    *string `json:"response,omitempty"`
	Comments    *string `json:"comments,omitempty"`
	Searchterm1 uint64  `json:"searchterm1"`
	Searchterm2 uint64  `json:"searchterm2"`
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
					// Quorum reached - send the message for review by setting spamreason
					// and moving it back to Pending collection (V1 parity: Message::sendForReview).
					sendForReview(db, req.Msgid, "Members think there is something wrong with this message.")
				}
			}
		}

		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	} else if req.Searchterm1 > 0 && req.Searchterm2 > 0 {
		// Response to a SearchTerm challenge.
		// The result column is enum('Approve','Reject') NOT NULL with no default.
		// Set to 'Approve' since search term responses don't map to approve/reject.
		db.Exec(`INSERT INTO microactions (actiontype, userid, item1, item2, version, result)
			VALUES (?, ?, ?, ?, ?, 'Approve')
			ON DUPLICATE KEY UPDATE userid = userid, version = ?`,
			ChallengeSearchTerm, myid, req.Searchterm1, req.Searchterm2, Version, Version)

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

// ModFeedbackRequest represents the body for PATCH /microvolunteering
type ModFeedbackRequest struct {
	ID            uint64  `json:"id"`
	Feedback      string  `json:"feedback"`
	ScorePositive float64 `json:"score_positive"`
	ScoreNegative float64 `json:"score_negative"`
}

// ModFeedback allows a moderator to provide feedback on a microaction
// @Summary Provide moderator feedback on microaction
// @Description Allows a moderator to set feedback, score_positive, and score_negative on a microaction
// @Tags microvolunteering
// @Accept json
// @Produce json
// @Param id body int true "Microaction ID"
// @Param feedback body string true "Moderator feedback text"
// @Param score_positive body number false "Positive score"
// @Param score_negative body number false "Negative score"
// @Security BearerAuth
// @Success 200 {object} fiber.Map
// @Failure 400 {object} fiber.Error "Invalid parameters"
// @Failure 401 {object} fiber.Error "Not logged in"
// @Failure 403 {object} fiber.Error "Not a moderator"
// @Router /microvolunteering [patch]
func ModFeedback(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	// Only moderators can provide feedback.
	if !auth.IsSystemMod(myid) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator")
	}

	var req ModFeedbackRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 || req.Feedback == "" {
		return fiber.NewError(fiber.StatusBadRequest, "id and feedback are required")
	}

	db := database.DBConn

	// Update the microaction with mod feedback and scores.
	db.Exec("UPDATE microactions SET modfeedback = ?, score_positive = ?, score_negative = ? WHERE id = ?",
		req.Feedback, req.ScorePositive, req.ScoreNegative, req.ID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// sendForReview moves a message back to Pending and records the spam reason.
// This is the Go equivalent of V1's Message::sendForReview().
func sendForReview(db *gorm.DB, msgid uint64, reason string) {
	db.Exec("UPDATE messages SET spamreason = ? WHERE id = ?", reason, msgid)
	db.Exec("UPDATE messages_groups SET collection = ? WHERE msgid = ?", utils.COLLECTION_PENDING, msgid)
}

// listMicroActions returns microvolunteering activity for moderator review.
// V1 parity: MicroVolunteering::list() in MicroVolunteering.php.
func listMicroActions(c *fiber.Ctx, db *gorm.DB, myid uint64) error {
	groupidParam := c.QueryInt("groupid", 0)
	limitParam := c.QueryInt("limit", 10)
	start := c.Query("start", "1970-01-01")
	context := c.QueryInt("context", 0)

	// Determine which groups to query.
	var groupIDs []uint64
	if groupidParam > 0 {
		groupIDs = []uint64{uint64(groupidParam)}
	} else {
		groupIDs = user.GetActiveModGroupIDs(myid)
	}

	if len(groupIDs) == 0 {
		return c.JSON(fiber.Map{
			"ret":                 0,
			"status":              "Success",
			"microvolunteerings":  make([]interface{}, 0),
			"context":             fiber.Map{},
		})
	}

	// Build query matching V1: microactions joined with memberships filtered by group.
	ctxq := ""
	args := []interface{}{}

	args = append(args, groupIDs, start)

	if context > 0 {
		ctxq = " AND microactions.id < ?"
		args = append(args, context)
	}

	args = append(args, limitParam)

	type MicroAction struct {
		ID            uint64     `json:"id"`
		Actiontype    string     `json:"actiontype"`
		Userid        uint64     `json:"userid"`
		Msgid         *uint64    `json:"msgid"`
		Msgcategory   *string    `json:"msgcategory"`
		Result        string     `json:"result"`
		Timestamp     time.Time  `json:"timestamp"`
		Comments      *string    `json:"comments"`
		Item1         *uint64    `json:"item1"`
		Item2         *uint64    `json:"item2"`
		Rotatedimage  *uint64    `json:"rotatedimage"`
		ScorePositive float64    `json:"score_positive"`
		ScoreNegative float64    `json:"score_negative"`
		Modfeedback   *string    `json:"modfeedback"`
	}

	var items []MicroAction
	db.Raw("SELECT DISTINCT microactions.* FROM microactions "+
		"INNER JOIN memberships ON memberships.userid = microactions.userid "+
		"WHERE memberships.groupid IN (?) AND microactions.timestamp >= ?"+ctxq+
		" ORDER BY microactions.id DESC LIMIT ?", args...).Scan(&items)

	if items == nil {
		items = []MicroAction{}
	}

	// Build pagination context.
	newCtx := fiber.Map{}
	if len(items) > 0 {
		newCtx["id"] = items[len(items)-1].ID
	}

	return c.JSON(fiber.Map{
		"ret":                0,
		"status":             "Success",
		"microvolunteerings": items,
		"context":            newCtx,
	})
}
