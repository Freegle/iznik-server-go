package membership

import (
	"encoding/json"
	"fmt"
	stdlog "log"
	"strconv"
	"strings"
	"time"

	"github.com/freegle/iznik-server-go/auth"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/log"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// logMembershipAction inserts a mod log entry for membership actions.
func logMembershipAction(db *gorm.DB, logType string, subtype string, groupid uint64, userid uint64, byuser uint64, text string) {
	db.Exec("INSERT INTO logs (timestamp, type, subtype, groupid, user, byuser, text) VALUES (NOW(), ?, ?, ?, ?, ?, ?)",
		logType, subtype, groupid, userid, byuser, text)
}

// isModOfGroup checks if the caller is a Moderator or Owner of the given group,
// or has Admin/Support system role.
func isModOfGroup(myid uint64, groupid uint64) bool {
	db := database.DBConn
	if db == nil {
		return false
	}

	if auth.IsAdminOrSupport(myid) {
		return true
	}

	if groupid == 0 {
		return false
	}

	var role string
	result := db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ? AND collection = ?",
		myid, groupid, utils.COLLECTION_APPROVED).Scan(&role)
	if result.Error != nil {
		stdlog.Printf("Failed to check mod role for user %d group %d: %v", myid, groupid, result.Error)
		return false
	}
	return role == utils.ROLE_MODERATOR || role == utils.ROLE_OWNER
}

// PostMembershipsRequest is the body for POST /memberships (moderator actions).
type PostMembershipsRequest struct {
	Userid    uint64  `json:"userid"`
	Groupid   uint64  `json:"groupid"`
	Action    string  `json:"action"`
	Subject   *string `json:"subject"`
	Body      *string `json:"body"`
	Stdmsgid  *uint64 `json:"stdmsgid"`
	Ban       *bool   `json:"ban"`
	Happiness *string `json:"happiness"`
}

