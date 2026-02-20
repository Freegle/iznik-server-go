package message

import (
	"encoding/json"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/misc"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// --- Message History types (for fetchMT with messagehistory=true) ---

type MessageGroupInfo struct {
	Groupid    uint64    `json:"groupid"`
	Collection string    `json:"collection"`
	Arrival    time.Time `json:"arrival"`
}

type UserEmail struct {
	Email     string    `json:"email"`
	Validated *string   `json:"validated"`
	Added     time.Time `json:"added"`
}

type RecentPost struct {
	ID         uint64    `json:"id"`
	Subject    string    `json:"subject"`
	Type       string    `json:"type"`
	Arrival    time.Time `json:"arrival"`
	Groupid    uint64    `json:"groupid"`
	Collection string    `json:"collection"`
}

type HeldByInfo struct {
	ID        uint64 `json:"id"`
	Fullname  string `json:"fullname"`
	Firstname string `json:"firstname"`
	Lastname  string `json:"lastname"`
}

type MessageHistory struct {
	Groups       []MessageGroupInfo `json:"groups,omitempty"`
	PosterEmails []UserEmail        `json:"posteremails,omitempty"`
	RecentPosts  []RecentPost       `json:"recentposts,omitempty"`
	HeldBy       *HeldByInfo        `json:"heldby,omitempty"`
}

// MessageWithHistory wraps a Message with optional history data for moderator views.
type MessageWithHistory struct {
	Message
	MessageHistoryData *MessageHistory `json:"messagehistory,omitempty"`
}

// GetMessageHistory fetches the history data for a message (moderator only).
func GetMessageHistory(msgid uint64, fromuser uint64) *MessageHistory {
	db := database.DBConn
	history := &MessageHistory{}

	var wg sync.WaitGroup

	// Fetch group collection info.
	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Raw("SELECT groupid, collection, arrival FROM messages_groups WHERE msgid = ? AND deleted = 0", msgid).Scan(&history.Groups)
	}()

	// Fetch poster emails.
	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Raw("SELECT email, validated, added FROM users_emails WHERE userid = ? ORDER BY preferred DESC, email", fromuser).Scan(&history.PosterEmails)
	}()

	// Fetch recent posts by this user.
	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Raw("SELECT m.id, m.subject, m.type, m.arrival, mg.groupid, mg.collection "+
			"FROM messages m "+
			"INNER JOIN messages_groups mg ON m.id = mg.msgid "+
			"WHERE m.fromuser = ? AND mg.deleted = 0 "+
			"ORDER BY m.arrival DESC LIMIT 20", fromuser).Scan(&history.RecentPosts)
	}()

	// Fetch held-by info.
	wg.Add(1)
	go func() {
		defer wg.Done()
		var heldByID *uint64
		db.Raw("SELECT heldby FROM messages WHERE id = ?", msgid).Scan(&heldByID)
		if heldByID != nil && *heldByID > 0 {
			var info HeldByInfo
			db.Raw("SELECT id, fullname, firstname, lastname FROM users WHERE id = ?", *heldByID).Scan(&info)
			if info.ID > 0 {
				history.HeldBy = &info
			}
		}
	}()

	wg.Wait()
	return history
}

// --- Message List types and handler ---

type PaginationContext struct {
	Date int64  `json:"Date"`
	ID   uint64 `json:"id"`
}

type ListMessageItem struct {
	ID                 uint64              `json:"id"`
	Subject            string              `json:"subject"`
	Type               string              `json:"type"`
	Fromuser           uint64              `json:"fromuser"`
	Arrival            time.Time           `json:"arrival"`
	Lat                float64             `json:"lat"`
	Lng                float64             `json:"lng"`
	Availablenow       uint                `json:"availablenow"`
	Availableinitially uint                `json:"availableinitially"`
	Groups             []MessageGroupInfo  `json:"groups"`
	Attachments        []MessageAttachment `json:"attachments,omitempty"`
	Replycount         int                 `json:"replycount"`
}

type ListMessagesResponse struct {
	Messages []ListMessageItem `json:"messages"`
	Context  *PaginationContext `json:"context,omitempty"`
}

// isModOfGroup checks if the user is a moderator/owner of the given group, or admin/support.
func isModOfGroup(myid uint64, groupid uint64) bool {
	db := database.DBConn

	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)
	if systemrole == utils.SYSTEMROLE_SUPPORT || systemrole == utils.SYSTEMROLE_ADMIN {
		return true
	}

	if groupid == 0 {
		return false
	}

	var role string
	db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ?",
		myid, groupid).Scan(&role)
	return role == utils.ROLE_MODERATOR || role == utils.ROLE_OWNER
}

