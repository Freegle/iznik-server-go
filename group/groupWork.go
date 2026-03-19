package group

import (
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
)

// GroupWork represents per-group work counts for a moderator.
// Active groups get primary fields (red badges), inactive/backup groups get
// "other" fields (blue badges).
type GroupWork struct {
	Groupid             uint64 `json:"groupid"`
	Pending             int64  `json:"pending"`
	Pendingother        int64  `json:"pendingother"`
	Spam                int64  `json:"spam"`
	Pendingmembers      int64  `json:"pendingmembers"`
	Pendingmembersother int64  `json:"pendingmembersother"`
	Spammembers         int64  `json:"spammembers"`
	Spammembersother    int64  `json:"spammembersother"`
	Pendingevents       int64  `json:"pendingevents"`
	Pendingvolunteering int64  `json:"pendingvolunteering"`
	Editreview          int64  `json:"editreview"`
	Pendingadmins       int64  `json:"pendingadmins"`
	Happiness           int64  `json:"happiness"`
	Relatedmembers      int64  `json:"relatedmembers"`
	Chatreview          int64  `json:"chatreview"`
	Chatreviewother     int64  `json:"chatreviewother"`
}

// isActiveModForGroup checks membership settings JSON for the active flag.
// Defaults to active=1 unless explicitly set otherwise.
func isActiveModForGroup(settingsJSON *string) bool {
	if settingsJSON == nil || *settingsJSON == "" {
		return true
	}
	var settings map[string]interface{}
	if err := json.Unmarshal([]byte(*settingsJSON), &settings); err != nil {
		return true
	}
	if active, ok := settings["active"]; ok {
		switch v := active.(type) {
		case float64:
			return v != 0
		case bool:
			return v
		}
	}
	if showmessages, ok := settings["showmessages"]; ok {
		switch v := showmessages.(type) {
		case float64:
			return v != 0
		case bool:
			return v
		}
	}
	return true
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
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	// Get all mod/owner memberships with settings to determine active/inactive.
	type membershipRow struct {
		Groupid  uint64  `json:"groupid"`
		Settings *string `json:"settings"`
	}
	var memberships []membershipRow
	db.Raw("SELECT groupid, settings FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner') AND collection = 'Approved'",
		myid).Scan(&memberships)

	if len(memberships) == 0 {
		return c.JSON([]GroupWork{})
	}

	// Split into active and inactive group IDs.
	var allGroupIDs, activeGroupIDs, inactiveGroupIDs []uint64
	activeMap := make(map[uint64]bool)
	for _, m := range memberships {
		allGroupIDs = append(allGroupIDs, m.Groupid)
		if isActiveModForGroup(m.Settings) {
			activeGroupIDs = append(activeGroupIDs, m.Groupid)
			activeMap[m.Groupid] = true
		} else {
			inactiveGroupIDs = append(inactiveGroupIDs, m.Groupid)
		}
	}

	// Build result map keyed by groupid.
	workMap := make(map[uint64]*GroupWork)
	for _, gid := range allGroupIDs {
		workMap[gid] = &GroupWork{Groupid: gid}
	}

	// countRow is used for GROUP BY groupid queries.
	type countRow struct {
		Groupid uint64
		Count   int64
	}

	// heldCountRow adds a held flag for pending/spam splitting.
	type heldCountRow struct {
		Groupid uint64
		Count   int64
		Held    int
	}

	var wg sync.WaitGroup

	// --- Pending messages: split by held status, active groups get primary/other, inactive all → pendingother ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		var rows []heldCountRow
		db.Raw("SELECT mg.groupid, COUNT(*) as count, (m.heldby IS NOT NULL) as held "+
			"FROM messages_groups mg "+
			"INNER JOIN messages m ON m.id = mg.msgid "+
			"WHERE mg.groupid IN ? AND mg.collection = 'Pending' AND mg.deleted = 0 AND m.fromuser IS NOT NULL "+
			"GROUP BY mg.groupid, held", allGroupIDs).Scan(&rows)
		for _, r := range rows {
			w := workMap[r.Groupid]
			if w == nil {
				continue
			}
			if activeMap[r.Groupid] {
				if r.Held == 0 {
					w.Pending = r.Count
				} else {
					w.Pendingother = r.Count
				}
			} else {
				w.Pendingother += r.Count
			}
		}
	}()

	// --- Spam messages: only active groups ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		if len(activeGroupIDs) == 0 {
			return
		}
		var rows []countRow
		db.Raw("SELECT mg.groupid, COUNT(*) as count FROM messages_groups mg "+
			"INNER JOIN messages m ON m.id = mg.msgid "+
			"WHERE mg.groupid IN ? AND mg.collection = 'Spam' AND mg.deleted = 0 AND m.fromuser IS NOT NULL "+
			"GROUP BY mg.groupid", activeGroupIDs).Scan(&rows)
		for _, r := range rows {
			if w := workMap[r.Groupid]; w != nil {
				w.Spam = r.Count
			}
		}
	}()

	// --- Pending members: all groups, no active/inactive split ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		var rows []countRow
		db.Raw("SELECT groupid, COUNT(*) as count FROM memberships "+
			"WHERE groupid IN ? AND collection = 'Pending' "+
			"GROUP BY groupid", allGroupIDs).Scan(&rows)
		for _, r := range rows {
			if w := workMap[r.Groupid]; w != nil {
				w.Pendingmembers = r.Count
			}
		}
	}()

	// --- Spam members: split by held, active → primary/other, inactive → other ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		var rows []heldCountRow
		db.Raw("SELECT groupid, COUNT(*) as count, (heldby IS NOT NULL) as held FROM memberships "+
			"WHERE groupid IN ? AND reviewrequestedat IS NOT NULL "+
			"AND (reviewedat IS NULL OR DATE(reviewedat) < DATE_SUB(NOW(), INTERVAL 31 DAY)) "+
			"GROUP BY groupid, held", allGroupIDs).Scan(&rows)
		for _, r := range rows {
			w := workMap[r.Groupid]
			if w == nil {
				continue
			}
			if activeMap[r.Groupid] {
				if r.Held == 0 {
					w.Spammembers = r.Count
				} else {
					w.Spammembersother = r.Count
				}
			} else {
				w.Spammembersother += r.Count
			}
		}
	}()

	// --- Pending community events: only active groups ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		if len(activeGroupIDs) == 0 {
			return
		}
		var rows []countRow
		db.Raw("SELECT ceg.groupid, COUNT(DISTINCT ce.id) as count FROM communityevents ce "+
			"INNER JOIN communityevents_groups ceg ON ceg.eventid = ce.id "+
			"INNER JOIN communityevents_dates ced ON ced.eventid = ce.id "+
			"WHERE ceg.groupid IN ? AND ce.pending = 1 AND ce.deleted = 0 AND ced.end >= NOW() "+
			"GROUP BY ceg.groupid", activeGroupIDs).Scan(&rows)
		for _, r := range rows {
			if w := workMap[r.Groupid]; w != nil {
				w.Pendingevents = r.Count
			}
		}
	}()

	// --- Pending volunteering: only active groups ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		if len(activeGroupIDs) == 0 {
			return
		}
		var rows []countRow
		db.Raw("SELECT vg.groupid, COUNT(DISTINCT v.id) as count FROM volunteering v "+
			"INNER JOIN volunteering_groups vg ON vg.volunteeringid = v.id "+
			"LEFT JOIN volunteering_dates vd ON vd.volunteeringid = v.id "+
			"WHERE vg.groupid IN ? AND v.pending = 1 AND v.deleted = 0 AND v.expired = 0 "+
			"AND (vd.end IS NULL OR vd.end >= NOW()) "+
			"GROUP BY vg.groupid", activeGroupIDs).Scan(&rows)
		for _, r := range rows {
			if w := workMap[r.Groupid]; w != nil {
				w.Pendingvolunteering = r.Count
			}
		}
	}()

	// --- Edit reviews: only active groups, last 7 days ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		if len(activeGroupIDs) == 0 {
			return
		}
		var rows []countRow
		db.Raw("SELECT mg.groupid, COUNT(DISTINCT me.msgid) as count FROM messages_edits me "+
			"INNER JOIN messages_groups mg ON mg.msgid = me.msgid "+
			"WHERE mg.groupid IN ? AND me.reviewrequired = 1 AND me.approvedat IS NULL AND me.revertedat IS NULL "+
			"AND me.timestamp > DATE_SUB(NOW(), INTERVAL 7 DAY) AND mg.deleted = 0 "+
			"GROUP BY mg.groupid", activeGroupIDs).Scan(&rows)
		for _, r := range rows {
			if w := workMap[r.Groupid]; w != nil {
				w.Editreview = r.Count
			}
		}
	}()

	// --- Pending admins: only active groups ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		if len(activeGroupIDs) == 0 {
			return
		}
		var rows []countRow
		db.Raw("SELECT groupid, COUNT(DISTINCT id) as count FROM admins "+
			"WHERE groupid IN ? AND complete IS NULL AND pending = 1 AND heldby IS NULL "+
			"GROUP BY groupid", activeGroupIDs).Scan(&rows)
		for _, r := range rows {
			if w := workMap[r.Groupid]; w != nil {
				w.Pendingadmins = r.Count
			}
		}
	}()

	// --- Happiness: only active groups ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		if len(activeGroupIDs) == 0 {
			return
		}
		hapCutoff := time.Now().AddDate(0, 0, -utils.CHAT_ACTIVE_LIMIT).Format("2006-01-02")
		var rows []countRow
		db.Raw("SELECT mg.groupid, COUNT(DISTINCT mo.id) as count FROM messages_outcomes mo "+
			"INNER JOIN messages_groups mg ON mg.msgid = mo.msgid "+
			"WHERE mo.timestamp >= ? AND mg.arrival >= ? "+
			"AND mg.groupid IN ? "+
			"AND mo.comments IS NOT NULL "+
			"AND mo.comments != 'Sorry, this is no longer available.' "+
			"AND mo.comments != 'Thanks, this has now been taken.' "+
			"AND mo.comments != 'Thanks, I''m no longer looking for this.' "+
			"AND mo.comments != 'Sorry, this has now been taken.' "+
			"AND mo.comments != 'Thanks for the interest, but this has now been taken.' "+
			"AND mo.comments != 'Thanks, these have now been taken.' "+
			"AND mo.comments != 'Thanks, this has now been received.' "+
			"AND mo.comments != 'Withdrawn on user unsubscribe' "+
			"AND mo.comments != 'Auto-Expired' "+
			"AND (mo.happiness = 'Happy' OR mo.happiness IS NULL) "+
			"AND mo.reviewed = 0 "+
			"GROUP BY mg.groupid",
			hapCutoff, hapCutoff, activeGroupIDs).Scan(&rows)
		for _, r := range rows {
			if w := workMap[r.Groupid]; w != nil {
				w.Happiness = r.Count
			}
		}
	}()

	// --- Related members: only active groups ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		if len(activeGroupIDs) == 0 {
			return
		}
		var rows []countRow
		db.Raw("SELECT groupid, COUNT(*) as count FROM ("+
			"SELECT ur.user1, m.groupid FROM users_related ur "+
			"INNER JOIN memberships m ON m.userid = ur.user1 "+
			"INNER JOIN users u1 ON ur.user1 = u1.id AND u1.deleted IS NULL AND u1.systemrole = 'User' "+
			"INNER JOIN users u2 ON ur.user2 = u2.id AND u2.deleted IS NULL AND u2.systemrole = 'User' "+
			"WHERE ur.user1 < ur.user2 AND ur.notified = 0 AND m.groupid IN ? "+
			"AND (SELECT COUNT(*) FROM users_logins WHERE userid = m.userid) > 0 "+
			"UNION "+
			"SELECT ur.user1, m.groupid FROM users_related ur "+
			"INNER JOIN memberships m ON m.userid = ur.user2 "+
			"INNER JOIN users u1 ON ur.user1 = u1.id AND u1.deleted IS NULL AND u1.systemrole = 'User' "+
			"INNER JOIN users u2 ON ur.user2 = u2.id AND u2.deleted IS NULL AND u2.systemrole = 'User' "+
			"WHERE ur.user1 < ur.user2 AND ur.notified = 0 AND m.groupid IN ? "+
			"AND (SELECT COUNT(*) FROM users_logins WHERE userid = m.userid) > 0 "+
			") t GROUP BY groupid", activeGroupIDs, activeGroupIDs).Scan(&rows)
		for _, r := range rows {
			if w := workMap[r.Groupid]; w != nil {
				w.Relatedmembers = r.Count
			}
		}
	}()

	// --- Chat review: per-group, split by active/inactive and held status ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		chatCutoff := time.Now().AddDate(0, 0, -utils.CHAT_ACTIVE_LIMIT).Format("2006-01-02")

		// Get per-group chat review counts. The recipient determines which group the review belongs to.
		// Primary: recipient is a member of the mod's group. Secondary: recipient not a member, use sender's group.
		type chatCountRow struct {
			Groupid uint64
			Count   int64
			Held    int
		}

		chatReviewByGroup := func(groupIDs []uint64) []chatCountRow {
			if len(groupIDs) == 0 {
				return nil
			}
			var rows []chatCountRow
			db.Raw("SELECT groupid, COUNT(*) as count, held FROM ("+
				"SELECT DISTINCT cm.id, "+
				"COALESCE("+
				"  (SELECT m1.groupid FROM memberships m1 INNER JOIN `groups` g ON m1.groupid = g.id AND g.type = 'Freegle' "+
				"   WHERE m1.userid = (CASE WHEN cm.userid = cr.user1 THEN cr.user2 ELSE cr.user1 END) "+
				"   AND m1.groupid IN ? LIMIT 1), "+
				"  (SELECT m2.groupid FROM memberships m2 INNER JOIN `groups` g2 ON m2.groupid = g2.id AND g2.type = 'Freegle' "+
				"   WHERE m2.userid = cm.userid AND m2.groupid IN ? LIMIT 1)"+
				") as groupid, "+
				"(cmh.userid IS NOT NULL) as held "+
				"FROM chat_messages cm "+
				"INNER JOIN chat_rooms cr ON cr.id = cm.chatid "+
				"LEFT JOIN chat_messages_held cmh ON cmh.msgid = cm.id "+
				"WHERE cm.reviewrequired = 1 AND cm.reviewrejected = 0 AND cm.date >= ? "+
				"AND ("+
				"  EXISTS (SELECT 1 FROM memberships m3 INNER JOIN `groups` g3 ON m3.groupid = g3.id AND g3.type = 'Freegle' "+
				"   WHERE m3.userid = (CASE WHEN cm.userid = cr.user1 THEN cr.user2 ELSE cr.user1 END) AND m3.groupid IN ?) "+
				"  OR (NOT EXISTS (SELECT 1 FROM memberships m4 INNER JOIN `groups` g4 ON m4.groupid = g4.id AND g4.type = 'Freegle' "+
				"   WHERE m4.userid = (CASE WHEN cm.userid = cr.user1 THEN cr.user2 ELSE cr.user1 END)) "+
				"   AND EXISTS (SELECT 1 FROM memberships m5 INNER JOIN `groups` g5 ON m5.groupid = g5.id AND g5.type = 'Freegle' "+
				"   WHERE m5.userid = cm.userid AND m5.groupid IN ?))"+
				")"+
				") sub WHERE groupid IS NOT NULL GROUP BY groupid, held",
				groupIDs, groupIDs, chatCutoff, groupIDs, groupIDs).Scan(&rows)
			return rows
		}

		// Active groups: not-held → chatreview, held → chatreviewother.
		for _, r := range chatReviewByGroup(activeGroupIDs) {
			if w := workMap[r.Groupid]; w != nil {
				if r.Held == 0 {
					w.Chatreview = r.Count
				} else {
					w.Chatreviewother = r.Count
				}
			}
		}
		// Inactive groups: all → chatreviewother.
		for _, r := range chatReviewByGroup(inactiveGroupIDs) {
			if w := workMap[r.Groupid]; w != nil {
				w.Chatreviewother += r.Count
			}
		}

		// Wider chat review: if user is eligible, count unheld messages from groups with
		// widerchatreview=1. These appear as new group entries with chatreviewother counts.
		if user.HasWiderReview(myid) {
			type widerCountRow struct {
				Groupid uint64
				Count   int64
			}
			var widerRows []widerCountRow
			db.Raw("SELECT m1.groupid, COUNT(DISTINCT cm.id) as count "+
				"FROM chat_messages cm "+
				"INNER JOIN chat_rooms cr ON cr.id = cm.chatid "+
				"LEFT JOIN chat_messages_held cmh ON cmh.msgid = cm.id "+
				"INNER JOIN memberships m1 ON m1.userid = (CASE WHEN cm.userid = cr.user1 THEN cr.user2 ELSE cr.user1 END) "+
				"INNER JOIN `groups` g ON m1.groupid = g.id AND g.type = 'Freegle' "+
				"WHERE cm.reviewrequired = 1 AND cm.reviewrejected = 0 AND cm.date >= ? "+
				"AND JSON_EXTRACT(g.settings, '$.widerchatreview') = 1 "+
				"AND cmh.id IS NULL "+
				"AND (cm.reportreason IS NULL OR cm.reportreason != 'User') "+
				"GROUP BY m1.groupid",
				chatCutoff).Scan(&widerRows)

			for _, r := range widerRows {
				if w, exists := workMap[r.Groupid]; exists {
					// Group already in workMap — add to chatreviewother.
					w.Chatreviewother += r.Count
				} else {
					// New group from wider review — add as new entry with chatreviewother only.
					workMap[r.Groupid] = &GroupWork{
						Groupid:         r.Groupid,
						Chatreviewother: r.Count,
					}
				}
			}
		}
	}()

	wg.Wait()

	// Convert to flat array sorted by groupid for deterministic output.
	result := make([]GroupWork, 0, len(workMap))
	for _, w := range workMap {
		result = append(result, *w)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Groupid < result[j].Groupid
	})

	return c.JSON(result)
}