// PostMemberships handles POST /memberships - moderator actions on memberships.
// Actions: Hold, Release, Approve, Leave Approved Member, Reject,
// Delete Approved Member, Ban, Unban, ReviewHold, ReviewRelease, ReviewIgnore, HappinessReviewed.
//
// @Summary Update membership actions
// @Tags membership
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/memberships [post]
func PostMemberships(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PostMembershipsRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Groupid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "groupid is required")
	}

	if req.Userid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "userid is required")
	}

	if req.Action == "" {
		return fiber.NewError(fiber.StatusBadRequest, "action is required")
	}

	// Permission check: caller must be mod/owner of group or admin/support.
	if !isModOfGroup(myid, req.Groupid) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator of this group")
	}

	db := database.DBConn

	switch req.Action {
	case "Hold":
		if result := db.Exec("UPDATE memberships SET heldby = ? WHERE userid = ? AND groupid = ?",
			myid, req.Userid, req.Groupid); result.Error != nil {
			stdlog.Printf("Failed to hold membership user %d group %d: %v", req.Userid, req.Groupid, result.Error)
		}
		logMembershipAction(db, "User", "Hold", req.Groupid, req.Userid, myid, "")
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "Release":
		db.Exec("UPDATE memberships SET heldby = NULL WHERE userid = ? AND groupid = ?",
			req.Userid, req.Groupid)
		logMembershipAction(db, "User", "Release", req.Groupid, req.Userid, myid, "")
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "Leave Member", "Leave Approved Member":
		// V1 parity: send modmail to the member without changing membership status.
		// PHP memberships.php line 291-294: just calls $u->mail().
		subject := ""
		if req.Subject != nil {
			subject = *req.Subject
		}
		body := ""
		if req.Body != nil {
			body = *req.Body
		}
		stdmsgid := uint64(0)
		if req.Stdmsgid != nil {
			stdmsgid = *req.Stdmsgid
		}
		db.Exec("INSERT INTO background_tasks (task_type, data) VALUES (?, JSON_OBJECT('userid', ?, 'groupid', ?, 'byuser', ?, 'subject', ?, 'body', ?, 'stdmsgid', ?, 'action', ?))",
			"email_mod_stdmsg", req.Userid, req.Groupid, myid, subject, body, stdmsgid, "Leave Approved Member")
		logMembershipAction(db, "User", "Modmail", req.Groupid, req.Userid, myid, subject)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "Approve":
		if result := db.Exec("UPDATE memberships SET collection = ?, heldby = NULL WHERE userid = ? AND groupid = ?",
			utils.COLLECTION_APPROVED, req.Userid, req.Groupid); result.Error != nil {
			stdlog.Printf("Failed to approve membership user %d group %d: %v", req.Userid, req.Groupid, result.Error)
		}
		logMembershipAction(db, "User", "Approved", req.Groupid, req.Userid, myid, "")

		// Queue welcome/approval email using JSON_OBJECT (same pattern as message_mod.go).
		subject := ""
		if req.Subject != nil {
			subject = *req.Subject
		}
		body := ""
		if req.Body != nil {
			body = *req.Body
		}
		db.Exec("INSERT INTO background_tasks (task_type, data) VALUES (?, JSON_OBJECT('userid', ?, 'groupid', ?, 'byuser', ?, 'subject', ?, 'body', ?))",
			TaskEmailMembershipApproved, req.Userid, req.Groupid, myid, subject, body)

		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "Reject", "Delete Approved Member":
		if result := db.Exec("DELETE FROM memberships WHERE userid = ? AND groupid = ? AND collection IN (?, ?)",
			req.Userid, req.Groupid, utils.COLLECTION_PENDING, utils.COLLECTION_APPROVED); result.Error != nil {
			stdlog.Printf("Failed to reject membership user %d group %d: %v", req.Userid, req.Groupid, result.Error)
		}
		logMembershipAction(db, "User", "Rejected", req.Groupid, req.Userid, myid, "")

		// Queue rejection notification using JSON_OBJECT (same pattern as message_mod.go).
		subject := ""
		if req.Subject != nil {
			subject = *req.Subject
		}
		body := ""
		if req.Body != nil {
			body = *req.Body
		}
		stdmsgid := uint64(0)
		if req.Stdmsgid != nil {
			stdmsgid = *req.Stdmsgid
		}
		db.Exec("INSERT INTO background_tasks (task_type, data) VALUES (?, JSON_OBJECT('userid', ?, 'groupid', ?, 'byuser', ?, 'subject', ?, 'body', ?, 'stdmsgid', ?))",
			TaskEmailMembershipRejected, req.Userid, req.Groupid, myid, subject, body, stdmsgid)

		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "Ban":
		// Delete existing membership.
		if result := db.Exec("DELETE FROM memberships WHERE userid = ? AND groupid = ? AND collection IN (?, ?)",
			req.Userid, req.Groupid, utils.COLLECTION_PENDING, utils.COLLECTION_APPROVED); result.Error != nil {
			stdlog.Printf("Failed to delete membership for ban user %d group %d: %v", req.Userid, req.Groupid, result.Error)
		}
		// Add banned record.
		if result := db.Exec("INSERT INTO memberships (userid, groupid, role, collection) VALUES (?, ?, ?, ?)",
			req.Userid, req.Groupid, utils.ROLE_MEMBER, utils.COLLECTION_BANNED); result.Error != nil {
			stdlog.Printf("Failed to insert ban record user %d group %d: %v", req.Userid, req.Groupid, result.Error)
		}
		logMembershipAction(db, "User", "Banned", req.Groupid, req.Userid, myid, "")
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "Unban":
		db.Exec("DELETE FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Banned'",
			req.Userid, req.Groupid)
		logMembershipAction(db, "User", "Unbanned", req.Groupid, req.Userid, myid, "")
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "ReviewHold":
		// ReviewHold is used in the chat review context - sets heldby on the membership.
		db.Exec("UPDATE memberships SET heldby = ? WHERE userid = ? AND groupid = ?",
			myid, req.Userid, req.Groupid)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "ReviewRelease":
		// ReviewRelease clears the heldby on the membership (chat review context).
		db.Exec("UPDATE memberships SET heldby = NULL WHERE userid = ? AND groupid = ?",
			req.Userid, req.Groupid)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "ReviewIgnore":
		// ReviewIgnore marks a spam/review member as reviewed so they drop off the Member Review list.
		// V1 parity: clear reviewrequestedat so the member doesn't reappear in review.
		// PHP User.php:6805 sets reviewrequestedat = NULL on review completion.
		db.Exec("UPDATE memberships SET reviewedat = NOW(), reviewrequestedat = NULL, heldby = NULL WHERE userid = ? AND groupid = ?",
			req.Userid, req.Groupid)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "HappinessReviewed":
		if req.Happiness == nil {
			return fiber.NewError(fiber.StatusBadRequest, "happiness is required for HappinessReviewed")
		}
		happinessID, err := strconv.ParseUint(*req.Happiness, 10, 64)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "happiness must be a valid ID")
		}
		db.Exec("UPDATE messages_outcomes SET reviewed = 1 WHERE id = ?", happinessID)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	default:
		return fiber.NewError(fiber.StatusBadRequest, "Unknown action: "+req.Action)
	}
}

// Task type constants for membership moderation emails.
const (
	TaskEmailMembershipApproved = "email_membership_approved"
	TaskEmailMembershipRejected = "email_membership_rejected"
)

// GetMembershipsMember is the response struct for individual members in GetMemberships.
type GetMembershipsMember struct {
	ID                  uint64                  `json:"id"`
	Userid              uint64                  `json:"userid"`
	Groupid             uint64                  `json:"groupid"`
	Role                string                  `json:"role"`
	Collection          string                  `json:"collection"`
	Added               *string                 `json:"added"`
	Heldby              *uint64                 `json:"heldby"`
	Fullname            *string                 `json:"fullname"`
	Firstname           *string                 `json:"firstname"`
	Lastname            *string                 `json:"lastname"`
	Displayname         string                  `json:"displayname" gorm:"-"`
	SettingsRaw         *string                 `json:"-" gorm:"column:settings"`
	Settings            *map[string]interface{} `json:"settings,omitempty" gorm:"-"`
	Emailfrequency      *int                    `json:"emailfrequency"`
	OurPostingStatus    *string                 `json:"ourpostingstatus" gorm:"column:ourPostingStatus"`
	Eventsallowed       *int                    `json:"eventsallowed"`
	Volunteeringallowed *int                    `json:"volunteeringallowed"`
	Bandate             *string                 `json:"bandate"`
	Bannedby            *uint64                 `json:"bannedby"`
	Reviewrequestedat   *string                 `json:"reviewrequestedat"`
	Reviewedat          *string                 `json:"reviewedat"`
	Reviewreason        *string                 `json:"reviewreason"`
	Engagement          *string                 `json:"engagement"`
}

