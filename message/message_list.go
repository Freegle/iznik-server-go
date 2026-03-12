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

// --- Message List types and handler ---

type MessageGroupInfo struct {
	Groupid    uint64    `json:"groupid"`
	Collection string    `json:"collection"`
	Arrival    time.Time `json:"arrival"`
}

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
	Source             *string             `json:"source"`
	Sourceheader       *string             `json:"sourceheader"`
	Fromaddr           *string             `json:"fromaddr"`
	Fromip             *string             `json:"fromip"`
	Fromcountry        *string             `json:"fromcountry"`
	Groups             []MessageGroupInfo  `json:"groups"`
	Attachments        []MessageAttachment `json:"attachments,omitempty"`
	Replycount         int                 `json:"replycount"`
}

type ListMessagesResponse struct {
	Messages []ListMessageItem `json:"messages"`
	Context  *PaginationContext `json:"context,omitempty"`
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

	// Determine which groups to query.  When groupid=0 for non-Approved
	// collections, fetch from all the user's moderated groups.
	var groupIDs []uint64

	if myid == 0 && collection != utils.COLLECTION_APPROVED {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	if groupid == 0 {
		if myid == 0 {
			return c.JSON(ListMessagesResponse{Messages: []ListMessageItem{}})
		}
		// Fetch from all groups this user moderates.  Return empty
		// list (not an error) if they don't moderate any groups,
		// matching PHP V1 behaviour.
		db.Raw("SELECT groupid FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner') AND collection = 'Approved'", myid).Pluck("groupid", &groupIDs)
		if len(groupIDs) == 0 {
			return c.JSON(ListMessagesResponse{Messages: []ListMessageItem{}})
		}
	} else {
		if collection != utils.COLLECTION_APPROVED {
			if !user.IsModOfGroup(myid, groupid) {
				return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this group")
			}
		}
		groupIDs = []uint64{groupid}
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
			"WHERE mg.groupid IN (?) "+
			"AND mg.collection = ? "+
			"AND mg.deleted = 0 "+
			"AND m.fromuser IS NOT NULL "+
			"AND m.subject LIKE ? "+
			"ORDER BY mg.arrival DESC LIMIT ?",
			groupIDs, collection, searchTerm, limit).Pluck("msgid", &msgIDs)
	} else if subaction == "searchmemb" && search != "" {
		searchTerm := "%" + search + "%"
		db.Raw("SELECT mg.msgid FROM messages_groups mg "+
			"INNER JOIN messages m ON m.id = mg.msgid "+
			"INNER JOIN users u ON u.id = m.fromuser "+
			"LEFT JOIN users_emails ue ON ue.userid = u.id "+
			"WHERE mg.groupid IN (?) "+
			"AND mg.collection = ? "+
			"AND mg.deleted = 0 "+
			"AND (u.fullname LIKE ? OR ue.email LIKE ?) "+
			"ORDER BY mg.arrival DESC LIMIT ?",
			groupIDs, collection, searchTerm, searchTerm, limit).Pluck("msgid", &msgIDs)
	} else {
		// Standard listing with optional pagination and fromuser filter.
		sql := "SELECT mg.msgid FROM messages_groups mg " +
			"INNER JOIN messages m ON m.id = mg.msgid " +
			"WHERE mg.groupid IN (?) " +
			"AND mg.collection = ? " +
			"AND mg.deleted = 0 " +
			"AND m.fromuser IS NOT NULL "

		args := []interface{}{groupIDs, collection}

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

	// Fetch message details in parallel.  Use a semaphore to cap the number
	// of concurrent message goroutines (each spawns 4 inner DB queries).
	messages := make([]ListMessageItem, len(msgIDs))
	var mu sync.Mutex
	var wgOuter sync.WaitGroup
	sem := make(chan struct{}, 10) // At most 10 messages × 4 queries = 40 DB connections.

	archiveDomain := os.Getenv("IMAGE_ARCHIVED_DOMAIN")
	imageDomain := os.Getenv("IMAGE_DOMAIN")

	wgOuter.Add(len(msgIDs))

	for idx, msgID := range msgIDs {
		go func(idx int, msgID uint64) {
			sem <- struct{}{}        // Acquire semaphore slot.
			defer func() { <-sem }() // Release on exit.
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
					"m.availablenow, m.availableinitially, "+
					"m.source, m.sourceheader, m.fromaddr, m.fromip, m.fromcountry "+
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

			// Convert 2-letter country code to full name for frontend display.
			if msg.Fromcountry != nil && len(*msg.Fromcountry) == 2 {
				if name, ok := utils.CountryName(*msg.Fromcountry); ok {
					msg.Fromcountry = &name
				}
			}

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

// ListMessagesMT handles GET /modtools/messages — returns message IDs only
// (the client fetches full details individually via GET /message/:id).
//
// @Summary List messages for modtools
// @Tags message
// @Produce json
// @Param groupid query integer false "Group ID"
// @Param collection query string false "Collection (Approved, Pending, Edits)"
// @Param limit query integer false "Max messages to return"
// @Param context query integer false "Pagination cursor"
// @Success 200 {object} map[string]interface{}
// @Router /api/modtools/messages [get]
func ListMessagesMT(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

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

	validCollections := map[string]bool{
		"Approved": true, "Pending": true, "Rejected": true, "Spam": true, "Edit": true,
	}
	if !validCollections[collection] {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid collection")
	}

	var groupIDs []uint64
	if groupid == 0 {
		db.Raw("SELECT groupid FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner') AND collection = 'Approved'", myid).Pluck("groupid", &groupIDs)
		if len(groupIDs) == 0 {
			return c.JSON(fiber.Map{"messages": []uint64{}})
		}
	} else {
		if collection != utils.COLLECTION_APPROVED {
			if !user.IsModOfGroup(myid, groupid) {
				return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this group")
			}
		}
		groupIDs = []uint64{groupid}
	}

	var ctx *PaginationContext
	contextStr := c.Query("context", "")
	if contextStr != "" {
		ctx = &PaginationContext{}
		if err := json.Unmarshal([]byte(contextStr), ctx); err != nil {
			ctx = nil
		}
	}

	subaction := c.Query("subaction", "")
	search := c.Query("search", "")
	fromuserStr := c.Query("fromuser", "0")
	fromuser, _ := strconv.ParseUint(fromuserStr, 10, 64)

	var msgIDs []uint64

	if collection == "Edit" {
		// Edit review uses messages_edits table, not messages_groups collection.
		db.Raw("SELECT me.msgid FROM messages_edits me "+
			"INNER JOIN messages_groups mg ON mg.msgid = me.msgid AND mg.deleted = 0 "+
			"WHERE mg.groupid IN (?) AND me.reviewrequired = 1 AND me.approvedat IS NULL AND me.revertedat IS NULL "+
			"AND me.timestamp > DATE_SUB(NOW(), INTERVAL 7 DAY) "+
			"ORDER BY me.timestamp DESC LIMIT ?",
			groupIDs, limit).Pluck("msgid", &msgIDs)
	} else if subaction == "searchall" && search != "" {
		searchTerm := "%" + search + "%"
		db.Raw("SELECT mg.msgid FROM messages_groups mg "+
			"INNER JOIN messages m ON m.id = mg.msgid "+
			"WHERE mg.groupid IN (?) AND mg.collection = ? AND mg.deleted = 0 "+
			"AND m.fromuser IS NOT NULL AND m.subject LIKE ? "+
			"ORDER BY mg.arrival DESC LIMIT ?",
			groupIDs, collection, searchTerm, limit).Pluck("msgid", &msgIDs)
	} else if subaction == "searchmemb" && search != "" {
		searchTerm := "%" + search + "%"
		db.Raw("SELECT mg.msgid FROM messages_groups mg "+
			"INNER JOIN messages m ON m.id = mg.msgid "+
			"INNER JOIN users u ON u.id = m.fromuser "+
			"LEFT JOIN users_emails ue ON ue.userid = u.id "+
			"WHERE mg.groupid IN (?) AND mg.collection = ? AND mg.deleted = 0 "+
			"AND (u.fullname LIKE ? OR ue.email LIKE ?) "+
			"ORDER BY mg.arrival DESC LIMIT ?",
			groupIDs, collection, searchTerm, searchTerm, limit).Pluck("msgid", &msgIDs)
	} else {
		sql := "SELECT mg.msgid FROM messages_groups mg " +
			"INNER JOIN messages m ON m.id = mg.msgid " +
			"WHERE mg.groupid IN (?) AND mg.collection = ? AND mg.deleted = 0 " +
			"AND m.fromuser IS NOT NULL "
		args := []interface{}{groupIDs, collection}

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
		return c.JSON(fiber.Map{"messages": []uint64{}})
	}

	// Build pagination context from last ID.
	var respCtx *PaginationContext
	if len(msgIDs) == limit {
		// Get arrival time of last message for pagination.
		var lastArrival time.Time
		db.Raw("SELECT arrival FROM messages_groups WHERE msgid = ? AND deleted = 0 LIMIT 1", msgIDs[len(msgIDs)-1]).Scan(&lastArrival)
		if !lastArrival.IsZero() {
			respCtx = &PaginationContext{
				Date: lastArrival.Unix(),
				ID:   msgIDs[len(msgIDs)-1],
			}
		}
	}

	return c.JSON(fiber.Map{
		"messages": msgIDs,
		"context":  respCtx,
	})
}

// GetMessagesWithHistory handles GET /message/:ids - fetches one or more messages.
// Message history is now returned via the user endpoint (GET /user/fetchmt?modtools=true).
func GetMessagesWithHistory(c *fiber.Ctx) error {
	ids := strings.Split(c.Params("ids"), ",")
	myid := user.WhoAmI(c)

	if len(ids) >= 20 {
		return fiber.NewError(fiber.StatusBadRequest, "Steady on")
	}

	messages := GetMessagesByIds(myid, ids)

	if len(ids) == 1 {
		if len(messages) == 1 {
			return c.JSON(messages[0])
		}
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}

	return c.JSON(messages)
}

