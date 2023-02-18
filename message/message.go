package message

import (
	"encoding/json"
	"errors"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/group"
	"github.com/freegle/iznik-server-go/item"
	"github.com/freegle/iznik-server-go/location"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
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
	Fromuser           uint64              `json:"fromuser"`
	Subject            string              `json:"subject"`
	Type               string              `json:"type"`
	Textbody           string              `json:"textbody"`
	Lat                float64             `json:"lat"`
	Lng                float64             `json:"lng"`
	Availablenow       uint                `json:"availablenow"`
	Availableinitially uint                `json:"availableinitially"`
	MessageGroups      []MessageGroup      `gorm:"-" json:"groups"`
	MessageAttachments []MessageAttachment `gorm:"-" json:"attachments"`
	MessageOutcomes    []MessageOutcome    `gorm:"-" json:"outcomes"`
	MessagePromises    []MessagePromise    `gorm:"-" json:"promises"`
	Promisecount       int                 `json:"promisecount"`
	Promised           bool                `json:"promised"`
	MessageReply       []MessageReply      `gorm:"ForeignKey:refmsgid" json:"replies"`
	Replycount         int                 `json:"replycount"`
	MessageURL         string              `json:"url"`
	Successful         bool                `json:"successful"`
	Refchatids         []uint64            `json:"refchatids" gorm:"-"`
	Locationid         uint64              `json:"-"`
	Location           *location.Location  `json:"location" gorm:"-"`
	Item               *item.Item          `json:"item" gorm:"-"`
	Repostat           *time.Time          `json:"repostat"`
	Canrepost          bool                `json:"canrepost"`
}