// GetMemberships handles GET /memberships - list group members (moderator use).
// Query params: groupid (required for most collections), collection (default "Approved"), limit (default 100), search (optional).
// Special collection "Happiness" queries messages_outcomes instead of memberships.
//
// @Summary Get memberships for modtools
// @Tags membership
// @Produce json
// @Param groupid query integer false "Group ID"
// @Param collection query string false "Collection"
// @Param limit query integer false "Max to return"
// @Param context query integer false "Pagination cursor"
// @Success 200 {object} map[string]interface{}
// @Router /api/memberships [get]
func GetMemberships(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	groupid := uint64(c.QueryInt("groupid", 0))
	collection := c.Query("collection", "Approved")
	limit := c.QueryInt("limit", 100)

	if collection == "Happiness" {
		return getHappinessMembers(c, myid, groupid, limit)
	}

	if collection == "Spam" {
		// Member review: members flagged for review across all mod groups.
		return getSpamMembers(c, myid, groupid, limit)
	}

	if collection == "Related" {
		return getRelatedMembers(c, myid, groupid, limit)
	}

	search := c.Query("search", "")

	if groupid == 0 {
		if search == "" {
			// No group and no search — return empty list.
			return c.JSON([]GetMembershipsMember{})
		}
		// V1 parity: search across all of the mod's groups when no group selected.
		// Fall through to the search logic with groupid=0 handled below.
	} else if !isModOfGroup(myid, groupid) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator of this group")
	}
	filter := c.QueryInt("filter", 0)

	db := database.DBConn

	// Handle Banned filter separately — queries Banned collection.
	if filter == 5 {
		var members []GetMembershipsMember
		db.Raw("SELECT m.id, m.userid, m.groupid, m.role, m.collection, m.added, m.heldby, "+
			"u.fullname, u.firstname, u.lastname, m.settings, "+
			"m.emailfrequency, m.ourPostingStatus, m.eventsallowed, m.volunteeringallowed, "+
			"b.date AS bandate, b.byuser AS bannedby, "+
			"m.reviewrequestedat, m.reviewedat, m.reviewreason, u.engagement "+
			"FROM memberships m "+
			"JOIN users u ON u.id = m.userid "+
			"LEFT JOIN users_banned b ON b.userid = m.userid AND b.groupid = m.groupid "+
			"WHERE m.groupid = ? AND m.collection = 'Banned' "+
			"ORDER BY m.added DESC LIMIT ?",
			groupid, limit).Scan(&members)
		if members == nil {
			members = make([]GetMembershipsMember, 0)
		}
		enrichMembers(members)
		return c.JSON(members)
	}

	var members []GetMembershipsMember

	selectCols := "m.id, m.userid, m.groupid, m.role, m.collection, m.added, m.heldby, " +
		"u.fullname, u.firstname, u.lastname, m.settings, " +
		"m.emailfrequency, m.ourPostingStatus, m.eventsallowed, m.volunteeringallowed, " +
		"b.date AS bandate, b.byuser AS bannedby, " +
		"m.reviewrequestedat, m.reviewedat, m.reviewreason, u.engagement"
	fromClause := "FROM memberships m " +
		"JOIN users u ON u.id = m.userid " +
		"LEFT JOIN users_banned b ON b.userid = m.userid AND b.groupid = m.groupid"

	// Build filter-specific clauses.
	filterJoin := ""
	filterWhere := ""
	switch filter {
	case 1: // With comments/notes
		filterJoin = " INNER JOIN users_comments uc ON uc.userid = m.userid AND uc.groupid = m.groupid"
	case 2: // Moderation team
		filterWhere = " AND m.role IN ('" + utils.ROLE_OWNER + "', '" + utils.ROLE_MODERATOR + "')"
	case 3: // Bouncing
		filterWhere = " AND u.bouncing = 1"
	}

	if search != "" {
		// Build group filter: specific group or all of mod's groups.
		groupFilter := "m.groupid = ?"
		var groupArg interface{} = groupid
		if groupid == 0 {
			// Search across all of the mod's active groups (V1 parity).
			groupFilter = "m.groupid IN (SELECT groupid FROM memberships WHERE userid = ? AND role IN ('" + utils.ROLE_MODERATOR + "', '" + utils.ROLE_OWNER + "') AND collection = '" + utils.COLLECTION_APPROVED + "')"
			groupArg = myid
		}

		// If search is a pure number, match on userid directly (fast indexed lookup).
		// Otherwise do LIKE search on name/email.
		searchID, numErr := strconv.ParseUint(search, 10, 64)
		if numErr == nil && searchID > 0 {
			db.Raw("SELECT "+selectCols+" "+
				fromClause+filterJoin+" "+
				"WHERE "+groupFilter+" AND m.collection = ?"+filterWhere+
				" AND m.userid = ? "+
				"ORDER BY m.added DESC LIMIT ?",
				groupArg, collection, searchID, limit).Scan(&members)
		} else {
			searchPattern := "%" + search + "%"
			db.Raw("SELECT "+selectCols+" "+
				fromClause+filterJoin+
				" LEFT JOIN users_emails ue ON ue.userid = m.userid "+
				"WHERE "+groupFilter+" AND m.collection = ?"+filterWhere+
				" AND (u.fullname LIKE ? OR ue.email LIKE ?) "+
				"GROUP BY m.id "+
				"ORDER BY m.added DESC LIMIT ?",
				groupArg, collection, searchPattern, searchPattern, limit).Scan(&members)
		}
	} else {
		result := db.Raw("SELECT "+selectCols+" "+
			fromClause+filterJoin+" "+
			"WHERE m.groupid = ? AND m.collection = ?"+filterWhere+
			" ORDER BY m.added DESC LIMIT ?",
			groupid, collection, limit).Scan(&members)
		if result.Error != nil {
			stdlog.Printf("Failed to query memberships group %d collection %s: %v", groupid, collection, result.Error)
		}
	}

	if members == nil {
		members = make([]GetMembershipsMember, 0)
	}

	enrichMembers(members)

	return c.JSON(members)
}

