package group

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// GroupWork represents per-group work counts for a moderator.
type GroupWork struct {
	Groupid        uint64 `json:"groupid"`
	Pending        int64  `json:"pending"`
	Spam           int64  `json:"spam"`
	Pendingmembers int64  `json:"pendingmembers"`
	Spammembers    int64  `json:"spammembers"`
}

// GetGroupWork returns per-group work counts for the logged-in moderator.
//
// @Summary Get per-group work counts
// @Tags group
// @Produce json
// @Security BearerAuth
// @Success 200 {array} GroupWork
// @Router /api/group/work [get]
func GetGroupWork(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	db := database.DBConn

	// Get groups where user is moderator or owner.
	type GroupIDRow struct {
		Groupid uint64
	}

	var modGroups []GroupIDRow
	db.Raw("SELECT groupid FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner') AND collection = 'Approved'", myid).Scan(&modGroups)

	if len(modGroups) == 0 {
		return c.JSON([]GroupWork{})
	}

	var groupIDs []uint64
	for _, g := range modGroups {
		groupIDs = append(groupIDs, g.Groupid)
	}

	// Get per-group counts in parallel.
	type countRow struct {
		Groupid uint64 `json:"groupid"`
		Count   int64  `json:"count"`
	}

	var pending, spam, pendingMembers, spamMembers []countRow

	var pendingCh = make(chan []countRow, 1)
	var spamCh = make(chan []countRow, 1)
	var pendingMembersCh = make(chan []countRow, 1)
	var spamMembersCh = make(chan []countRow, 1)

	go func() {
		var rows []countRow
		db.Raw("SELECT mg.groupid, COUNT(*) as count FROM messages_groups mg "+
			"INNER JOIN messages m ON m.id = mg.msgid "+
			"WHERE mg.groupid IN ? AND mg.collection = 'Pending' AND mg.deleted = 0 AND m.fromuser IS NOT NULL "+
			"GROUP BY mg.groupid", groupIDs).Scan(&rows)
		pendingCh <- rows
	}()

	go func() {
		var rows []countRow
		db.Raw("SELECT mg.groupid, COUNT(*) as count FROM messages_groups mg "+
			"INNER JOIN messages m ON m.id = mg.msgid "+
			"WHERE mg.groupid IN ? AND mg.collection = 'Spam' AND mg.deleted = 0 AND m.fromuser IS NOT NULL "+
			"GROUP BY mg.groupid", groupIDs).Scan(&rows)
		spamCh <- rows
	}()

	go func() {
		var rows []countRow
		db.Raw("SELECT groupid, COUNT(*) as count FROM memberships "+
			"WHERE groupid IN ? AND collection = 'Pending' "+
			"GROUP BY groupid", groupIDs).Scan(&rows)
		pendingMembersCh <- rows
	}()

	go func() {
		var rows []countRow
		db.Raw("SELECT groupid, COUNT(*) as count FROM memberships "+
			"WHERE groupid IN ? AND collection = 'Spam' "+
			"GROUP BY groupid", groupIDs).Scan(&rows)
		spamMembersCh <- rows
	}()

	pending = <-pendingCh
	spam = <-spamCh
	pendingMembers = <-pendingMembersCh
	spamMembers = <-spamMembersCh

	// Build result map keyed by groupid.
	workMap := make(map[uint64]*GroupWork)
	for _, gid := range groupIDs {
		workMap[gid] = &GroupWork{Groupid: gid}
	}

	for _, r := range pending {
		if w, ok := workMap[r.Groupid]; ok {
			w.Pending = r.Count
		}
	}
	for _, r := range spam {
		if w, ok := workMap[r.Groupid]; ok {
			w.Spam = r.Count
		}
	}
	for _, r := range pendingMembers {
		if w, ok := workMap[r.Groupid]; ok {
			w.Pendingmembers = r.Count
		}
	}
	for _, r := range spamMembers {
		if w, ok := workMap[r.Groupid]; ok {
			w.Spammembers = r.Count
		}
	}

	// Convert to flat array.
	result := make([]GroupWork, 0, len(workMap))
	for _, w := range workMap {
		result = append(result, *w)
	}

	return c.JSON(result)
}