func GetMessages(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	db := database.DBConn
	archiveDomain := os.Getenv("IMAGE_ARCHIVED_DOMAIN")
	userSite := os.Getenv("USER_SITE")

	// This can be used to fetch one or more messages.  Fetch them in parallel.  Empirically this is faster than
	// fetching the information in parallel for multiple messages.
	ids := strings.Split(c.Params("ids"), ",")
	var mu sync.Mutex
	var messages []Message
	var er = regexp.MustCompile(utils.EMAIL_REGEXP)
	var ep = regexp.MustCompile(utils.PHONE_REGEXP)

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
					err := db.Select([]string{"id", "arrival", "date", "fromuser", "subject", "type", "textbody", "lat", "lng", "availablenow", "availableinitially", "locationid"}).Where("messages.id = ? AND messages.deleted IS NULL", id).First(&message).Error
					found = !errors.Is(err, gorm.ErrRecordNotFound)
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
					db.Raw("SELECT chat_messages.id, refmsgid, date, userid,"+
						"CASE WHEN users.fullname IS NOT NULL THEN users.fullname ELSE CONCAT(users.firstname, ' ', users.lastname) END AS displayname "+
						"FROM chat_messages "+
						"INNER JOIN users ON users.id = chat_messages.userid "+
						"WHERE refmsgid = ? AND chat_messages.type = ?;", id, utils.MESSAGE_INTERESTED).Scan(&messageReply)
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

				var refchatids []uint64
				wg.Add(1)
				go func() {
					defer wg.Done()
					db.Raw("SELECT DISTINCT(chatid) FROM chat_messages WHERE refmsgid = ?;", id).Pluck("id", &refchatids)
				}()

				wg.Wait()

				message.MessageGroups = messageGroups
				message.MessageAttachments = messageAttachments
				message.MessageReply = messageReply
				message.MessageOutcomes = messageOutcomes
				message.MessagePromises = messagePromises
				message.Repostat = nil
				message.Canrepost = false

				if found {
					message.Replycount = len(message.MessageReply)
					message.MessageURL = "https://" + os.Getenv("USER_SITE") + "/message/" + strconv.FormatUint(message.ID, 10)

					// Protect anonymity of poster a bit.
					message.Lat, message.Lng = utils.Blur(message.Lat, message.Lng, utils.BLUR_USER)

					// Remove confidential info.
					message.Textbody = er.ReplaceAllString(message.Textbody, "***@***.com")
					message.Textbody = ep.ReplaceAllString(message.Textbody, "***")

					// Get the paths.
					for i, a := range message.MessageAttachments {
						if a.Archived > 0 {
							message.MessageAttachments[i].Path = "https://" + archiveDomain + "/img_" + strconv.FormatUint(a.ID, 10) + ".jpg"
							message.MessageAttachments[i].Paththumb = "https://" + archiveDomain + "/timg_" + strconv.FormatUint(a.ID, 10) + ".jpg"
						} else {
							message.MessageAttachments[i].Path = "https://" + userSite + "/img_" + strconv.FormatUint(a.ID, 10) + ".jpg"
							message.MessageAttachments[i].Paththumb = "https://" + userSite + "/timg_" + strconv.FormatUint(a.ID, 10) + ".jpg"
						}
					}

					message.Promisecount = len(message.MessagePromises)
					message.Promised = message.Promisecount > 0

					for _, o := range message.MessageOutcomes {
						if o.Outcome == utils.OUTCOME_TAKEN || o.Outcome == utils.OUTCOME_RECEIVED {
							message.Successful = true
						}
					}

					if message.Fromuser != myid {
						// Shouldn't see promise details.
						message.MessagePromises = nil
					} else {
						message.Refchatids = refchatids

						if message.Locationid > 0 {
							// Need extra info for own messages.
							var wgMine sync.WaitGroup

							var loc *location.Location
							var i *item.Item

							wgMine.Add(1)
							go func() {
								defer wgMine.Done()
								loc = location.FetchSingle(message.Locationid)
							}()

							wgMine.Add(1)
							go func() {
								defer wgMine.Done()
								i = item.FetchForMessage(message.ID)
							}()

							wgMine.Add(1)
							go func() {
								defer wgMine.Done()
								var repostStr []string
								db.Raw("SELECT CASE WHEN JSON_EXTRACT(settings, '$.reposts') IS NULL THEN '{''offer'' => 3, ''wanted'' => 7, ''max'' => 5, ''chaseups'' => 5}' ELSE JSON_EXTRACT(settings, '$.reposts') END AS reposts FROM `groups` INNER JOIN messages_groups ON messages_groups.groupid = groups.id WHERE msgid = ?", message.ID).Pluck("reposts", &repostStr)

								var reposts []group.RepostSettings

								// Unmarshall repostStr into reposts
								for _, r := range repostStr {
									var rs group.RepostSettings
									json.Unmarshal([]byte(r), &rs)
									reposts = append(reposts, rs)
								}

								for _, r := range reposts {
									// If message is an offer
									var interval int

									if message.Type == utils.OFFER {
										interval = r.Offer
									} else {
										interval = r.Wanted
									}

									if interval < 365 {
										// Some groups set very high values as a way of turning this off.
										repostAt := message.Arrival.AddDate(0, 0, interval)
										message.Repostat = &repostAt

										if repostAt.Before(time.Now()) {
											message.Canrepost = true
										}
									}
								}
							}()

							wgMine.Wait()

							message.Location = loc
							message.Item = i
						}
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
				"EXISTS(SELECT id FROM messages_outcomes WHERE messages_outcomes.msgid = messages.id) AS hasoutcome, " +
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

func Search(c *fiber.Ctx) error {
	// TODO Record search, popularity, etc.
	// TODO Restrict search by group, location, etc.
	db := database.DBConn
	term := c.Params("term")
	term = strings.TrimSpace(term)
	var res []SearchResult

	if len(term) > 0 {
		if term == "" {
			return fiber.NewError(fiber.StatusBadRequest, "No search term")
		}

		res = GetWordsExact(db, term, SEARCH_LIMIT)

		if len(res) == 0 {
			res = GetWordsTypo(db, term, SEARCH_LIMIT)
		}

		if len(res) == 0 {
			res = GetWordsStarts(db, term, SEARCH_LIMIT)
		}

		if len(res) == 0 {
			res = GetWordsSounds(db, term, SEARCH_LIMIT)
		}

		// Blur
		for ix, r := range res {
			res[ix].Lat, res[ix].Lng = utils.Blur(r.Lat, r.Lng, utils.BLUR_USER)
		}
	}

	return c.JSON(res)
}