// enrichMembers computes displayname from name fields, resolves posting status, and parses settings JSON.
func enrichMembers(members []GetMembershipsMember) {
	for i := range members {
		m := &members[i]

		// Compute displayname from fullname/firstname/lastname.
		if m.Fullname != nil && *m.Fullname != "" {
			m.Displayname = *m.Fullname
		} else {
			parts := []string{}
			if m.Firstname != nil && *m.Firstname != "" {
				parts = append(parts, *m.Firstname)
			}
			if m.Lastname != nil && *m.Lastname != "" {
				parts = append(parts, *m.Lastname)
			}
			m.Displayname = strings.Join(parts, " ")
		}

		// V1 parity: NULL ourPostingStatus defaults to MODERATED.
		// DEFAULT stays as DEFAULT — it's an explicit status (Group.php line 967).
		if m.OurPostingStatus == nil || *m.OurPostingStatus == "" {
			moderated := utils.POSTING_STATUS_MODERATED
			m.OurPostingStatus = &moderated
		}

		// Parse settings JSON.
		if m.SettingsRaw != nil && *m.SettingsRaw != "" {
			var settings map[string]interface{}
			if json.Unmarshal([]byte(*m.SettingsRaw), &settings) == nil {
				m.Settings = &settings
			}
		}
	}
}

// getSpamMembers returns members flagged for review (reviewrequestedat IS NOT NULL).
// Two-step query:
//  1. Find userids who have a flagged membership on at least one group the viewer moderates.
//  2. Return ALL flagged memberships for those users (including groups the viewer doesn't moderate),
//     so the frontend can show the full picture and disable action buttons for non-moderated groups.
//
// Used by the Member Review page.
func getSpamMembers(c *fiber.Ctx, myid uint64, groupid uint64, limit int) error {
	db := database.DBConn

	// Get all groups this user moderates.
	var modGroupIDs []uint64

	if groupid > 0 {
		if !isModOfGroup(myid, groupid) {
			return fiber.NewError(fiber.StatusForbidden, "Not a moderator of this group")
		}
		modGroupIDs = []uint64{groupid}
	} else {
		modGroupIDs = user.GetActiveModGroupIDs(myid)
	}

	if len(modGroupIDs) == 0 {
		return c.JSON(make([]GetMembershipsMember, 0))
	}

	// Return flagged memberships on groups the mod moderates.
	var members []GetMembershipsMember

	selectCols := "m.id, m.userid, m.groupid, m.role, m.collection, m.added, m.heldby, " +
		"u.fullname, u.firstname, u.lastname, m.settings, " +
		"m.emailfrequency, m.ourPostingStatus, m.eventsallowed, m.volunteeringallowed, " +
		"b.date AS bandate, b.byuser AS bannedby, " +
		"m.reviewrequestedat, m.reviewedat, m.reviewreason, u.engagement"
	fromClause := "FROM memberships m " +
		"JOIN users u ON u.id = m.userid " +
		"LEFT JOIN users_banned b ON b.userid = m.userid AND b.groupid = m.groupid"

	// Show members where reviewrequestedat is set AND either never
	// reviewed or the review is stale (more than 31 days old).
	result := db.Raw("SELECT "+selectCols+" "+
		fromClause+" "+
		"WHERE m.groupid IN ? AND m.reviewrequestedat IS NOT NULL "+
		"AND (m.reviewedat IS NULL OR DATE(m.reviewedat) < DATE_SUB(NOW(), INTERVAL 31 DAY)) "+
		"ORDER BY m.userid DESC LIMIT ?",
		modGroupIDs, limit).Scan(&members)
	if result.Error != nil {
		stdlog.Printf("Failed to query spam members for user %d: %v", myid, result.Error)
	}

	if members == nil {
		members = make([]GetMembershipsMember, 0)
	}

	enrichMembers(members)

	return c.JSON(members)
}