// ListMessages handles GET /messages - list messages with moderation queue support.
func ListMessages(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	db := database.DBConn

	// Parse parameters.
	collection := c.Query("collection", utils.COLLECTION_APPROVED)
	groupidStr := c.Query("groupid", "0")
	groupid, _ := strconv.ParseUint(groupidStr, 10, 64)
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	subaction := c.Query("subaction", "")
	search := c.Query("search", "")
	fromuserStr := c.Query("fromuser", "0")
	fromuser, _ := strconv.ParseUint(fromuserStr, 10, 64)

	// Validate collection.
	validCollections := map[string]bool{
		"Approved": true,
		"Pending":  true,
		"Rejected": true,
		"Spam":     true,
	}
	if !validCollections[collection] {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid collection")
	}

	// Non-Approved collections require moderator access.
	if collection != utils.COLLECTION_APPROVED {
		if myid == 0 {
			return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
		}
		if groupid == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "groupid required for non-Approved collection")
		}
		if !isModOfGroup(myid, groupid) {
			return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this group")
		}
	}

	// If groupid is required but not set for approved, it's still needed for listing.
	if groupid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "groupid is required")
	}

	// Parse pagination context.
	var ctx *PaginationContext
	contextStr := c.Query("context", "")
	if contextStr != "" {
		ctx = &PaginationContext{}
		if err := json.Unmarshal([]byte(contextStr), ctx); err != nil {
			ctx = nil
		}
	}

	var msgIDs []uint64

	// Handle search modes.
	if subaction == "searchall" && search != "" {
		searchTerm := "%" + search + "%"
		db.Raw("SELECT mg.msgid FROM messages_groups mg "+
			"INNER JOIN messages m ON m.id = mg.msgid "+
			"WHERE mg.groupid = ? "+
			"AND mg.collection = ? "+
			"AND mg.deleted = 0 "+
			"AND m.fromuser IS NOT NULL "+
			"AND m.subject LIKE ? "+
			"ORDER BY mg.arrival DESC LIMIT ?",
			groupid, collection, searchTerm, limit).Pluck("msgid", &msgIDs)
	} else if subaction == "searchmemb" && search != "" {
		searchTerm := "%" + search + "%"
		db.Raw("SELECT mg.msgid FROM messages_groups mg "+
			"INNER JOIN messages m ON m.id = mg.msgid "+
			"INNER JOIN users u ON u.id = m.fromuser "+
			"LEFT JOIN users_emails ue ON ue.userid = u.id "+
			"WHERE mg.groupid = ? "+
			"AND mg.collection = ? "+
			"AND mg.deleted = 0 "+
			"AND (u.fullname LIKE ? OR ue.email LIKE ?) "+
			"ORDER BY mg.arrival DESC LIMIT ?",
			groupid, collection, searchTerm, searchTerm, limit).Pluck("msgid", &msgIDs)
	} else {
		// Standard listing with optional pagination and fromuser filter.
		sql := "SELECT mg.msgid FROM messages_groups mg " +
			"INNER JOIN messages m ON m.id = mg.msgid " +
			"WHERE mg.groupid = ? " +
			"AND mg.collection = ? " +
			"AND mg.deleted = 0 " +
			"AND m.fromuser IS NOT NULL "

		args := []interface{}{groupid, collection}

		if fromuser > 0 {
			sql += "AND m.fromuser = ? "
			args = append(args, fromuser)
		}

		if ctx != nil && ctx.Date > 0 {
			ctxTime := time.Unix(ctx.Date, 0).UTC().Format("2006-01-02 15:04:05")
			sql += "AND (mg.arrival < ? OR (mg.arrival = ? AND mg.msgid < ?)) "
			args = append(args, ctxTime, ctxTime, ctx.ID)
		}

		sql += "ORDER BY mg.arrival DESC, mg.msgid DESC LIMIT ?"
		args = append(args, limit)

		db.Raw(sql, args...).Pluck("msgid", &msgIDs)
	}

	if len(msgIDs) == 0 {
		return c.JSON(ListMessagesResponse{
			Messages: []ListMessageItem{},
		})
	}

	// Fetch message details in parallel.
	messages := make([]ListMessageItem, len(msgIDs))
	var mu sync.Mutex
	var wgOuter sync.WaitGroup

	archiveDomain := os.Getenv("IMAGE_ARCHIVED_DOMAIN")
	imageDomain := os.Getenv("IMAGE_DOMAIN")

	wgOuter.Add(len(msgIDs))

	for idx, msgID := range msgIDs {
		go func(idx int, msgID uint64) {
			defer wgOuter.Done()

			var msg ListMessageItem
			var groups []MessageGroupInfo
			var attachments []MessageAttachment
			var replycount int64

			var wg sync.WaitGroup

			wg.Add(4)

			go func() {
				defer wg.Done()
				db.Raw("SELECT m.id, m.subject, m.type, m.fromuser, m.arrival, m.lat, m.lng, "+
					"m.availablenow, m.availableinitially "+
					"FROM messages m WHERE m.id = ?", msgID).Scan(&msg)
			}()

			go func() {
				defer wg.Done()
				db.Raw("SELECT groupid, collection, arrival FROM messages_groups WHERE msgid = ? AND deleted = 0", msgID).Scan(&groups)
			}()

			go func() {
				defer wg.Done()
				// Fetch first image only for thumbnail.
				db.Raw("SELECT id, msgid, archived, externaluid, externalmods FROM messages_attachments WHERE msgid = ? ORDER BY `primary` DESC, id ASC LIMIT 1", msgID).Scan(&attachments)
			}()

			go func() {
				defer wg.Done()
				db.Raw("SELECT COUNT(*) FROM chat_messages WHERE refmsgid = ? AND type = ? AND reviewrequired = 0 AND reviewrejected = 0",
					msgID, utils.MESSAGE_INTERESTED).Scan(&replycount)
			}()

			wg.Wait()

			msg.Groups = groups
			msg.Replycount = int(replycount)

			// Process attachment paths.
			for i, a := range attachments {
				if a.Externaluid != "" {
					attachments[i].Ouruid = a.Externaluid
					attachments[i].Externalmods = a.Externalmods
					attachments[i].Path = misc.GetImageDeliveryUrl(a.Externaluid, string(a.Externalmods))
					attachments[i].Paththumb = misc.GetImageDeliveryUrl(a.Externaluid, string(a.Externalmods))
				} else if a.Archived > 0 {
					attachments[i].Path = "https://" + archiveDomain + "/img_" + strconv.FormatUint(a.ID, 10) + ".jpg"
					attachments[i].Paththumb = "https://" + archiveDomain + "/timg_" + strconv.FormatUint(a.ID, 10) + ".jpg"
				} else {
					attachments[i].Path = "https://" + imageDomain + "/img_" + strconv.FormatUint(a.ID, 10) + ".jpg"
					attachments[i].Paththumb = "https://" + imageDomain + "/timg_" + strconv.FormatUint(a.ID, 10) + ".jpg"
				}
			}
			msg.Attachments = attachments

			// Blur location for privacy.
			msg.Lat, msg.Lng = utils.Blur(msg.Lat, msg.Lng, utils.BLUR_USER)

			mu.Lock()
			messages[idx] = msg
			mu.Unlock()
		}(idx, msgID)
	}

	wgOuter.Wait()

	// Filter out zero-ID entries (shouldn't happen, but be defensive).
	var filtered []ListMessageItem
	for _, m := range messages {
		if m.ID > 0 {
			filtered = append(filtered, m)
		}
	}

	// Build pagination context from the last message.
	var respCtx *PaginationContext
	if len(filtered) > 0 && len(filtered) == limit {
		last := filtered[len(filtered)-1]
		respCtx = &PaginationContext{
			Date: last.Arrival.Unix(),
			ID:   last.ID,
		}
	}

	return c.JSON(ListMessagesResponse{
		Messages: filtered,
		Context:  respCtx,
	})
}

