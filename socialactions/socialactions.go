package socialactions

import (
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// SocialAction represents a pending social action for a group
type SocialAction struct {
	ID         uint64     `json:"id"`
	Userid     uint64     `json:"userid"`
	Groupid    uint64     `json:"groupid"`
	Msgid      *uint64    `json:"msgid,omitempty"`
	ActionType string     `json:"action_type"`
	UID        *string    `json:"uid,omitempty"`
	Created    *time.Time `json:"created,omitempty"`
}

// TableName overrides GORM table name to avoid race conditions in testing
func (SocialAction) TableName() string {
	return "socialactions"
}

// PostRequest represents the body for POST /socialactions
type PostRequest struct {
	ID      uint64 `json:"id"`
	UID     string `json:"uid"`
	Action  string `json:"action"`
	Groupid uint64 `json:"groupid"`
	Msgid   uint64 `json:"msgid"`
}

// GetSocialActions returns pending social actions for groups the logged-in user moderates
// @Summary Get pending social actions
// @Description Returns pending social actions for groups the user moderates
// @Tags socialactions
// @Produce json
// @Param groupid query int false "Filter by group ID"
// @Security BearerAuth
// @Success 200 {array} SocialAction
// @Failure 401 {object} fiber.Error "Not logged in"
// @Router /socialactions [get]
func GetSocialActions(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	groupid := c.QueryInt("groupid", 0)

	query := `SELECT sa.id, sa.userid, sa.groupid, sa.msgid, sa.action_type, sa.uid, sa.created
		FROM socialactions sa
		INNER JOIN memberships m ON m.groupid = sa.groupid AND m.userid = ? AND m.role IN ('Owner', 'Moderator')
		WHERE sa.performed IS NULL`
	params := []interface{}{myid}

	if groupid > 0 {
		query += " AND sa.groupid = ?"
		params = append(params, groupid)
	}

	query += " ORDER BY sa.created DESC"

	var actions []SocialAction
	db.Raw(query, params...).Scan(&actions)

	// Split into socialactions and popularposts for the client store.
	socialactions := make([]SocialAction, 0)
	popularposts := make([]SocialAction, 0)

	for _, a := range actions {
		if a.ActionType == "popular" {
			popularposts = append(popularposts, a)
		} else {
			socialactions = append(socialactions, a)
		}
	}

	return c.JSON(fiber.Map{
		"socialactions": socialactions,
		"popularposts":  popularposts,
	})
}

// PostSocialAction handles social action operations (Do, Hide, DoPopular, HidePopular)
// @Summary Perform a social action
// @Description Marks a social action as performed (shared or hidden)
// @Tags socialactions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} fiber.Map
// @Failure 400 {object} fiber.Error "Invalid request"
// @Failure 401 {object} fiber.Error "Not logged in"
// @Failure 403 {object} fiber.Error "Not authorized"
// @Router /socialactions [post]
func PostSocialAction(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PostRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Action == "" {
		return fiber.NewError(fiber.StatusBadRequest, "action is required")
	}

	db := database.DBConn

	switch req.Action {
	case "Do":
		if req.ID == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "id is required")
		}

		// Check the user is a mod on the group this social action belongs to
		if !isModForSocialAction(myid, req.ID) {
			return fiber.NewError(fiber.StatusForbidden, "Not authorized")
		}

		db.Exec("UPDATE socialactions SET performed = NOW(), uid = ? WHERE id = ?", req.UID, req.ID)

	case "Hide":
		if req.ID == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "id is required")
		}

		// Check the user is a mod on the group this social action belongs to
		if !isModForSocialAction(myid, req.ID) {
			return fiber.NewError(fiber.StatusForbidden, "Not authorized")
		}

		db.Exec("UPDATE socialactions SET performed = NOW() WHERE id = ?", req.ID)

	case "DoPopular":
		if req.Groupid == 0 || req.Msgid == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "groupid and msgid are required")
		}

		// Check the user is a mod on this group
		if !isModForGroup(myid, req.Groupid) {
			return fiber.NewError(fiber.StatusForbidden, "Not authorized")
		}

		db.Exec("UPDATE socialactions SET performed = NOW(), uid = ? WHERE groupid = ? AND msgid = ? AND action_type = 'popular'",
			req.UID, req.Groupid, req.Msgid)

	case "HidePopular":
		if req.Groupid == 0 || req.Msgid == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "groupid and msgid are required")
		}

		// Check the user is a mod on this group
		if !isModForGroup(myid, req.Groupid) {
			return fiber.NewError(fiber.StatusForbidden, "Not authorized")
		}

		db.Exec("UPDATE socialactions SET performed = NOW() WHERE groupid = ? AND msgid = ? AND action_type = 'popular'",
			req.Groupid, req.Msgid)

	default:
		return fiber.NewError(fiber.StatusBadRequest, "Unknown action: "+req.Action)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// isModForSocialAction checks if the user is a moderator on the group that a social action belongs to
func isModForSocialAction(myid uint64, socialActionID uint64) bool {
	db := database.DBConn

	var count int64
	db.Raw(`SELECT COUNT(*) FROM socialactions sa
		INNER JOIN memberships m ON m.groupid = sa.groupid AND m.userid = ? AND m.role IN ('Owner', 'Moderator')
		WHERE sa.id = ?`, myid, socialActionID).Scan(&count)

	return count > 0
}

// isModForGroup checks if the user is a moderator on a specific group
func isModForGroup(myid uint64, groupid uint64) bool {
	db := database.DBConn

	var count int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ? AND role IN ('Owner', 'Moderator')",
		myid, groupid).Scan(&count)

	return count > 0
}