// getRelatedMembers returns pairs of users who appear to be related (same person / same household)
// based on the users_related table. Returns IDs only — frontend fetches user details from stores.
// V1 parity: User.php listRelated() filtering logic (deleted, no logins → auto-notified).
func getRelatedMembers(c *fiber.Ctx, myid uint64, groupid uint64, limit int) error {
	db := database.DBConn

	var modGroupIDs []uint64
	if groupid > 0 {
		if !isModOfGroup(myid, groupid) {
			return fiber.NewError(fiber.StatusForbidden, "Not a moderator of this group")
		}
		modGroupIDs = []uint64{groupid}
	} else {
		modGroupIDs = user.GetActiveModGroupIDs(myid)
	}

	if len(modGroupIDs) == 0 {
		return c.JSON(make([]fiber.Map, 0))
	}

	// Query related pairs where at least one user is in a modded group.
	// V1 parity: user1 < user2, notified = 0.
	type relatedRow struct {
		ID    uint64 `gorm:"column:id"`
		User1 uint64 `gorm:"column:user1"`
		User2 uint64 `gorm:"column:user2"`
	}

	var rows []relatedRow
	db.Raw("SELECT DISTINCT id, user1, user2 FROM ("+
		"SELECT users_related.id, user1, user2 FROM users_related "+
		"INNER JOIN memberships ON users_related.user1 = memberships.userid "+
		"INNER JOIN users u1 ON users_related.user1 = u1.id AND u1.deleted IS NULL AND u1.systemrole = 'User' "+
		"WHERE user1 < user2 AND notified = 0 AND memberships.groupid IN ? "+
		"UNION "+
		"SELECT users_related.id, user1, user2 FROM users_related "+
		"INNER JOIN memberships ON users_related.user2 = memberships.userid "+
		"INNER JOIN users u2 ON users_related.user2 = u2.id AND u2.deleted IS NULL AND u2.systemrole = 'User' "+
		"WHERE user1 < user2 AND notified = 0 AND memberships.groupid IN ? "+
		") t ORDER BY id DESC LIMIT ?", modGroupIDs, modGroupIDs, limit).Scan(&rows)

	if len(rows) == 0 {
		return c.JSON(make([]fiber.Map, 0))
	}

	// V1 parity: filter out pairs where either user has no logins (can't log in).
	// The SQL JOINs already filter deleted users and non-User systemroles.
	// Check logins in bulk.
	uidSet := make(map[uint64]bool)
	for _, r := range rows {
		uidSet[r.User1] = true
		uidSet[r.User2] = true
	}
	uidList := make([]uint64, 0, len(uidSet))
	for uid := range uidSet {
		uidList = append(uidList, uid)
	}

	type loginCount struct {
		Userid uint64 `gorm:"column:userid"`
		Count  int    `gorm:"column:count"`
	}
	var loginCounts []loginCount
	db.Raw("SELECT userid, COUNT(*) as count FROM users_logins WHERE userid IN ? GROUP BY userid", uidList).Scan(&loginCounts)
	hasLogins := make(map[uint64]bool)
	for _, lc := range loginCounts {
		if lc.Count > 0 {
			hasLogins[lc.Userid] = true
		}
	}

	result := make([]fiber.Map, 0, len(rows))
	for _, r := range rows {
		if !hasLogins[r.User1] || !hasLogins[r.User2] {
			// Auto-mark as notified since these are not actionable.
			db.Exec("UPDATE users_related SET notified = 1 WHERE id = ?", r.ID)
			continue
		}

		result = append(result, fiber.Map{
			"id":    r.ID,
			"user1": r.User1,
			"user2": r.User2,
		})
	}

	return c.JSON(result)
}

// HappinessMember is the response struct for happiness/feedback items.
type HappinessMember struct {
	ID        uint64          `json:"id"`
	Timestamp string          `json:"timestamp"`
	Outcome   *string         `json:"outcome"`
	Happiness *string         `json:"happiness"`
	Comments  *string         `json:"comments"`
	Reviewed  int             `json:"reviewed"`
	Fromuser  uint64          `json:"fromuser"`
	Groupid   uint64          `json:"groupid"`
	User      HappinessUser   `json:"user"`
	Message   HappinessMsg    `json:"message"`
}

// HappinessUser is the user info embedded in happiness results.
type HappinessUser struct {
	ID          uint64  `json:"id"`
	Displayname string  `json:"displayname"`
	Email       *string `json:"email"`
}

// HappinessMsg is the message info embedded in happiness results.
type HappinessMsg struct {
	ID      uint64  `json:"id"`
	Subject *string `json:"subject"`
}

// Rating is the response struct for user ratings in the feedback page.
type Rating struct {
	ID               uint64  `json:"id"`
	Rater            uint64  `json:"rater"`
	Ratee            uint64  `json:"ratee"`
	Rating           *string `json:"rating"`
	Reason           *string `json:"reason"`
	Text             *string `json:"text"`
	Visible          bool    `json:"visible"`
	Timestamp        string  `json:"timestamp"`
	Reviewrequired   int     `json:"reviewrequired"`
	Groupid          uint64  `json:"groupid"`
	Raterdisplayname string  `json:"raterdisplayname"`
	Rateedisplayname string  `json:"rateedisplayname"`
}

// ratingRow is the raw DB row for ratings.
type ratingRow struct {
	ID               uint64  `gorm:"column:id"`
	Rater            uint64  `gorm:"column:rater"`
	Ratee            uint64  `gorm:"column:ratee"`
	Rating           *string `gorm:"column:rating"`
	Reason           *string `gorm:"column:reason"`
	Text             *string `gorm:"column:text"`
	Visible          bool    `gorm:"column:visible"`
	Timestamp        string  `gorm:"column:timestamp"`
	Reviewrequired   int     `gorm:"column:reviewrequired"`
	Groupid          uint64  `gorm:"column:groupid"`
	Raterdisplayname string  `gorm:"column:raterdisplayname"`
	Rateedisplayname string  `gorm:"column:rateedisplayname"`
}

// HappinessResponse wraps happiness members and ratings.
type HappinessResponse struct {
	Members []HappinessMember `json:"members"`
	Ratings []Rating          `json:"ratings"`
}

// happinessRow is the raw DB row before assembly.
type happinessRow struct {
	ID        uint64  `gorm:"column:id"`
	Timestamp string  `gorm:"column:timestamp"`
	Msgid     uint64  `gorm:"column:msgid"`
	Outcome   *string `gorm:"column:outcome"`
	Happiness *string `gorm:"column:happiness"`
	Comments  *string `gorm:"column:comments"`
	Reviewed  int     `gorm:"column:reviewed"`
	Fromuser  uint64  `gorm:"column:fromuser"`
	Groupid   uint64  `gorm:"column:groupid"`
	Subject   *string `gorm:"column:subject"`
}

