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
	"net/url"
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
	PromisedToYou      bool                `json:"promisedtoyou"`
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
	ids := strings.Split(c.Params("ids"), ",")
	myid := user.WhoAmI(c)

	if len(ids) < 20 {
		messages := GetMessagesByIds(myid, ids)

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

func GetMessagesByIds(myid uint64, ids []string) []Message {
	db := database.DBConn
	archiveDomain := os.Getenv("IMAGE_ARCHIVED_DOMAIN")
	imageDomain := os.Getenv("IMAGE_DOMAIN")

	// This can be used to fetch one or more messages.  Fetch them in parallel.  Empirically this is faster than
	// fetching the information in parallel for multiple messages.
	var mu sync.Mutex
	messages := []Message{}
	var er = regexp.MustCompile(utils.EMAIL_REGEXP)
	var ep = regexp.MustCompile(utils.PHONE_REGEXP)

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
					db.Raw("SELECT groupid, msgid, arrival, collection, autoreposts, approvedby FROM messages_groups WHERE msgid = ? AND deleted = 0", id).Scan(&messageGroups)
				} else {
					// Only showing approved messages.
					db.Raw("SELECT groupid, msgid, arrival, collection, autoreposts,approvedby FROM messages_groups WHERE msgid = ? AND collection = ? AND deleted = 0", id, utils.COLLECTION_APPROVED).Scan(&messageGroups)
				}
			}()

			var messageAttachments []MessageAttachment

			wg.Add(1)
			go func() {
				defer wg.Done()
				db.Raw("SELECT id, msgid, archived FROM messages_attachments WHERE msgid = ? ORDER BY id ASC", id).Scan(&messageAttachments)
			}()

			var messageReply []MessageReply
			wg.Add(1)
			go func() {
				defer wg.Done()

				// There is some strange case where people can end up replying to themselves.  Don't show such
				// replies.
				//
				// If someone has replied multiple times, we only want to return one of them, so group by userid.
				db.Raw("SELECT DISTINCT chat_messages.id, refmsgid, date, userid,"+
					"CASE WHEN users.fullname IS NOT NULL THEN users.fullname ELSE CONCAT(users.firstname, ' ', users.lastname) END AS displayname "+
					"FROM chat_messages "+
					"INNER JOIN users ON users.id = chat_messages.userid "+
					"WHERE refmsgid = ? AND chat_messages.type = ? AND chat_messages.userid != ? "+
					"GROUP BY userid;", id, utils.MESSAGE_INTERESTED, myid).Scan(&messageReply)
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

			if found {
				message.Replycount = len(message.MessageReply)
				message.MessageURL = "https://" + os.Getenv("USER_SITE") + "/message/" + strconv.FormatUint(message.ID, 10)

				// Protect anonymity of poster a bit.
				message.Lat, message.Lng = utils.Blur(message.Lat, message.Lng, utils.BLUR_USER)

				if myid == 0 {
					// Remove confidential info.
					message.Textbody = er.ReplaceAllString(message.Textbody, "***@***.com")
					message.Textbody = ep.ReplaceAllString(message.Textbody, "***")
				}

				// Get the paths.
				for i, a := range message.MessageAttachments {
					if a.Archived > 0 {
						message.MessageAttachments[i].Path = "https://" + archiveDomain + "/img_" + strconv.FormatUint(a.ID, 10) + ".jpg"
						message.MessageAttachments[i].Paththumb = "https://" + archiveDomain + "/timg_" + strconv.FormatUint(a.ID, 10) + ".jpg"
					} else {
						message.MessageAttachments[i].Path = "https://" + imageDomain + "/img_" + strconv.FormatUint(a.ID, 10) + ".jpg"
						message.MessageAttachments[i].Paththumb = "https://" + imageDomain + "/timg_" + strconv.FormatUint(a.ID, 10) + ".jpg"
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
					// Shouldn't see promise details, but should see if it's promised to them.
					for i := range message.MessagePromises {
						if message.MessagePromises[i].Userid == myid {
							message.PromisedToYou = true
						}
					}

					message.MessagePromises = nil
				} else {
					message.Refchatids = refchatids

					if message.Locationid > 0 {
						// Need extra info for own messages.
						var wgMine sync.WaitGroup

						var loc *location.Location
						var i *item.Item
						var repostAt *time.Time
						var canRepost bool

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
									if len(message.MessageGroups) > 0 {
										ra := message.MessageGroups[0].Arrival.AddDate(0, 0, interval)
										repostAt = &ra

										if repostAt.Before(time.Now()) {
											canRepost = true
										}
									}
								}
							}
						}()

						wgMine.Wait()

						message.Location = loc
						message.Item = i
						message.Repostat = repostAt
						message.Canrepost = canRepost
					}
				}

				mu.Lock()
				messages = append(messages, message)
				mu.Unlock()
			}
		}(id)
	}

	wgOuter.Wait()

	return messages
}

