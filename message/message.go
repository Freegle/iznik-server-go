package message

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Message struct {
	ID                 uint64              `json:"id" gorm:"primary_key"`
	Arrival            time.Time           `json:"arrival"`
	Date               time.Time           `json:"date"`
	Fromuser           uint64              `json:"-"`
	FromuserObj        user.User           `json:"fromuser" gorm:"-"`
	Subject            string              `json:"subject"`
	Type               string              `json:"type"`
	Textbody           string              `json:"textbody"`
	Lat                float64             `json:"lat"`
	Lng                float64             `json:"lng"`
	Availablenow       uint                `json:"availablenow"`
	Availableinitially uint                `json:"availableinitially"`
	MessageGroups      []MessageGroup      `gorm:"ForeignKey:msgid" json:"groups"`
	MessageAttachments []MessageAttachment `gorm:"ForeignKey:msgid" json:"attachments"`
	MessageOutcomes    []MessageOutcome    `gorm:"ForeignKey:msgid" json:"outcomes"`
	MessagePromises    []MessagePromise    `gorm:"ForeignKey:msgid" json:"promises"`
	Promisecount       int                 `json:"promisecount"` // TODO Is this used, as well as Promised?
	Promised           bool                `json:"promised"`
	MessageReply       []MessageReply      `gorm:"ForeignKey:refmsgid" json:"replies"`
	Replycount         int                 `json:"replycount"`
	MessageURL         string              `json:"url"`
	Successful         bool                `json:"successful"`
}

func GetMessages(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	db := database.DBConn

	// This can be used to fetch one or more messages.  Fetch them in parallel.  Empically this is faster than
	// fetching the information in parallel for multiple messages.
	ids := strings.Split(c.Params("ids"), ",")
	var mu sync.Mutex
	var messages []Message

	if len(ids) < 20 {
		var wgOuter sync.WaitGroup

		wgOuter.Add(len(ids))

		for _, id := range ids {
			go func(id string) {
				defer wgOuter.Done()

				var message Message
				found := false

				// We have lots to load here.  db.preload is tempting, but loads in series - so if we use go routines we can
				// load in parallel and reduce latency.
				var wg sync.WaitGroup

				wg.Add(1)
				go func() {
					defer wg.Done()
					found = !db.Select([]string{"id", "arrival", "date", "fromuser", "subject", "type", "textbody", "lat", "lng", "availablenow", "availableinitially"}).Where("messages.id = ? AND messages.deleted IS NULL", id).Find(&message).RecordNotFound()
				}()

				var messageGroups []MessageGroup
				wg.Add(1)
				go func() {
					defer wg.Done()
					if myid != 0 {
						// Can see own messages even if they are still pending.
						db.Where("msgid = ? AND deleted = 0", id).Find(&messageGroups)
					} else {
						// Only showing approved messages.
						db.Where("msgid = ? AND collection = ? AND deleted = 0", id, utils.COLLECTION_APPROVED).Find(&messageGroups)
					}
				}()

				var messageAttachments []MessageAttachment
				wg.Add(1)
				go func() {
					defer wg.Done()
					db.Select([]string{"id", "msgid", "archived"}).Where("msgid = ?", id).Find(&messageAttachments).Order("id ASC")
				}()

				var messageReply []MessageReply
				wg.Add(1)
				go func() {
					defer wg.Done()
					db.Select([]string{"id", "refmsgid", "date"}).Where("refmsgid = ? AND type = ?", id, utils.MESSAGE_INTERESTED).Find(&messageReply)
				}()

				var messageOutcomes []MessageOutcome
				wg.Add(1)
				go func() {
					defer wg.Done()
					db.Where("msgid = ?", id).Find(&messageOutcomes)
				}()

				var messagePromises []MessagePromise
				wg.Add(1)
				go func() {
					defer wg.Done()
					db.Where("msgid = ?", id).Find(&messagePromises)
				}()

				wg.Wait()

				message.MessageGroups = messageGroups
				message.MessageAttachments = messageAttachments
				message.MessageReply = messageReply
				message.MessageOutcomes = messageOutcomes
				message.MessagePromises = messagePromises

				if found {
					message.Replycount = len(message.MessageReply)
					message.MessageURL = "https://" + os.Getenv("USER_SITE") + "/message/" + strconv.FormatUint(message.ID, 10)

					// Protect anonymity of poster a bit.
					message.Lat, message.Lng = utils.Blur(message.Lat, message.Lng, utils.BLUR_USER)

					// Remove confidential info.
					var er = regexp.MustCompile(utils.EMAIL_REGEXP)
					message.Textbody = er.ReplaceAllString(message.Textbody, "***@***.com")
					var ep = regexp.MustCompile(utils.PHONE_REGEXP)
					message.Textbody = ep.ReplaceAllString(message.Textbody, "***")

					// Get the paths.
					for i, a := range message.MessageAttachments {
						if a.Archived > 0 {
							message.MessageAttachments[i].Path = "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/img_" + strconv.FormatUint(a.ID, 10) + ".jpg"
							message.MessageAttachments[i].Paththumb = "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/timg_" + strconv.FormatUint(a.ID, 10) + ".jpg"
						} else {
							message.MessageAttachments[i].Path = "https://" + os.Getenv("USER_SITE") + "/img_" + strconv.FormatUint(a.ID, 10) + ".jpg"
							message.MessageAttachments[i].Paththumb = "https://" + os.Getenv("USER_SITE") + "/timg_" + strconv.FormatUint(a.ID, 10) + ".jpg"
						}
					}

					message.FromuserObj = user.GetUserById(message.Fromuser, myid)
					message.Promisecount = len(message.MessagePromises)
					message.Promised = message.Promisecount > 0

					for _, o := range message.MessageOutcomes {
						if o.Outcome == utils.OUTCOME_TAKEN || o.Outcome == utils.OUTCOME_RECEIVED {
							message.Successful = true
						}
					}

					if message.FromuserObj.ID != myid {
						// Shouldn't see promise details.
						message.MessagePromises = nil
					}

					mu.Lock()
					messages = append(messages, message)
					mu.Unlock()
				}
			}(id)
		}

		wgOuter.Wait()

		if len(ids) == 1 {
			if len(messages) == 1 {
				return c.JSON(messages[0])
			} else {
				return fiber.NewError(fiber.StatusNotFound, "Message not found")
			}
		} else {
			return c.JSON(messages)
		}
	} else {
		return fiber.NewError(fiber.StatusBadRequest, "Steady on")
	}
}