// Auto-generated outcome comments to filter out (not real feedback).
var happinessFilterComments = []string{
	"Sorry, this is no longer available.",
	"Thanks, this has now been taken.",
	"Thanks, I'm no longer looking for this.",
	"Sorry, this has now been taken.",
	"Thanks for the interest, but this has now been taken.",
	"Thanks, these have now been taken.",
	"Thanks, this has now been received.",
	"Withdrawn on user unsubscribe",
	"Auto-Expired",
}

// getHappinessMembers handles the Happiness collection - queries messages_outcomes.
func getHappinessMembers(c *fiber.Ctx, myid uint64, groupid uint64, limit int) error {
	db := database.DBConn

	// Determine which group IDs to query.
	var groupIDs []uint64
	if groupid > 0 {
		if !isModOfGroup(myid, groupid) {
			return fiber.NewError(fiber.StatusForbidden, "Not a moderator of this group")
		}
		groupIDs = []uint64{groupid}
	} else {
		// No group specified - get all groups where caller is a mod.
		groupIDs = user.GetActiveModGroupIDs(myid)
		if len(groupIDs) == 0 {
			return c.JSON([]HappinessMember{})
		}
	}

	filter := c.Query("filter", "")

	// Build filter clause for happiness level.
	filterClause := ""
	switch filter {
	case "Happy":
		filterClause = " AND mo.happiness = 'Happy'"
	case "Unhappy":
		filterClause = " AND mo.happiness = 'Unhappy'"
	case "Fine":
		filterClause = " AND (mo.happiness IS NULL OR mo.happiness = 'Fine')"
	}

	// Only show recent outcomes (last 31 days).
	start := time.Now().AddDate(0, 0, -31).Format("2006-01-02")

	// Build the comments filter to exclude auto-generated messages.
	commentsFilter := " AND mo.comments IS NOT NULL"
	for i := range happinessFilterComments {
		if i == 0 {
			commentsFilter += " AND mo.comments NOT IN (?"
		} else {
			commentsFilter += ", ?"
		}
	}
	commentsFilter += ")"

	// Build group ID placeholders.
	groupPlaceholders := make([]string, len(groupIDs))
	groupArgs := make([]interface{}, len(groupIDs))
	for i, gid := range groupIDs {
		groupPlaceholders[i] = "?"
		groupArgs[i] = gid
	}
	groupIn := strings.Join(groupPlaceholders, ",")

	// Build query args in order.
	args := make([]interface{}, 0, len(groupIDs)+len(happinessFilterComments)+2)
	args = append(args, groupArgs...)
	args = append(args, start)
	for _, comment := range happinessFilterComments {
		args = append(args, comment)
	}
	args = append(args, start)
	args = append(args, limit)

	sql := fmt.Sprintf(
		"SELECT mo.id, mo.timestamp, mo.msgid, mo.outcome, mo.happiness, mo.comments, mo.reviewed, "+
			"m.fromuser, mg.groupid, m.subject "+
			"FROM messages_outcomes mo "+
			"INNER JOIN messages_groups mg ON mg.msgid = mo.msgid AND mg.groupid IN (%s) "+
			"INNER JOIN messages m ON m.id = mo.msgid "+
			"WHERE mo.timestamp > ?"+
			"%s%s"+
			" AND mg.arrival > ?"+
			" ORDER BY mo.reviewed ASC, mo.timestamp DESC, mo.id DESC LIMIT ?",
		groupIn, commentsFilter+filterClause, "")

	var rows []happinessRow
	db.Raw(sql, args...).Scan(&rows)

	if rows == nil {
		ratings := getVisibleRatings(db, groupIDs)
		return c.JSON(HappinessResponse{Members: []HappinessMember{}, Ratings: ratings})
	}

	// Collect unique user IDs for batch lookup.
	userIDSet := make(map[uint64]bool)
	for _, r := range rows {
		userIDSet[r.Fromuser] = true
	}

	// Fetch user display names.
	type userInfo struct {
		ID       uint64  `gorm:"column:id"`
		Fullname *string `gorm:"column:fullname"`
	}
	userIDs := make([]uint64, 0, len(userIDSet))
	for uid := range userIDSet {
		userIDs = append(userIDs, uid)
	}
	var users []userInfo
	if len(userIDs) > 0 {
		db.Raw("SELECT id, fullname FROM users WHERE id IN ?", userIDs).Scan(&users)
	}
	userMap := make(map[uint64]*userInfo)
	for i := range users {
		userMap[users[i].ID] = &users[i]
	}

	// Fetch preferred emails for each user.
	type emailInfo struct {
		Userid    uint64 `gorm:"column:userid"`
		Email     string `gorm:"column:email"`
		Preferred int    `gorm:"column:preferred"`
	}
	var emails []emailInfo
	if len(userIDs) > 0 {
		db.Raw("SELECT userid, email, preferred FROM users_emails WHERE userid IN ? ORDER BY preferred DESC",
			userIDs).Scan(&emails)
	}
	emailMap := make(map[uint64]string)
	for _, e := range emails {
		if _, ok := emailMap[e.Userid]; !ok || e.Preferred == 1 {
			emailMap[e.Userid] = e.Email
		}
	}

	// Assemble results, deduplicating by msgid.
	seenMsgids := make(map[uint64]bool)
	results := make([]HappinessMember, 0, len(rows))
	for _, r := range rows {
		if seenMsgids[r.Msgid] {
			continue
		}
		seenMsgids[r.Msgid] = true

		displayname := ""
		if ui, ok := userMap[r.Fromuser]; ok && ui.Fullname != nil {
			displayname = *ui.Fullname
		}
		var email *string
		if e, ok := emailMap[r.Fromuser]; ok {
			email = &e
		}

		results = append(results, HappinessMember{
			ID:        r.ID,
			Timestamp: r.Timestamp,
			Outcome:   r.Outcome,
			Happiness: r.Happiness,
			Comments:  r.Comments,
			Reviewed:  r.Reviewed,
			Fromuser:  r.Fromuser,
			Groupid:   r.Groupid,
			User: HappinessUser{
				ID:          r.Fromuser,
				Displayname: displayname,
				Email:       email,
			},
			Message: HappinessMsg{
				ID:      r.Msgid,
				Subject: r.Subject,
			},
		})
	}

	// Fetch visible ratings for the moderator's groups.
	ratings := getVisibleRatings(db, groupIDs)

	return c.JSON(HappinessResponse{Members: results, Ratings: ratings})
}

