package membership

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
)

// isModOfGroup checks if the caller is a Moderator or Owner of the given group,
// or has Admin/Support system role.
func isModOfGroup(myid uint64, groupid uint64) bool {
	db := database.DBConn
	if db == nil {
		return false
	}

	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)
	if systemrole == utils.SYSTEMROLE_SUPPORT || systemrole == utils.SYSTEMROLE_ADMIN {
		return true
	}

	if groupid == 0 {
		return false
	}

	var role string
	db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Approved'",
		myid, groupid).Scan(&role)
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
// Delete Approved Member, Ban, Unban, ReviewHold, ReviewRelease, HappinessReviewed.
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
		db.Exec("UPDATE memberships SET heldby = ? WHERE userid = ? AND groupid = ?",
			myid, req.Userid, req.Groupid)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "Release":
		db.Exec("UPDATE memberships SET heldby = NULL WHERE userid = ? AND groupid = ?",
			req.Userid, req.Groupid)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "Approve", "Leave Approved Member":
		db.Exec("UPDATE memberships SET collection = 'Approved', heldby = NULL WHERE userid = ? AND groupid = ?",
			req.Userid, req.Groupid)

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
		db.Exec("DELETE FROM memberships WHERE userid = ? AND groupid = ? AND collection IN ('Pending', 'Approved')",
			req.Userid, req.Groupid)

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
		db.Exec("DELETE FROM memberships WHERE userid = ? AND groupid = ? AND collection IN ('Pending', 'Approved')",
			req.Userid, req.Groupid)
		// Add banned record.
		db.Exec("INSERT INTO memberships (userid, groupid, role, collection) VALUES (?, ?, 'Member', 'Banned')",
			req.Userid, req.Groupid)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "Unban":
		db.Exec("DELETE FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Banned'",
			req.Userid, req.Groupid)
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
	ID                  uint64  `json:"id"`
	Userid              uint64  `json:"userid"`
	Groupid             uint64  `json:"groupid"`
	Role                string  `json:"role"`
	Collection          string  `json:"collection"`
	Added               *string `json:"added"`
	Heldby              *uint64 `json:"heldby"`
	Fullname            *string `json:"fullname"`
	Firstname           *string `json:"firstname"`
	Lastname            *string `json:"lastname"`
	Emailfrequency      *int    `json:"emailfrequency"`
	OurPostingStatus    *string `json:"ourpostingstatus"`
	Eventsallowed       *int    `json:"eventsallowed"`
	Volunteeringallowed *int    `json:"volunteeringallowed"`
	Bandate             *string `json:"bandate"`
	Bannedby            *uint64 `json:"bannedby"`
}

// GetMemberships handles GET /memberships - list group members (moderator use).
// Query params: groupid (required for most collections), collection (default "Approved"), limit (default 100), search (optional).
// Special collection "Happiness" queries messages_outcomes instead of memberships.
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

	if groupid == 0 {
		// No group selected - return empty list so ModTools pages
		// degrade gracefully, matching PHP V1 behaviour.
		return c.JSON([]GetMembershipsMember{})
	}

	if !isModOfGroup(myid, groupid) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator of this group")
	}

	search := c.Query("search", "")

	db := database.DBConn

	var members []GetMembershipsMember

	selectCols := "m.id, m.userid, m.groupid, m.role, m.collection, m.added, m.heldby, " +
		"u.fullname, u.firstname, u.lastname, " +
		"m.emailfrequency, m.ourPostingStatus, m.eventsallowed, m.volunteeringallowed, " +
		"b.date AS bandate, b.byuser AS bannedby"
	fromClause := "FROM memberships m " +
		"JOIN users u ON u.id = m.userid " +
		"LEFT JOIN users_banned b ON b.userid = m.userid AND b.groupid = m.groupid"

	if search != "" {
		searchPattern := "%" + search + "%"
		db.Raw("SELECT "+selectCols+" "+
			fromClause+" "+
			"WHERE m.groupid = ? AND m.collection = ? "+
			"AND (u.fullname LIKE ? OR EXISTS (SELECT 1 FROM users_emails WHERE userid = m.userid AND email LIKE ?)) "+
			"ORDER BY m.added DESC LIMIT ?",
			groupid, collection, searchPattern, searchPattern, limit).Scan(&members)
	} else {
		db.Raw("SELECT "+selectCols+" "+
			fromClause+" "+
			"WHERE m.groupid = ? AND m.collection = ? "+
			"ORDER BY m.added DESC LIMIT ?",
			groupid, collection, limit).Scan(&members)
	}

	if members == nil {
		members = make([]GetMembershipsMember, 0)
	}

	return c.JSON(members)
}

// getSpamMembers returns members flagged for review (reviewrequestedat IS NOT NULL)
// across all groups the moderator is on. Used by the Member Review page.
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
		db.Raw("SELECT groupid FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner')", myid).Scan(&modGroupIDs)
	}

	if len(modGroupIDs) == 0 {
		return c.JSON(make([]GetMembershipsMember, 0))
	}

	var members []GetMembershipsMember

	selectCols := "m.id, m.userid, m.groupid, m.role, m.collection, m.added, m.heldby, " +
		"u.fullname, u.firstname, u.lastname, " +
		"m.emailfrequency, m.ourPostingStatus, m.eventsallowed, m.volunteeringallowed, " +
		"b.date AS bandate, b.byuser AS bannedby"
	fromClause := "FROM memberships m " +
		"JOIN users u ON u.id = m.userid " +
		"LEFT JOIN users_banned b ON b.userid = m.userid AND b.groupid = m.groupid"

	db.Raw("SELECT "+selectCols+" "+
		fromClause+" "+
		"WHERE m.groupid IN ? AND m.reviewrequestedat IS NOT NULL "+
		"AND (m.reviewedat IS NULL OR DATE(m.reviewedat) < DATE_SUB(NOW(), INTERVAL 31 DAY)) "+
		"ORDER BY m.added DESC LIMIT ?",
		modGroupIDs, limit).Scan(&members)

	if members == nil {
		members = make([]GetMembershipsMember, 0)
	}

	return c.JSON(members)
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
		db.Raw("SELECT groupid FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner') AND collection = 'Approved'",
			myid).Scan(&groupIDs)
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

	// Only show recent outcomes (last 31 days, matching PHP RECENTPOSTS).
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
		return c.JSON([]HappinessMember{})
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

	return c.JSON(results)
}