// GetMessagesWithHistory extends GetMessages to include messagehistory data for moderators.
// This wraps the single-message fetch case of GetMessages.
func GetMessagesWithHistory(c *fiber.Ctx) error {
	ids := strings.Split(c.Params("ids"), ",")
	myid := user.WhoAmI(c)
	messagehistory := c.Query("messagehistory", "false") == "true"

	if len(ids) >= 20 {
		return fiber.NewError(fiber.StatusBadRequest, "Steady on")
	}

	messages := GetMessagesByIds(myid, ids)

	// If messagehistory=true and single message, check mod access and include history.
	if messagehistory && len(ids) == 1 && len(messages) == 1 {
		msg := messages[0]

		// Check if user is a mod for any group this message is on.
		db := database.DBConn
		isMod := false

		if myid > 0 {
			// Check system role.
			var systemrole string
			db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)
			if systemrole == utils.SYSTEMROLE_ADMIN || systemrole == utils.SYSTEMROLE_SUPPORT {
				isMod = true
			}

			if !isMod {
				// Check if moderator of any group the message is on.
				var count int64
				db.Raw("SELECT COUNT(*) FROM messages_groups mg "+
					"JOIN memberships m ON m.groupid = mg.groupid "+
					"WHERE mg.msgid = ? AND m.userid = ? AND m.role IN ('Moderator', 'Owner')",
					msg.ID, myid).Scan(&count)
				isMod = count > 0
			}
		}

		if isMod {
			history := GetMessageHistory(msg.ID, msg.Fromuser)
			resp := MessageWithHistory{
				Message:            msg,
				MessageHistoryData: history,
			}
			return c.JSON(resp)
		}

		// Not a mod - return without history.
		return c.JSON(msg)
	}

	if len(ids) == 1 {
		if len(messages) == 1 {
			return c.JSON(messages[0])
		}
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}

	return c.JSON(messages)
}