// getVisibleRatings returns ratings visible to the moderator for the given groups.
// Both rater and ratee must be members of the same group.
func getVisibleRatings(db *gorm.DB, groupIDs []uint64) []Rating {
	if len(groupIDs) == 0 {
		return []Rating{}
	}

	since := time.Now().AddDate(0, 0, -7).Format("2006-01-02")

	groupPlaceholders := make([]string, len(groupIDs))
	groupArgs := make([]interface{}, len(groupIDs))
	for i, gid := range groupIDs {
		groupPlaceholders[i] = "?"
		groupArgs[i] = gid
	}
	groupIn := strings.Join(groupPlaceholders, ",")

	args := make([]interface{}, 0, len(groupArgs)*2+1)
	args = append(args, since)
	args = append(args, groupArgs...)
	args = append(args, groupArgs...)

	sql := fmt.Sprintf(
		"SELECT ratings.id, ratings.rater, ratings.ratee, ratings.rating, ratings.reason, "+
			"ratings.text, ratings.visible, ratings.timestamp, ratings.reviewrequired, "+
			"m1.groupid, "+
			"CASE WHEN u1.fullname IS NOT NULL THEN u1.fullname ELSE CONCAT(u1.firstname, ' ', u1.lastname) END AS raterdisplayname, "+
			"CASE WHEN u2.fullname IS NOT NULL THEN u2.fullname ELSE CONCAT(u2.firstname, ' ', u2.lastname) END AS rateedisplayname "+
			"FROM ratings "+
			"INNER JOIN memberships m1 ON m1.userid = ratings.rater "+
			"INNER JOIN memberships m2 ON m2.userid = ratings.ratee "+
			"INNER JOIN users u1 ON ratings.rater = u1.id "+
			"INNER JOIN users u2 ON ratings.ratee = u2.id "+
			"WHERE ratings.timestamp >= ? "+
			"AND m1.groupid IN (%s) "+
			"AND m2.groupid IN (%s) "+
			"AND m1.groupid = m2.groupid "+
			"AND ratings.rating IS NOT NULL "+
			"GROUP BY ratings.id "+
			"ORDER BY ratings.timestamp DESC",
		groupIn, groupIn)

	var rows []ratingRow
	db.Raw(sql, args...).Scan(&rows)

	if rows == nil {
		return []Rating{}
	}

	ratings := make([]Rating, len(rows))
	for i, r := range rows {
		ratings[i] = Rating{
			ID:               r.ID,
			Rater:            r.Rater,
			Ratee:            r.Ratee,
			Rating:           r.Rating,
			Reason:           r.Reason,
			Text:             r.Text,
			Visible:          r.Visible,
			Timestamp:        r.Timestamp,
			Reviewrequired:   r.Reviewrequired,
			Groupid:          r.Groupid,
			Raterdisplayname: r.Raterdisplayname,
			Rateedisplayname: r.Rateedisplayname,
		}
	}

	return ratings
}

// PutMembershipsRequest is the body for PUT /memberships (join group).
type PutMembershipsRequest struct {
	Userid  uint64 `json:"userid"`
	Groupid uint64 `json:"groupid"`
	Manual  *bool  `json:"manual"`
}

// PutMemberships handles PUT /memberships - user joins a group.
// FD sends: {userid, groupid, manual}
// Only self-join is supported here (userid must match authenticated user).
func PutMemberships(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PutMembershipsRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Groupid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "groupid is required")
	}

	// Default userid to the authenticated user if not provided.
	userid := req.Userid
	if userid == 0 {
		userid = myid
	}

	// FD only does self-join. Non-self joins require moderator permissions which
	// are not yet supported here.
	if userid != myid {
		return fiber.NewError(fiber.StatusForbidden, "Cannot add another user")
	}

	db := database.DBConn

	// Check the group exists.
	var groupExists int64
	db.Raw("SELECT COUNT(*) FROM `groups` WHERE id = ?", req.Groupid).Scan(&groupExists)
	if groupExists == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Group not found")
	}

	// Check if already a member.
	var existingRole string
	db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ?",
		userid, req.Groupid).Scan(&existingRole)

	if existingRole != "" {
		// Already a member - just return success (joining shouldn't demote).
		return c.JSON(fiber.Map{"ret": 0, "status": "Success", "addedto": "Approved"})
	}

	// Check if banned - unban on explicit join.
	var bannedCount int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Banned'",
		userid, req.Groupid).Scan(&bannedCount)
	if bannedCount > 0 {
		db.Exec("DELETE FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Banned'",
			userid, req.Groupid)
	}

	// Get an email ID for the user.
	var emailid uint64
	db.Raw("SELECT id FROM users_emails WHERE userid = ? ORDER BY preferred DESC, id ASC LIMIT 1",
		userid).Scan(&emailid)

	// Insert membership as approved member.
	db.Exec("INSERT INTO memberships (userid, groupid, role, collection) VALUES (?, ?, ?, ?)",
		userid, req.Groupid, utils.ROLE_MEMBER, utils.COLLECTION_APPROVED)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "addedto": utils.COLLECTION_APPROVED})
}