func GetMessagesForUser(c *fiber.Ctx) error {
	db := database.DBConn

	if c.Params("id") != "" {
		id, err1 := strconv.ParseUint(c.Params("id"), 10, 64)
		active, err2 := strconv.ParseBool(c.Query("active", "false"))

		if err1 == nil && err2 == nil {
			msgs := []MessageSummary{}

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

			sql += "WHERE fromuser = ? AND messages.deleted IS NULL AND messages_groups.deleted = 0 AND " +
				"messages.type IN (?, ?)"

			if active {
				sql += " HAVING hasoutcome = FALSE"
			}

			sql += " ORDER BY messages_groups.arrival DESC"

			db.Debug().Raw(sql, utils.TAKEN, utils.RECEIVED, id, utils.OFFER, utils.WANTED).Scan(&msgs)

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
	db := database.DBConn
	term, _ := url.QueryUnescape(c.Params("term"))
	term = strings.TrimSpace(term)
	myid := user.WhoAmI(c)

	msgtype := c.Query("messagetype", "All")

	groupidss := strings.Split(c.Query("groupids", ""), ",")
	var groupids []uint64

	if len(groupidss) > 0 {
		for _, g := range groupidss {
			gid, err := strconv.ParseUint(g, 10, 64)
			if err == nil {
				groupids = append(groupids, gid)
			}
		}
	}

	// We want to record the search history, but we can do that in parallel to the actual search.
	// Word popularity is handled when the message is inserted into the index.
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		var ID int64

		if myid > 0 {
			db.Raw("INSERT INTO search_history (userid, term, locationid, `groups`) VALUES (?, ?, ?, ?);",
				myid,
				term,
				nil,
				c.Query("groupids", ""),
			).Scan(&ID)

			db.Raw("INSERT INTO users_searches (userid, term, locationid) VALUES (?, ?, ?);",
				myid,
				term,
				nil,
			).Scan(&ID)
		} else {
			db.Raw("INSERT INTO search_history (userid, term, locationid, `groups`) VALUES (NULL, ?, ?, ?);",
				term,
				nil,
				c.Query("groupids", ""),
			).Scan(&ID)
		}
	}()

	nelat, _ := strconv.ParseFloat(c.Query("nelat", "0"), 32)
	nelng, _ := strconv.ParseFloat(c.Query("nelng", "0"), 32)
	swlat, _ := strconv.ParseFloat(c.Query("swlat", "0"), 32)
	swlng, _ := strconv.ParseFloat(c.Query("swlng", "0"), 32)

	// We've seen problems with crashes inside Gorm.  Best I can tell, it looks like a Gorm bug exposed when an
	// array is resized.  So as a workaround we create slices with capacity, then filter out the empty ones at
	// the end.
	var res []SearchResult
	var res2 []SearchResult

	if len(term) > 0 {
		if term == "" {
			return fiber.NewError(fiber.StatusBadRequest, "No search term")
		}

		words := GetWords(term)

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			res = GetWordsExact(db, words, SEARCH_LIMIT, groupids, msgtype, float32(nelat), float32(nelng), float32(swlat), float32(swlng))
		}()

		go func() {
			defer wg.Done()
			// Add in prefix matches, which helps with plurals.
			res2 = GetWordsStarts(db, words, SEARCH_LIMIT, groupids, msgtype, float32(nelat), float32(nelng), float32(swlat), float32(swlng))
		}()

		wg.Wait()

		res = append(res, res2...)

		if len(res) == 0 {
			res = GetWordsTypo(db, words, SEARCH_LIMIT, groupids, msgtype, float32(nelat), float32(nelng), float32(swlat), float32(swlng))
		}

		if len(res) == 0 {
			res = GetWordsSounds(db, words, SEARCH_LIMIT, groupids, msgtype, float32(nelat), float32(nelng), float32(swlat), float32(swlng))
		}

		// Blur
		for ix, r := range res {
			res[ix].Lat, res[ix].Lng = utils.Blur(r.Lat, r.Lng, utils.BLUR_USER)
		}
	}

	// Return results where Msgid is not 0
	filtered := []SearchResult{}

	for _, r := range res {
		if r.Msgid != 0 {
			filtered = append(filtered, r)
		}
	}

	wg.Wait()

	return c.JSON(filtered)
}