func GetMessagesForUser(c *fiber.Ctx) error {
	db := database.DBConn

	if c.Params("id") != "" {
		id, err1 := strconv.ParseUint(c.Params("id"), 10, 64)
		active, err2 := strconv.ParseBool(c.Query("active", "false"))

		if err1 == nil && err2 == nil {
			var msgs []MessageSummary

			sql := "SELECT lat, lng, messages.id, messages_groups.groupid, type, messages_groups.arrival, " +
				"EXISTS(SELECT id FROM messages_outcomes WHERE messages_outcomes.msgid = messages.id AND outcome IN (?, ?)) AS successful, " +
				"EXISTS(SELECT id FROM messages_promises WHERE messages_promises.msgid = messages.id) AS promised " +
				"FROM messages " +
				"INNER JOIN messages_groups ON messages_groups.msgid = messages.id "

			if active {
				// We are only interested in active messages.
				sql += "INNER JOIN messages_spatial ON messages_spatial.msgid = messages.id "
			}

			sql += "WHERE fromuser = ? AND messages.deleted IS NULL AND messages_groups.deleted = 0 " +
				"ORDER BY messages_groups.arrival DESC"

			db.Raw(sql, utils.TAKEN, utils.RECEIVED, id).Scan(&msgs)

			for ix, r := range msgs {
				// Protect anonymity of poster a bit.
				msgs[ix].Lat, msgs[ix].Lng = utils.Blur(r.Lat, r.Lng, utils.BLUR_USER)
			}

			return c.JSON(msgs)
		}
	}

	return fiber.NewError(fiber.StatusNotFound, "User not found")
}