// DeleteMembershipsRequest is for DELETE /memberships (leave group).
type DeleteMembershipsRequest struct {
	Userid  uint64 `json:"userid"`
	Groupid uint64 `json:"groupid"`
}

// DeleteMemberships handles DELETE /memberships - user leaves a group.
// Frontend $delv2 sends JSON body (BaseAPI.js line 166 JSON-stringifies config.params for non-GET/POST).
func DeleteMemberships(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req DeleteMembershipsRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Groupid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "groupid is required")
	}

	userid := req.Userid
	if userid == 0 {
		userid = myid
	}

	// Self-leave is always allowed. Non-self removals require mod/owner of the group.
	db := database.DBConn
	if userid != myid {
		if !isModOfGroup(myid, req.Groupid) {
			return fiber.NewError(fiber.StatusForbidden, "Not a moderator of this group")
		}
		logMembershipAction(db, "User", "Deleted", req.Groupid, userid, myid, "")
	}

	// Remove the membership.
	result := db.Exec("DELETE FROM memberships WHERE userid = ? AND groupid = ? AND collection = ?",
		userid, req.Groupid, utils.COLLECTION_APPROVED)

	if result.RowsAffected == 0 {
		// Not a member - still return success (idempotent).
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// PatchMembershipsRequest is for PATCH /memberships (update settings).
type PatchMembershipsRequest struct {
	Userid              uint64  `json:"userid"`
	ID                  uint64  `json:"id"`
	Groupid             uint64  `json:"groupid"`
	Emailfrequency      *int    `json:"emailfrequency"`
	Eventsallowed       *int    `json:"eventsallowed"`
	Volunteeringallowed *int    `json:"volunteeringallowed"`
	OurPostingStatus    *string `json:"ourPostingStatus"`
}

// PatchMemberships handles PATCH /memberships - update membership settings.
// Users can update their own settings. Moderators can update ourPostingStatus
// and emailfrequency for members of groups they moderate (stdmsg side effects).
func PatchMemberships(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PatchMembershipsRequest
	if err := c.BodyParser(&req); err != nil {
		stdlog.Printf("[PatchMemberships] BodyParser error for user %d: %v body=%q", myid, err, string(c.Body()))
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Groupid == 0 {
		stdlog.Printf("[PatchMemberships] Missing groupid for user %d: parsed=%+v body=%q", myid, req, string(c.Body()))
		return fiber.NewError(fiber.StatusBadRequest, "groupid is required")
	}

	userid := req.Userid
	if userid == 0 {
		userid = req.ID
	}
	if userid == 0 {
		userid = myid
	}

	// Users can update their own settings. Moderators can update settings for
	// members of groups they moderate (e.g. stdmsg newmodstatus/newdelstatus).
	if userid != myid {
		if !isModOfGroup(myid, req.Groupid) {
			return fiber.NewError(fiber.StatusForbidden, "Cannot modify another user's settings")
		}
	}

	db := database.DBConn

	// Verify the membership exists.
	var membershipExists int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ? AND collection = ?",
		userid, req.Groupid, utils.COLLECTION_APPROVED).Scan(&membershipExists)
	if membershipExists == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Not a member of this group")
	}

	// Update whichever settings were provided.
	if req.Emailfrequency != nil {
		db.Exec("UPDATE memberships SET emailfrequency = ? WHERE userid = ? AND groupid = ?",
			*req.Emailfrequency, userid, req.Groupid)
		logMembershipAction(db, log.LOG_TYPE_USER, log.LOG_SUBTYPE_OUR_EMAIL_FREQUENCY, req.Groupid, userid, myid,
			fmt.Sprintf("emailfrequency=%d", *req.Emailfrequency))
	}

	if req.Eventsallowed != nil {
		db.Exec("UPDATE memberships SET eventsallowed = ? WHERE userid = ? AND groupid = ?",
			*req.Eventsallowed, userid, req.Groupid)
	}

	if req.Volunteeringallowed != nil {
		db.Exec("UPDATE memberships SET volunteeringallowed = ? WHERE userid = ? AND groupid = ?",
			*req.Volunteeringallowed, userid, req.Groupid)
	}

	if req.OurPostingStatus != nil {
		// ourPostingStatus is mod-only — users must not change their own moderation status.
		if !isModOfGroup(myid, req.Groupid) {
			return fiber.NewError(fiber.StatusForbidden, "Only moderators can change posting status")
		}
		db.Exec("UPDATE memberships SET ourPostingStatus = ? WHERE userid = ? AND groupid = ?",
			*req.OurPostingStatus, userid, req.Groupid)
		logMembershipAction(db, log.LOG_TYPE_USER, log.LOG_SUBTYPE_OUR_POSTING_STATUS, req.Groupid, userid, myid,
			*req.OurPostingStatus)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
