package microvolunteering

import (
	"fmt"
	"os"
	"strings"
	"time"

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
	Facebook *FacebookPost `json:"facebook,omitempty"`
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

// FacebookPost represents a Facebook post to share
type FacebookPost struct {
	ID   uint64 `json:"id"`
	Date time.Time `json:"date"`
	Data string `json:"data"`
}

// Challenge types
const (
	ChallengeCheckMessage   = "CheckMessage"
	ChallengeSearchTerm     = "SearchTerm"
	ChallengePhotoRotate    = "PhotoRotate"
	ChallengeFacebookShare  = "FacebookShare"
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
