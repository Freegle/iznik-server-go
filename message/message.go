package message

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/freegle/iznik-server-go/auth"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/group"
	"github.com/freegle/iznik-server-go/item"
	"github.com/freegle/iznik-server-go/location"
	flog "github.com/freegle/iznik-server-go/log"
	"github.com/freegle/iznik-server-go/misc"
	"github.com/freegle/iznik-server-go/queue"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"gorm.io/gorm"
)

// Declaring the table name seems to help with a race seen in testing.
func (Message) TableName() string {
	return "messages"
}

// Message represents a posting (offer or wanted)
// swagger:model Message
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
	Unseen             bool                `json:"unseen"`
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
	Heldby           *uint64    `json:"heldby"`
	Source           *string    `json:"source"`
	Sourceheader     *string    `json:"sourceheader"`
	Fromaddr         *string    `json:"fromaddr"`
	Fromip           *string    `json:"fromip"`
	Fromcountry      *string    `json:"fromcountry"`
	Repostat           *time.Time          `json:"repostat"`
	Canrepost        bool       `json:"canrepost"`
	Deliverypossible bool       `json:"deliverypossible"`
	Deadline         *time.Time `json:"deadline"`
	Edits            []MessageEdit `json:"edits,omitempty" gorm:"-"`
	RawMessage       *string       `json:"message,omitempty" gorm:"column:message"`
}

type MessageEdit struct {
	ID              uint64     `json:"id"`
	Oldsubject      *string    `json:"oldsubject"`
	Newsubject      *string    `json:"newsubject"`
	Oldtext         *string    `json:"oldtext"`
	Newtext         *string    `json:"newtext"`
	Reviewrequired  int        `json:"reviewrequired"`
	Timestamp       *time.Time `json:"timestamp"`
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
			isMod := auth.IsSystemMod(myid)

			wg.Add(1)
			go func() {
				defer wg.Done()
				userDeletedFilter := "AND users.deleted IS NULL"
				rawMessageField := ""
				if isMod {
					userDeletedFilter = ""
					rawMessageField = "messages.message, "
				}
				err := db.Raw("SELECT messages.id, messages.arrival, messages.date, messages.fromuser, "+
					"messages.subject, messages.type, textbody, lat, lng, availablenow, availableinitially, locationid,"+
					"deliverypossible, deadline, heldby, messages.source, messages.sourceheader, messages.fromaddr, messages.fromip, messages.fromcountry, "+
					rawMessageField+
					"CASE WHEN messages_likes.msgid IS NULL THEN 1 ELSE 0 END AS unseen FROM messages "+
					"LEFT JOIN users ON users.id = messages.fromuser "+
					"LEFT JOIN messages_likes ON messages_likes.msgid = messages.id AND messages_likes.userid = ? AND messages_likes.type = 'View' "+
					"WHERE messages.id = ? AND messages.deleted IS NULL " + userDeletedFilter, myid, id).First(&message).Error
				found = !errors.Is(err, gorm.ErrRecordNotFound)
			}()

			var messageGroups []MessageGroup
			wg.Add(1)
			go func() {
				defer wg.Done()

				// Get messages_groups entries for this message.
				// Messages must have at least one entry in messages_groups to be publicly accessible.
				// This prevents internal messages (like chat messages received by email) from being
				// exposed on the public web.
				//
				// Both APPROVED and PENDING messages are visible to all users. This is not a privacy
				// issue because these messages were posted with the intention of being public. It also
				// allows shared links to work even before moderation approval.
				db.Raw("SELECT groupid, msgid, arrival, collection, autoreposts, approvedby FROM messages_groups WHERE msgid = ? AND deleted = 0", id).Scan(&messageGroups)
			}()

			var messageAttachments []MessageAttachment

			wg.Add(1)
			go func() {
				defer wg.Done()
				db.Raw("SELECT id, msgid, archived, externaluid, externalmods FROM messages_attachments WHERE msgid = ? ORDER BY `primary` DESC, id ASC", id).Scan(&messageAttachments)
			}()

			var messageReply []MessageReply
			wg.Add(1)
			go func() {
				defer wg.Done()

				// There is some strange case where people can end up replying to themselves.  Don't show such
				// replies.
				//
				// If someone has replied multiple times, we only want to return one of them, so group by userid.
				//
				// Check that the reply isn't too long ago compared to the most recent post of it.  That can happen
				// very occasionally if someone posts, an item for a long time, and there is a reply
				db.Raw("SELECT DISTINCT chat_messages.id, refmsgid, chat_messages.date, userid, fromuser, "+
					"CASE WHEN users.fullname IS NOT NULL THEN users.fullname ELSE CONCAT(users.firstname, ' ', users.lastname) END AS displayname "+
					"FROM chat_messages "+
					"INNER JOIN messages ON messages.id = chat_messages.refmsgid "+
					"INNER JOIN messages_groups ON messages_groups.msgid = messages.id "+
					"INNER JOIN users ON users.id = chat_messages.userid "+
					"WHERE refmsgid = ? AND chat_messages.type = ? AND (messages.fromuser != ? OR chat_messages.userid != ?) "+
					"AND reviewrequired = 0 AND reviewrejected = 0 "+
					"AND DATEDIFF(chat_messages.date, messages_groups.arrival) < ? "+
					"GROUP BY userid;", id, utils.MESSAGE_INTERESTED, myid, myid, utils.OPEN_AGE).Scan(&messageReply)

				tnre := regexp.MustCompile(utils.TN_REGEXP)

				for i, r := range messageReply {
					if r.Fromuser != myid {
						// Not our message so we shouldn't see who replied.
						messageReply[i].Userid = 0
						messageReply[i].Displayname = ""
					} else {
						messageReply[i].Displayname = tnre.ReplaceAllString(messageReply[i].Displayname, "$1")
					}
				}
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

			// Fetch pending edits (mod-only, for edit review page).
			var messageEdits []MessageEdit
			if isMod {
				wg.Add(1)
				go func() {
					defer wg.Done()
					result := db.Raw("SELECT id, oldsubject, newsubject, oldtext, newtext, reviewrequired, `timestamp` AS `timestamp` "+
						"FROM messages_edits WHERE msgid = ? AND reviewrequired = 1 AND approvedat IS NULL AND revertedat IS NULL "+
						"ORDER BY id DESC", id).Scan(&messageEdits)
					log.Printf("Edits query for message %s: rows=%d err=%v edits=%d", id, result.RowsAffected, result.Error, len(messageEdits))
				}()
			}

			wg.Wait()

			message.MessageGroups = messageGroups
			message.MessageAttachments = messageAttachments
			message.MessageReply = messageReply
			message.MessageOutcomes = messageOutcomes
			message.MessagePromises = messagePromises
			if isMod && len(messageEdits) > 0 {
				message.Edits = messageEdits
			}

			if found && (len(messageGroups) > 0 || isMod) {
				message.Replycount = len(message.MessageReply)
				message.MessageURL = "https://" + os.Getenv("USER_SITE") + "/message/" + strconv.FormatUint(message.ID, 10)

				// Compute nearby groups from original (unblurred) coords for mod use.
				if isMod && message.Lat != 0 && message.Lng != 0 {
					loc := &location.Location{}
					loc.GroupsNear = location.ClosestGroups(float64(message.Lat), float64(message.Lng), location.NEARBY, 10)
					message.Location = loc
				}

				// Protect anonymity of poster a bit.
				message.Lat, message.Lng = utils.Blur(message.Lat, message.Lng, utils.BLUR_USER)

				// source/fromip/fromcountry are mod-only fields.
				if !isMod {
					message.Source = nil
					message.Sourceheader = nil
					message.Fromaddr = nil
					message.Fromip = nil
					message.Fromcountry = nil
				}

				// Convert 2-letter country code to full name for frontend display.
				if message.Fromcountry != nil && len(*message.Fromcountry) == 2 {
					if name, ok := utils.CountryName(*message.Fromcountry); ok {
						message.Fromcountry = &name
					}
				}

				if myid == 0 {
					// Remove confidential info.
					message.Textbody = er.ReplaceAllString(message.Textbody, "***@***.com")
					message.Textbody = ep.ReplaceAllString(message.Textbody, "***")
				}

				// Get the paths.
				for i, a := range message.MessageAttachments {
					if a.Externaluid != "" {
						message.MessageAttachments[i].Ouruid = a.Externaluid
						message.MessageAttachments[i].Externalmods = a.Externalmods
						message.MessageAttachments[i].Path = misc.GetImageDeliveryUrl(a.Externaluid, string(a.Externalmods))
						message.MessageAttachments[i].Paththumb = misc.GetImageDeliveryUrl(a.Externaluid, string(a.Externalmods))
					} else if a.Archived > 0 {
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

	myid := user.WhoAmI(c)

	if c.Params("id") != "" {
		id, err1 := strconv.ParseUint(c.Params("id"), 10, 64)
		active, err2 := strconv.ParseBool(c.Query("active", "false"))

		if err1 == nil && err2 == nil {
			msgs := []MessageSummary{}

			sql := "SELECT messages.lat, messages.lng, messages.id, messages_groups.groupid, messages_groups.collection, messages.type, messages_groups.arrival, " +
				"messages_spatial.id AS spatialid, " +
				"EXISTS(SELECT id FROM messages_outcomes WHERE messages_outcomes.msgid = messages.id) AS hasoutcome, " +
				"EXISTS(SELECT id FROM messages_outcomes WHERE messages_outcomes.msgid = messages.id AND outcome IN (?, ?)) AS successful, " +
				"EXISTS(SELECT id FROM messages_promises WHERE messages_promises.msgid = messages.id) AS promised, "

			if myid > 0 && id == myid {
				// Own messages are always treated as seen.
				sql += "0 AS unseen "
			} else {
				sql += "NOT EXISTS(SELECT msgid FROM messages_likes WHERE messages_likes.msgid = messages.id AND messages_likes.userid = ? AND messages_likes.type = 'View') AS unseen "
			}

			sql += "FROM messages " +
				"INNER JOIN messages_groups ON messages_groups.msgid = messages.id " +
				"INNER JOIN users ON users.id = messages.fromuser "

			if active {
				if myid > 0 && id == myid {
					// For our own user, we might have messages which are not public yet because they're pending,
					// and we still want to show those.
					sql += "LEFT JOIN messages_spatial ON messages_spatial.msgid = messages.id "
				} else {
					// Another user - we are only interested in active and public messages.
					sql += "INNER JOIN messages_spatial ON messages_spatial.msgid = messages.id "
				}
			} else {
				sql += "LEFT JOIN messages_spatial ON messages_spatial.msgid = messages.id "
			}

			sql += "WHERE fromuser = ? AND messages.deleted IS NULL AND users.deleted IS NULL AND messages_groups.deleted = 0 AND " +
				"messages.type IN (?, ?)"

			if active {
				if myid > 0 && id == myid {
					sql += " HAVING ((hasoutcome = 0 AND spatialid IS NOT NULL) OR messages_groups.collection = '" + utils.COLLECTION_PENDING + "')"
				} else {
					sql += " HAVING hasoutcome = 0"
				}
			}

			sql += " ORDER BY unseen DESC, messages_groups.arrival DESC"

			if myid > 0 && id == myid {
				// Own messages - no unseen userid parameter needed.
				db.Raw(sql, utils.TAKEN, utils.RECEIVED, id, utils.OFFER, utils.WANTED).Scan(&msgs)
			} else {
				db.Raw(sql, utils.TAKEN, utils.RECEIVED, myid, id, utils.OFFER, utils.WANTED).Scan(&msgs)
			}

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

	// If groupids contains 0 ("All my communities" in ModTools), replace with the
	// user's actual group memberships to avoid returning messages from all groups.
	hasZero := false
	for _, gid := range groupids {
		if gid == 0 {
			hasZero = true
			break
		}
	}
	if hasZero && myid > 0 {
		var userGroupIDs []uint64
		db.Raw("SELECT groupid FROM memberships WHERE userid = ? AND collection = 'Approved'", myid).Scan(&userGroupIDs)
		if len(userGroupIDs) > 0 {
			groupids = userGroupIDs
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

// Activity represents a recent activity in groups
// swagger:model Activity
type Activity struct {
	ID      uint64          `json:"id"`
	Message ActivityMessage `json:"message"`
	Group   ActivityGroup   `json:"group"`
}

// ActivityMessage represents a message in an activity
// swagger:model ActivityMessage
type ActivityMessage struct {
	ID      uint64    `json:"id"`
	Subject string    `json:"subject"`
	Arrival time.Time `json:"arrival"`
	Delta   int64     `json:"delta"`
}

// ActivityGroup represents a group in an activity
// swagger:model ActivityGroup
type ActivityGroup struct {
	ID          uint64  `json:"id"`
	Nameshort   string  `json:"nameshort"`
	Namefull    string  `json:"-"`
	Namedisplay string  `json:"namedisplay"`
	Lat         float32 `json:"lat"`
	Lng         float32 `json:"lng"`
}

type ActivityQuery struct {
	Id        uint64
	Subject   string
	Arrival   time.Time
	Delta     int64
	Groupid   uint64
	Nameshort string
	Namefull  string
	Lat       float32
	Lng       float32
}

func GetRecentActivity(c *fiber.Ctx) error {
	var activity []ActivityQuery

	db := database.DBConn

	start := time.Now().Add(-time.Hour * 24).Format("2006-01-02 15:04:05")

	db.Raw("SELECT messages.id, messages_groups.arrival, messages_groups.groupid, messages.subject, "+
		"groups.nameshort, groups.namefull, groups.lat, groups.lng "+
		"FROM messages "+
		"INNER JOIN messages_groups ON messages.id = messages_groups.msgid "+
		"INNER JOIN `groups` ON messages_groups.groupid = groups.id "+
		"INNER JOIN users ON messages.fromuser = users.id "+
		"WHERE messages_groups.arrival > ? AND collection = ? "+
		"ORDER BY messages_groups.arrival ASC LIMIT 100;",
		start,
		utils.COLLECTION_APPROVED).Scan(&activity)

	last := int64(0)

	var ret []Activity

	for _, r := range activity {
		namedisplay := r.Nameshort

		if len(r.Namefull) > 0 {
			namedisplay = r.Namefull
		}

		arrival := r.Arrival.Unix()
		delta := int64(0)

		if last != 0 {
			delta = arrival - last
		}

		last = arrival

		ret = append(ret, Activity{
			ID: r.Id,
			Message: ActivityMessage{
				ID:      r.Id,
				Subject: r.Subject,
				Arrival: r.Arrival,
				Delta:   delta,
			},
			Group: ActivityGroup{
				ID:          r.Groupid,
				Lat:         r.Lat,
				Lng:         r.Lng,
				Nameshort:   r.Nameshort,
				Namefull:    r.Namefull,
				Namedisplay: namedisplay,
			},
		})
	}

	return c.JSON(ret)
}

// =============================================================================
// Merged from message/message_mod.go
// =============================================================================

// logModAction inserts a mod log entry for message actions (approve, reject, reply, etc).
func logModAction(db *gorm.DB, logType string, subtype string, groupid uint64, userid uint64, byuser uint64, msgid uint64, stdmsgid uint64, text string) {
	if stdmsgid > 0 {
		db.Exec("INSERT INTO logs (timestamp, type, subtype, groupid, user, byuser, msgid, stdmsgid, text) VALUES (NOW(), ?, ?, ?, ?, ?, ?, ?, ?)",
			logType, subtype, groupid, userid, byuser, msgid, stdmsgid, text)
	} else {
		db.Exec("INSERT INTO logs (timestamp, type, subtype, groupid, user, byuser, msgid, text) VALUES (NOW(), ?, ?, ?, ?, ?, ?, ?)",
			logType, subtype, groupid, userid, byuser, msgid, text)
	}
}

// getPrimaryGroupForMessage returns the first groupid for a message.
func getPrimaryGroupForMessage(db *gorm.DB, msgid uint64) uint64 {
	var groupid uint64
	db.Raw("SELECT groupid FROM messages_groups WHERE msgid = ? LIMIT 1", msgid).Scan(&groupid)
	return groupid
}

// getAllGroupsForMessage returns all groupids for a message.
func getAllGroupsForMessage(db *gorm.DB, msgid uint64) []uint64 {
	var groupids []uint64
	db.Raw("SELECT groupid FROM messages_groups WHERE msgid = ?", msgid).Scan(&groupids)
	return groupids
}

// isModForMessage checks if the user is a system admin/support or a moderator/owner
// of any group the message is on.
func isModForMessage(db *gorm.DB, myid uint64, msgid uint64) bool {
	// Check system admin/support.
	if auth.IsAdminOrSupport(myid) {
		return true
	}

	// Check if mod of any group the message is on.
	var count int64
	result := db.Raw(`SELECT COUNT(*) FROM messages_groups mg
		JOIN memberships m ON m.groupid = mg.groupid
		WHERE mg.msgid = ? AND m.userid = ? AND m.role IN ('Moderator', 'Owner')`, msgid, myid).Scan(&count)
	if result.Error != nil {
		log.Printf("Failed to check mod permission for user %d message %d: %v", myid, msgid, result.Error)
		return false
	}
	return count > 0
}

// MessageModContext holds common context needed by mod action handlers.
type MessageModContext struct {
	Fromuser uint64
	Groupid  uint64
	Groupids []uint64
	Subject  string
}

// getMessageModContext checks mod permission and fetches common context for mod actions.
// Returns nil if the user is not a moderator for this message.
func getMessageModContext(db *gorm.DB, myid uint64, msgid uint64) *MessageModContext {
	if !isModForMessage(db, myid, msgid) {
		return nil
	}
	ctx := &MessageModContext{}
	row := db.Raw("SELECT fromuser, subject FROM messages WHERE id = ?", msgid).Row()
	if err := row.Scan(&ctx.Fromuser, &ctx.Subject); err != nil {
		log.Printf("Failed to fetch mod context for message %d: %v", msgid, err)
		return nil
	}
	ctx.Groupid = getPrimaryGroupForMessage(db, msgid)
	ctx.Groupids = getAllGroupsForMessage(db, msgid)
	return ctx
}

// logAndNotifyMods logs a mod action and queues push notifications to moderators of all groups the message is on.
func logAndNotifyMods(db *gorm.DB, subtype string, ctx *MessageModContext, myid uint64, msgid uint64, stdmsgid uint64, text string) {
	logModAction(db, flog.LOG_TYPE_MESSAGE, subtype, ctx.Groupid, ctx.Fromuser, myid, msgid, stdmsgid, text)
	for _, gid := range ctx.Groupids {
		if err := queue.QueueTask(queue.TaskPushNotifyGroupMods, map[string]interface{}{
			"group_id": gid,
		}); err != nil {
			log.Printf("Failed to queue push notification for group %d: %v", gid, err)
		}
	}
}

// handleApprove approves a pending message.
func handleApprove(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	ctx := getMessageModContext(db, myid, req.ID)
	if ctx == nil {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	// Use request groupid if provided, otherwise fall back to context.
	if req.Groupid != nil && *req.Groupid > 0 {
		ctx.Groupid = *req.Groupid
	}
	groupid := ctx.Groupid

	// Move to Approved with arrival=NOW() so immediate-email recipients get it.
	// Guard against double-approve by requiring collection != 'Approved'.
	if req.Groupid != nil && *req.Groupid > 0 {
		if result := db.Exec("UPDATE messages_groups SET collection = 'Approved', approvedby = ?, approvedat = NOW(), arrival = NOW() WHERE msgid = ? AND groupid = ? AND collection != 'Approved'",
			myid, req.ID, groupid); result.Error != nil {
			log.Printf("Failed to approve message %d group %d: %v", req.ID, groupid, result.Error)
		}
	} else {
		if result := db.Exec("UPDATE messages_groups SET collection = 'Approved', approvedby = ?, approvedat = NOW(), arrival = NOW() WHERE msgid = ? AND collection != 'Approved'",
			myid, req.ID); result.Error != nil {
			log.Printf("Failed to approve message %d: %v", req.ID, result.Error)
		}
	}

	// Release any hold.
	db.Exec("UPDATE messages SET heldby = NULL WHERE id = ?", req.ID)

	// Mark as ham if it was flagged as spam (matching V1 Message::notSpam).
	var spamtype *string
	db.Raw("SELECT spamtype FROM messages WHERE id = ?", req.ID).Scan(&spamtype)
	if spamtype != nil && *spamtype != "" {
		db.Exec("REPLACE INTO messages_spamham (msgid, spamham) VALUES (?, 'Ham')", req.ID)
	}

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

	// Queue email to poster (includes stdmsg content for the batch processor).
	db.Exec("INSERT INTO background_tasks (task_type, data) VALUES (?, JSON_OBJECT('msgid', ?, 'groupid', ?, 'byuser', ?, 'subject', ?, 'body', ?, 'stdmsgid', ?))",
		"email_message_approved", req.ID, groupid, myid, subject, body, stdmsgid)

	// Log the approval and notify group moderators (V1 logs subject as the text field).
	logAndNotifyMods(db, flog.LOG_SUBTYPE_APPROVED, ctx, myid, req.ID, stdmsgid, subject)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleReject rejects a pending message.
func handleReject(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	ctx := getMessageModContext(db, myid, req.ID)
	if ctx == nil {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

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

	// Use request groupid if provided, otherwise fall back to context.
	if req.Groupid != nil && *req.Groupid > 0 {
		ctx.Groupid = *req.Groupid
	}
	groupid := ctx.Groupid

	// V1 behavior: with a subject (stdmsg), move to Rejected collection (user can edit and resubmit).
	// Without a subject (plain delete), mark as deleted.
	if subject != "" {
		if groupid > 0 {
			if result := db.Exec("UPDATE messages_groups SET collection = 'Rejected', rejectedat = NOW() WHERE msgid = ? AND groupid = ? AND collection = 'Pending'", req.ID, groupid); result.Error != nil {
				log.Printf("Failed to reject message %d group %d: %v", req.ID, groupid, result.Error)
			}
		} else {
			if result := db.Exec("UPDATE messages_groups SET collection = 'Rejected', rejectedat = NOW() WHERE msgid = ? AND collection = 'Pending'", req.ID); result.Error != nil {
				log.Printf("Failed to reject message %d: %v", req.ID, result.Error)
			}
		}
	} else {
		if groupid > 0 {
			if result := db.Exec("UPDATE messages_groups SET deleted = 1 WHERE msgid = ? AND groupid = ? AND collection = 'Pending'", req.ID, groupid); result.Error != nil {
				log.Printf("Failed to delete pending message %d group %d: %v", req.ID, groupid, result.Error)
			}
		} else {
			if result := db.Exec("UPDATE messages_groups SET deleted = 1 WHERE msgid = ? AND collection = 'Pending'", req.ID); result.Error != nil {
				log.Printf("Failed to delete pending message %d: %v", req.ID, result.Error)
			}
		}
	}

	// Queue rejection email.
	db.Exec("INSERT INTO background_tasks (task_type, data) VALUES (?, JSON_OBJECT('msgid', ?, 'groupid', ?, 'byuser', ?, 'subject', ?, 'body', ?, 'stdmsgid', ?))",
		"email_message_rejected", req.ID, groupid, myid, subject, body, stdmsgid)

	// Log the rejection and notify group moderators (V1 logs subject as the text field).
	logAndNotifyMods(db, flog.LOG_SUBTYPE_REJECTED, ctx, myid, req.ID, stdmsgid, subject)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleDeleteMessage deletes a message (mod action).
func handleDeleteMessage(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	// Get context before deleting (needs messages_groups rows).
	ctx := getMessageModContext(db, myid, req.ID)
	if ctx == nil {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	// Use request groupid if provided, otherwise fall back to context.
	if req.Groupid != nil && *req.Groupid > 0 {
		ctx.Groupid = *req.Groupid
	}
	groupid := ctx.Groupid

	if result := db.Exec("DELETE FROM messages_groups WHERE msgid = ?", req.ID); result.Error != nil {
		log.Printf("Failed to delete messages_groups for message %d: %v", req.ID, result.Error)
	}
	if result := db.Exec("UPDATE messages SET deleted = NOW(), messageid = NULL WHERE id = ?", req.ID); result.Error != nil {
		log.Printf("Failed to soft-delete message %d: %v", req.ID, result.Error)
	}

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

	// Queue email if stdmsg content provided (delete with a standard message).
	if subject != "" || body != "" {
		db.Exec("INSERT INTO background_tasks (task_type, data) VALUES (?, JSON_OBJECT('msgid', ?, 'groupid', ?, 'byuser', ?, 'subject', ?, 'body', ?, 'stdmsgid', ?))",
			"email_message_rejected", req.ID, groupid, myid, subject, body, stdmsgid)
	}

	// Log the deletion and notify group moderators.
	logAndNotifyMods(db, flog.LOG_SUBTYPE_DELETED, ctx, myid, req.ID, stdmsgid, body)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleSpam marks a message as spam.
func handleSpam(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if !isModForMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	// Record for spam training (matching PHP Message::spam).
	db.Exec("REPLACE INTO messages_spamham (msgid, spamham) VALUES (?, 'Spam')", req.ID)

	// Delete the message (matching PHP - spam() calls delete()).
	db.Exec("UPDATE messages_groups SET deleted = 1 WHERE msgid = ?", req.ID)
	db.Exec("UPDATE messages SET deleted = NOW() WHERE id = ?", req.ID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleHold holds a pending message (assigns heldby to the mod).
func handleHold(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	ctx := getMessageModContext(db, myid, req.ID)
	if ctx == nil {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	db.Exec("UPDATE messages SET heldby = ? WHERE id = ?", myid, req.ID)

	logAndNotifyMods(db, flog.LOG_SUBTYPE_HOLD, ctx, myid, req.ID, 0, "")

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleBackToPending moves an approved message back to pending.
func handleBackToPending(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	ctx := getMessageModContext(db, myid, req.ID)
	if ctx == nil {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	// Hold the message for re-review (V1 parity: calls hold() before moving to Pending).
	db.Exec("UPDATE messages SET heldby = ? WHERE id = ?", myid, req.ID)

	// Move from Approved back to Pending. If groupid is specified, only for that group (cross-post support).
	if req.Groupid != nil && *req.Groupid > 0 {
		db.Exec("UPDATE messages_groups SET collection = 'Pending', approvedby = NULL, approvedat = NULL WHERE msgid = ? AND groupid = ? AND collection = 'Approved'",
			req.ID, *req.Groupid)
	} else {
		db.Exec("UPDATE messages_groups SET collection = 'Pending', approvedby = NULL, approvedat = NULL WHERE msgid = ? AND collection = 'Approved'",
			req.ID)
	}

	// Log and notify (V1 parity).
	logAndNotifyMods(db, flog.LOG_SUBTYPE_HOLD, ctx, myid, req.ID, 0, "Back to pending")

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleRelease releases a held message.
func handleRelease(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	ctx := getMessageModContext(db, myid, req.ID)
	if ctx == nil {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	db.Exec("UPDATE messages SET heldby = NULL WHERE id = ?", req.ID)

	logAndNotifyMods(db, flog.LOG_SUBTYPE_RELEASE, ctx, myid, req.ID, 0, "")

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleApproveEdits approves pending edits on a message.
func handleApproveEdits(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if !isModForMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	// Clear the editedby flag.
	db.Exec("UPDATE messages SET editedby = NULL WHERE id = ?", req.ID)

	// Find the latest pending edit.
	type editRecord struct {
		ID         uint64
		Newsubject *string
		Newtext    *string
	}
	var edit editRecord
	db.Raw("SELECT id, newsubject, newtext FROM messages_edits WHERE msgid = ? AND reviewrequired = 1 AND approvedat IS NULL AND revertedat IS NULL ORDER BY id DESC LIMIT 1",
		req.ID).Scan(&edit)

	if edit.ID > 0 {
		// Apply the edits.
		if edit.Newsubject != nil {
			db.Exec("UPDATE messages SET subject = ? WHERE id = ?", *edit.Newsubject, req.ID)
		}
		if edit.Newtext != nil {
			db.Exec("UPDATE messages SET textbody = ? WHERE id = ?", *edit.Newtext, req.ID)
		}
		// Mark as approved.
		db.Exec("UPDATE messages_edits SET approvedat = NOW() WHERE id = ?", edit.ID)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleRevertEdits reverts pending edits on a message.
func handleRevertEdits(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if !isModForMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	// Clear the editedby flag.
	db.Exec("UPDATE messages SET editedby = NULL WHERE id = ?", req.ID)

	// Mark all pending edits as reverted.
	db.Exec("UPDATE messages_edits SET revertedat = NOW() WHERE msgid = ? AND reviewrequired = 1 AND approvedat IS NULL AND revertedat IS NULL",
		req.ID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handlePartnerConsent records partner consent on a message.
// Matches PHP Message.php:partnerConsent() - requires mod role and partner name.
func handlePartnerConsent(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if !isModForMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	if req.Partner == nil || *req.Partner == "" {
		return fiber.NewError(fiber.StatusBadRequest, "partner is required")
	}

	// Look up partner in partners_keys.
	var partnerID uint64
	db.Raw("SELECT id FROM partners_keys WHERE partner = ?", *req.Partner).Scan(&partnerID)
	if partnerID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Partner not found")
	}

	// Record consent in partners_messages.
	db.Exec("INSERT IGNORE INTO partners_messages (partnerid, msgid) VALUES (?, ?)", partnerID, req.ID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleReply queues a mod reply email to the message poster.
func handleReply(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	ctx := getMessageModContext(db, myid, req.ID)
	if ctx == nil {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

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

	// Use request groupid if provided, otherwise fall back to context.
	if req.Groupid != nil && *req.Groupid > 0 {
		ctx.Groupid = *req.Groupid
	}

	db.Exec("INSERT INTO background_tasks (task_type, data) VALUES (?, JSON_OBJECT('msgid', ?, 'groupid', ?, 'byuser', ?, 'subject', ?, 'body', ?, 'stdmsgid', ?))",
		"email_message_reply", req.ID, ctx.Groupid, myid, subject, body, stdmsgid)

	// Log the reply (V1 logs subject as the text field). No push notification for replies.
	logModAction(db, flog.LOG_TYPE_MESSAGE, flog.LOG_SUBTYPE_REPLIED, ctx.Groupid, ctx.Fromuser, myid, req.ID, stdmsgid, subject)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleJoinAndPost joins a group and posts a message in one action.
func handleJoinAndPost(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	// Look up the existing draft message.
	type msgInfo struct {
		Fromuser uint64
		Type     string
	}
	var msg msgInfo
	db.Raw("SELECT fromuser, type FROM messages WHERE id = ?", req.ID).Scan(&msg)
	if msg.Fromuser == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}
	if msg.Fromuser != myid {
		return fiber.NewError(fiber.StatusForbidden, "Not your message")
	}

	// Find the group — from request, then messages_drafts, then messages_groups.
	groupid := uint64(0)
	if req.Groupid != nil && *req.Groupid > 0 {
		groupid = *req.Groupid
	} else {
		db.Raw("SELECT groupid FROM messages_drafts WHERE msgid = ? LIMIT 1", req.ID).Scan(&groupid)
	}
	if groupid == 0 {
		groupid = getPrimaryGroupForMessage(db, req.ID)
	}
	if groupid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "groupid is required")
	}

	// Check if user is banned from this group (V1 parity).
	var bannedCount int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ? AND collection = ?", myid, groupid, utils.COLLECTION_BANNED).Scan(&bannedCount)
	if bannedCount > 0 {
		return fiber.NewError(fiber.StatusForbidden, "You are banned from this group")
	}

	// Join group if not already a member.
	db.Exec("INSERT IGNORE INTO memberships (userid, groupid, role, collection) VALUES (?, ?, 'Member', 'Approved')",
		myid, groupid)

	// Determine collection based on user's posting status and group settings (V1 parity).
	collection := utils.COLLECTION_APPROVED
	var ourPostingStatus *string
	db.Raw("SELECT ourPostingStatus FROM memberships WHERE userid = ? AND groupid = ?", myid, groupid).Scan(&ourPostingStatus)

	if ourPostingStatus != nil && strings.EqualFold(*ourPostingStatus, utils.POSTING_STATUS_PROHIBITED) {
		return fiber.NewError(fiber.StatusForbidden, "You are not allowed to post on this group")
	}

	if ourPostingStatus != nil && strings.EqualFold(*ourPostingStatus, utils.POSTING_STATUS_MODERATED) {
		// User is explicitly moderated on this group — message goes to Pending.
		collection = utils.COLLECTION_PENDING
	} else if ourPostingStatus == nil || strings.EqualFold(*ourPostingStatus, utils.POSTING_STATUS_DEFAULT) || *ourPostingStatus == "" {
		// Check the group's default posting status.
		var defaultPostingStatus *string
		db.Raw("SELECT JSON_EXTRACT(settings, '$.defaultpostingstatus') FROM `groups` WHERE id = ?", groupid).Scan(&defaultPostingStatus)
		if defaultPostingStatus != nil && (strings.EqualFold(*defaultPostingStatus, utils.POSTING_STATUS_MODERATED) || strings.EqualFold(*defaultPostingStatus, "\""+utils.POSTING_STATUS_MODERATED+"\"")) {
			collection = utils.COLLECTION_PENDING
		}
	}

	// Submit: insert into messages_groups and clean up draft.
	db.Exec("INSERT IGNORE INTO messages_groups (msgid, groupid, collection, arrival) VALUES (?, ?, ?, NOW())",
		req.ID, groupid, collection)
	db.Exec("DELETE FROM messages_drafts WHERE msgid = ?", req.ID)

	// Notify group moderators about the new message (V1 parity: notifyGroupMods in submit()).
	if collection == utils.COLLECTION_PENDING {
		if err := queue.QueueTask(queue.TaskPushNotifyGroupMods, map[string]interface{}{
			"group_id": groupid,
		}); err != nil {
			log.Printf("Failed to queue push notification for group %d on submit: %v", groupid, err)
		}
	}

	// Check if user has a password (to determine if they're a new user).
	var hasPassword int64
	db.Raw("SELECT COUNT(*) FROM users_logins WHERE userid = ? AND type = 'Native'", myid).Scan(&hasPassword)

	resp := fiber.Map{
		"ret":     0,
		"status":  "Success",
		"id":      req.ID,
		"groupid": groupid,
	}

	if hasPassword == 0 {
		// New user without a password — generate one and return it.
		password := utils.RandomHex(8)
		salt := auth.GetPasswordSalt()
		hashed := auth.HashPassword(password, salt)

		// uid must be the user ID (not email) so that VerifyPassword can find the row.
		db.Exec("INSERT INTO users_logins (userid, type, uid, credentials, salt) VALUES (?, 'Native', ?, ?, ?) ON DUPLICATE KEY UPDATE credentials = VALUES(credentials), salt = VALUES(salt)",
			myid, myid, hashed, salt)
		resp["newuser"] = true
		resp["newpassword"] = password
	}

	return c.JSON(resp)
}

// PatchMessage updates a message (PATCH /message).
//
// @Summary Update a message
// @Tags message
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/message [patch]
func PatchMessage(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	type PatchMessageRequest struct {
		ID           uint64   `json:"id"`
		Subject      *string  `json:"subject"`
		Textbody     *string  `json:"textbody"`
		Type         *string  `json:"type"`
		Item         *string  `json:"item"`
		Availablenow *int     `json:"availablenow"`
		Lat          *float64 `json:"lat"`
		Lng          *float64 `json:"lng"`
		Locationid   *uint64  `json:"locationid"`
		Attachments  []uint64 `json:"attachments"`
	}

	var req PatchMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	db := database.DBConn

	// Check ownership or mod permission.
	var fromuser uint64
	db.Raw("SELECT fromuser FROM messages WHERE id = ?", req.ID).Scan(&fromuser)
	if fromuser == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}

	isOwner := fromuser == myid
	isMod := isModForMessage(db, myid, req.ID)

	if !isOwner && !isMod {
		return fiber.NewError(fiber.StatusForbidden, "Not allowed to modify this message")
	}

	// Get old values for edit tracking.
	type msgValues struct {
		Subject  string
		Textbody string
	}
	var old msgValues
	db.Raw("SELECT subject, COALESCE(textbody, '') as textbody FROM messages WHERE id = ?", req.ID).Scan(&old)

	// Build a single UPDATE with all changed fields.
	setClauses := []string{}
	args := []interface{}{}

	if req.Subject != nil {
		setClauses = append(setClauses, "subject = ?")
		args = append(args, *req.Subject)
	}
	if req.Textbody != nil {
		setClauses = append(setClauses, "textbody = ?")
		args = append(args, *req.Textbody)
	}
	if req.Type != nil {
		setClauses = append(setClauses, "type = ?")
		args = append(args, *req.Type)
	}
	if req.Availablenow != nil {
		setClauses = append(setClauses, "availablenow = ?")
		args = append(args, *req.Availablenow)
	}
	if req.Locationid != nil {
		setClauses = append(setClauses, "locationid = ?")
		args = append(args, *req.Locationid)
	}

	if len(setClauses) > 0 {
		args = append(args, req.ID)
		db.Exec("UPDATE messages SET "+strings.Join(setClauses, ", ")+" WHERE id = ?", args...)
	}

	// Issue 1: If the message OWNER edits a rejected message, move back to Pending for re-review.
	// Mods editing a rejected message should NOT auto-resubmit it.
	if fromuser == myid {
		db.Exec("UPDATE messages_groups SET collection = ? WHERE msgid = ? AND collection = ?", utils.COLLECTION_PENDING, req.ID, utils.COLLECTION_REJECTED)
	}

	// Issue 2: Log the edit (V1 parity: type='Message', subtype='Edit').
	logModAction(db, flog.LOG_TYPE_MESSAGE, flog.LOG_SUBTYPE_EDIT, 0, fromuser, myid, req.ID, 0, "Message edited")

	// Update attachment ordering if provided.
	if len(req.Attachments) > 0 {
		for i, attid := range req.Attachments {
			primary := i == 0
			db.Exec("UPDATE messages_attachments SET msgid = ?, `primary` = ? WHERE id = ?", req.ID, primary, attid)
		}

		// Delete any attachments for this message that are not in the new list.
		db.Exec("DELETE FROM messages_attachments WHERE msgid = ? AND id NOT IN (?)", req.ID, req.Attachments)
	}

	// If subject or textbody changed and user is not mod, create edit record for review.
	subjectChanged := req.Subject != nil && *req.Subject != old.Subject
	textChanged := req.Textbody != nil && *req.Textbody != old.Textbody

	if (subjectChanged || textChanged) && !isMod {
		newSubject := old.Subject
		if req.Subject != nil {
			newSubject = *req.Subject
		}
		newText := old.Textbody
		if req.Textbody != nil {
			newText = *req.Textbody
		}

		db.Exec("INSERT INTO messages_edits (msgid, byuser, oldsubject, newsubject, oldtext, newtext, reviewrequired) VALUES (?, ?, ?, ?, ?, ?, 1)",
			req.ID, myid, old.Subject, newSubject, old.Textbody, newText)
		db.Exec("UPDATE messages SET editedby = ? WHERE id = ?", myid, req.ID)

		// Issue 3: Notify group mods that an edit needs review (V1 parity: notifyGroupMods).
		groupIDs := getAllGroupsForMessage(db, req.ID)
		for _, gid := range groupIDs {
			if err := queue.QueueTask(queue.TaskPushNotifyGroupMods, map[string]interface{}{
				"group_id": gid,
			}); err != nil {
				log.Printf("Failed to queue push notification for group %d on edit review: %v", gid, err)
			}
		}
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// DeleteMessageEndpoint handles DELETE /message/:id.
//
// @Summary Delete a message
// @Tags message
// @Produce json
// @Param id path integer true "Message ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/message/{id} [delete]
func DeleteMessageEndpoint(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid message ID")
	}
	msgid := uint64(id)

	db := database.DBConn

	// Check ownership.
	var fromuser uint64
	db.Raw("SELECT fromuser FROM messages WHERE id = ?", msgid).Scan(&fromuser)
	if fromuser == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}

	if fromuser != myid && !isModForMessage(db, myid, msgid) {
		return fiber.NewError(fiber.StatusForbidden, "Not allowed to delete this message")
	}

	db.Exec("UPDATE messages SET deleted = NOW() WHERE id = ?", msgid)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// findOrCreateUserForDraft looks up a user by email, or creates one if not found.
// Returns the user ID, JWT string, persistent token map, and any error.
// This supports the give/want flow where users post without signing up first.
//
// SECURITY: For existing users, we do NOT create a session/JWT. Knowing someone's
// email address must not grant authentication. A session is only created for
// brand-new users.
func findOrCreateUserForDraft(db *gorm.DB, email string) (uint64, string, fiber.Map, error) {
	email = strings.TrimSpace(email)

	// Basic email validation.
	if !strings.Contains(email, "@") || len(email) > 254 {
		return 0, "", nil, fmt.Errorf("invalid email address")
	}

	// Look up existing user by email.
	var existingUID uint64
	db.Raw("SELECT userid FROM users_emails WHERE email = ? LIMIT 1", email).Scan(&existingUID)

	if existingUID > 0 {
		// Existing user — return their ID so the draft is linked to them,
		// but do NOT create a session.  The user must authenticate separately.
		return existingUID, "", nil, nil
	}

	// New user — create user, email, session, JWT.
	// Use raw database/sql to get LastInsertId() from the same result —
	// avoids the GORM connection-pool race where a separate
	// SELECT LAST_INSERT_ID() query could land on a different connection.
	sqlDB, err := db.DB()
	if err != nil {
		return 0, "", nil, fmt.Errorf("failed to get DB connection: %w", err)
	}

	sqlResult, err := sqlDB.Exec("INSERT INTO users (added) VALUES (NOW())")
	if err != nil {
		return 0, "", nil, fmt.Errorf("failed to create user: %w", err)
	}

	newUserIDInt, err := sqlResult.LastInsertId()
	if err != nil || newUserIDInt == 0 {
		return 0, "", nil, fmt.Errorf("failed to get new user ID")
	}
	newUserID := uint64(newUserIDInt)

	// Add email.
	canon := user.CanonicalizeEmail(email)
	db.Exec("INSERT INTO users_emails (userid, email, preferred, validated, canon) VALUES (?, ?, 1, NOW(), ?)",
		newUserID, email, canon)

	// Create session.
	token := utils.RandomHex(16)
	db.Exec("INSERT INTO sessions (userid, series, token, lastactive) VALUES (?, ?, ?, NOW())",
		newUserID, newUserID, token)

	// Use token to find our specific session (avoids race with concurrent requests).
	var sessionID uint64
	db.Raw("SELECT id FROM sessions WHERE userid = ? AND token = ? ORDER BY id DESC LIMIT 1", newUserID, token).Scan(&sessionID)

	// Generate JWT.
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":        fmt.Sprint(newUserID),
		"sessionid": fmt.Sprint(sessionID),
		"exp":       time.Now().Unix() + 30*24*60*60,
	})
	jwtString, err := jwtToken.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return 0, "", nil, err
	}

	persistent := fiber.Map{
		"id":     sessionID,
		"series": newUserID,
		"token":  token,
		"userid": newUserID,
	}
	return newUserID, jwtString, persistent, nil
}

// PutMessage creates a new message draft (PUT /message).
// Accepts both authenticated and unauthenticated requests (with email).
// For unauthenticated requests, finds or creates the user by email.
//
// @Summary Create or update a message
// @Tags message
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/message [put]
func PutMessage(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	type PutMessageRequest struct {
		Groupid            uint64   `json:"groupid"`
		Type               string   `json:"type"`
		Messagetype        string   `json:"messagetype"` // Client sends this; alias for Type.
		Subject            string   `json:"subject"`
		Item               string   `json:"item"`
		Textbody           string   `json:"textbody"`
		Collection         string   `json:"collection"` // Draft (default) or Pending.
		Locationid         *uint64  `json:"locationid"`
		Availableinitially *int     `json:"availableinitially"`
		Availablenow       *int     `json:"availablenow"`
		Attachments        []uint64 `json:"attachments"`
		Email              string   `json:"email"`
	}

	var req PutMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	// Handle messagetype alias from client.
	if req.Type == "" && req.Messagetype != "" {
		req.Type = req.Messagetype
	}

	// Generate subject from type + item if subject not provided.
	if req.Subject == "" && req.Item != "" {
		req.Subject = req.Type + ": " + req.Item
	}

	// Default to Draft collection (client compose flow creates drafts).
	if req.Collection == "" {
		req.Collection = "Draft"
	}

	db := database.DBConn

	// Handle unauthenticated user with email — find or create, then generate JWT.
	var jwtString string
	var persistent fiber.Map
	if myid == 0 && req.Email != "" {
		var err error
		myid, jwtString, persistent, err = findOrCreateUserForDraft(db, req.Email)
		if err != nil {
			if strings.Contains(err.Error(), "invalid email") {
				return fiber.NewError(fiber.StatusBadRequest, "Invalid email address")
			}
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to create user")
		}
	}

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	if req.Type != "Offer" && req.Type != "Wanted" {
		return fiber.NewError(fiber.StatusBadRequest, "type must be Offer or Wanted")
	}

	// For non-Draft, require group membership.
	if req.Collection != "Draft" && req.Groupid > 0 {
		var memberCount int64
		db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ?", myid, req.Groupid).Scan(&memberCount)
		if memberCount == 0 {
			return fiber.NewError(fiber.StatusForbidden, "Not a member of this group")
		}
	}

	availInit := 1
	if req.Availableinitially != nil {
		availInit = *req.Availableinitially
	}
	availNow := availInit
	if req.Availablenow != nil {
		availNow = *req.Availablenow
	}

	// Create message.
	result := db.Exec("INSERT INTO messages (fromuser, type, subject, textbody, arrival, date, source, availableinitially, availablenow) VALUES (?, ?, ?, ?, NOW(), NOW(), 'Platform', ?, ?)",
		myid, req.Type, req.Subject, req.Textbody, availInit, availNow)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create message")
	}

	var newMsgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", myid).Scan(&newMsgID)

	if newMsgID == 0 {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to retrieve message ID")
	}

	// For Draft collection, store in messages_drafts (matching PHP behavior).
	// For other collections, add to messages_groups.
	if req.Collection == "Draft" {
		db.Exec("INSERT INTO messages_drafts (msgid, groupid, userid) VALUES (?, ?, ?)",
			newMsgID, req.Groupid, myid)
	} else if req.Groupid > 0 {
		db.Exec("INSERT INTO messages_groups (msgid, groupid, collection, arrival) VALUES (?, ?, ?, NOW())",
			newMsgID, req.Groupid, req.Collection)
	}

	// Link attachments.
	for _, attID := range req.Attachments {
		db.Exec("UPDATE messages_attachments SET msgid = ? WHERE id = ?", newMsgID, attID)
	}

	// Add spatial data if locationid is provided, and update the user's last known location
	// (matching PHP behavior so that GET /isochrone can auto-create an isochrone for the user).
	if req.Locationid != nil && *req.Locationid > 0 {
		db.Exec("UPDATE users SET lastlocation = ? WHERE id = ?", *req.Locationid, myid)

		var lat, lng float64
		db.Raw("SELECT lat, lng FROM locations WHERE id = ?", *req.Locationid).Row().Scan(&lat, &lng)
		if lat != 0 || lng != 0 {
			db.Exec("INSERT INTO messages_spatial (msgid, point, successful, groupid, msgtype) VALUES (?, ST_GeomFromText(CONCAT('POINT(', ?, ' ', ?, ')'), 3857), 1, ?, ?)",
				newMsgID, lng, lat, req.Groupid, req.Type)
		}
	}

	resp := fiber.Map{"ret": 0, "status": "Success", "id": newMsgID}
	if jwtString != "" {
		resp["jwt"] = jwtString
		resp["persistent"] = persistent
	}
	return c.JSON(resp)
}

// =============================================================================
// Merged from message/message_write.go
// =============================================================================

// PostMessageRequest handles action-based POST to /message.
type PostMessageRequest struct {
	ID        uint64  `json:"id"`
	Action    string  `json:"action"`
	Userid    *uint64 `json:"userid"`
	Count     *int    `json:"count"`
	Outcome   string  `json:"outcome"`
	Happiness *string `json:"happiness"`
	Comment   *string `json:"comment"`
	Message   *string `json:"message"`
	Subject   *string `json:"subject"`
	Body      *string `json:"body"`
	Stdmsgid  *uint64 `json:"stdmsgid"`
	Groupid   *uint64 `json:"groupid"`
	Type      string  `json:"type"`
	Textbody  *string `json:"textbody"`
	Item      *string `json:"item"`
	Partner   *string `json:"partner"`
}

// PostMessage dispatches POST /message actions.
func PostMessage(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PostMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	switch req.Action {
	case "Promise":
		return handlePromise(c, myid, req)
	case "Renege":
		return handleRenege(c, myid, req)
	case "OutcomeIntended":
		return handleOutcomeIntended(c, myid, req)
	case "Outcome":
		return handleOutcome(c, myid, req)
	case "AddBy":
		return handleAddBy(c, myid, req)
	case "RemoveBy":
		return handleRemoveBy(c, myid, req)
	case "View":
		return handleView(c, myid, req)
	case "Approve":
		return handleApprove(c, myid, req)
	case "Reject":
		return handleReject(c, myid, req)
	case "Delete":
		return handleDeleteMessage(c, myid, req)
	case "Spam":
		return handleSpam(c, myid, req)
	case "Hold":
		return handleHold(c, myid, req)
	case "Release":
		return handleRelease(c, myid, req)
	case "ApproveEdits":
		return handleApproveEdits(c, myid, req)
	case "RevertEdits":
		return handleRevertEdits(c, myid, req)
	case "PartnerConsent":
		return handlePartnerConsent(c, myid, req)
	case "Reply":
		return handleReply(c, myid, req)
	case "JoinAndPost":
		return handleJoinAndPost(c, myid, req)
	case "Move":
		return handleMove(c, myid, req)
	case "BackToPending":
		return handleBackToPending(c, myid, req)
	default:
		return fiber.NewError(fiber.StatusBadRequest, "Unknown action")
	}
}

// handlePromise records a promise of an item to a user.
func handlePromise(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	// Verify message exists and is owned by the current user.
	var msgUserid uint64
	db.Raw("SELECT fromuser FROM messages WHERE id = ?", req.ID).Scan(&msgUserid)
	if msgUserid == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}
	if msgUserid != myid {
		return fiber.NewError(fiber.StatusForbidden, "Not your message")
	}

	promisedTo := myid
	if req.Userid != nil && *req.Userid > 0 {
		promisedTo = *req.Userid
	}

	// REPLACE INTO - idempotent.
	db.Exec("REPLACE INTO messages_promises (msgid, userid) VALUES (?, ?)", req.ID, promisedTo)

	// Create a chat message of type Promised if promising to another user.
	if req.Userid != nil && *req.Userid > 0 && *req.Userid != myid {
		createSystemChatMessage(db, myid, *req.Userid, req.ID, utils.CHAT_MESSAGE_PROMISED)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleRenege removes a promise and records reliability data.
func handleRenege(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	var msgUserid uint64
	db.Raw("SELECT fromuser FROM messages WHERE id = ?", req.ID).Scan(&msgUserid)
	if msgUserid == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}
	if msgUserid != myid {
		return fiber.NewError(fiber.StatusForbidden, "Not your message")
	}

	promisedTo := myid
	if req.Userid != nil && *req.Userid > 0 {
		promisedTo = *req.Userid
	}

	// Record renege for reliability tracking (only if not reneging on self).
	if promisedTo != myid {
		db.Exec("INSERT INTO messages_reneged (userid, msgid) VALUES (?, ?)", promisedTo, req.ID)
	}

	// Delete the promise.
	db.Exec("DELETE FROM messages_promises WHERE msgid = ? AND userid = ?", req.ID, promisedTo)

	// Create a chat message of type Reneged if reneging on another user.
	if req.Userid != nil && *req.Userid > 0 && *req.Userid != myid {
		createSystemChatMessage(db, myid, *req.Userid, req.ID, utils.CHAT_MESSAGE_RENEGED)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleOutcomeIntended records an intended outcome.
func handleOutcomeIntended(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if req.Outcome == "" {
		return fiber.NewError(fiber.StatusBadRequest, "outcome is required")
	}

	// Verify valid outcome.
	if req.Outcome != utils.OUTCOME_TAKEN && req.Outcome != utils.OUTCOME_RECEIVED && req.Outcome != utils.OUTCOME_WITHDRAWN {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid outcome")
	}

	// Verify caller owns the message or is a moderator (matching PHP canmod check).
	if !canModifyMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not allowed to modify this message")
	}

	// Simple insert-or-update.
	db.Exec("INSERT INTO messages_outcomes_intended (msgid, outcome) VALUES (?, ?) ON DUPLICATE KEY UPDATE outcome = VALUES(outcome)",
		req.ID, req.Outcome)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleOutcome marks a message with an outcome (Taken, Received, Withdrawn).
// This has complex async side effects that PHP handles via background jobs.
// We record the outcome in the DB and queue background processing.
func handleOutcome(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if req.Outcome == "" {
		return fiber.NewError(fiber.StatusBadRequest, "outcome is required")
	}

	if req.Outcome != utils.OUTCOME_TAKEN && req.Outcome != utils.OUTCOME_RECEIVED && req.Outcome != utils.OUTCOME_WITHDRAWN {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid outcome")
	}

	// Get message type and verify existence.
	var msgType string
	db.Raw("SELECT type FROM messages WHERE id = ?", req.ID).Scan(&msgType)
	if msgType == "" {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}

	// Verify caller owns the message or is a moderator (matching PHP canmod check).
	if !canModifyMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not allowed to modify this message")
	}

	// Validate outcome against message type (Taken only on Offer, Received only on Wanted).
	if req.Outcome == utils.OUTCOME_TAKEN && msgType != "Offer" {
		return fiber.NewError(fiber.StatusBadRequest, "Taken outcome only valid for Offer messages")
	}
	if req.Outcome == utils.OUTCOME_RECEIVED && msgType != "Wanted" {
		return fiber.NewError(fiber.StatusBadRequest, "Received outcome only valid for Wanted messages")
	}

	// For Withdrawn: if the message is still pending on any group, delete it entirely
	// instead of recording an outcome (matching PHP behaviour).
	if req.Outcome == utils.OUTCOME_WITHDRAWN {
		var pendingCount int64
		db.Raw("SELECT COUNT(*) FROM messages_groups WHERE msgid = ? AND collection = 'Pending'", req.ID).Scan(&pendingCount)
		if pendingCount > 0 {
			db.Exec("DELETE FROM messages WHERE id = ?", req.ID)
			return c.JSON(fiber.Map{"ret": 0, "status": "Success", "deleted": true})
		}
	}

	// Check for existing outcome (prevent duplicates unless expired).
	var existingOutcome string
	db.Raw("SELECT outcome FROM messages_outcomes WHERE msgid = ?", req.ID).Scan(&existingOutcome)
	if existingOutcome != "" && existingOutcome != utils.OUTCOME_EXPIRED {
		return fiber.NewError(fiber.StatusConflict, "Outcome already recorded")
	}

	// Clear any intended outcome.
	db.Exec("DELETE FROM messages_outcomes_intended WHERE msgid = ?", req.ID)

	// Clear any existing outcome (for expired overwrite).
	db.Exec("DELETE FROM messages_outcomes WHERE msgid = ?", req.ID)

	// Record the outcome.
	happiness := ""
	if req.Happiness != nil {
		happiness = *req.Happiness
	}
	comment := ""
	if req.Comment != nil {
		comment = *req.Comment
	}

	if happiness != "" {
		db.Exec("INSERT INTO messages_outcomes (msgid, outcome, happiness, comments) VALUES (?, ?, ?, ?)",
			req.ID, req.Outcome, happiness, comment)
	} else {
		db.Exec("INSERT INTO messages_outcomes (msgid, outcome, comments) VALUES (?, ?, ?)",
			req.ID, req.Outcome, comment)
	}

	// Record who took/received the item (matching PHP Message::mark()).
	if (req.Outcome == utils.OUTCOME_TAKEN || req.Outcome == utils.OUTCOME_RECEIVED) && req.Userid != nil && *req.Userid > 0 {
		var availNow int
		db.Raw("SELECT availablenow FROM messages WHERE id = ?", req.ID).Scan(&availNow)
		db.Exec("INSERT INTO messages_by (msgid, userid, count) VALUES (?, ?, ?)",
			req.ID, *req.Userid, availNow)
	}

	// Queue background processing for notifications/chat messages.
	// PHP's backgroundMark() handles: logging, chat notifications to interested users,
	// marking chats as up-to-date.
	messageForOthers := ""
	if req.Message != nil {
		messageForOthers = *req.Message
	}
	userid := uint64(0)
	if req.Userid != nil {
		userid = *req.Userid
	}

	db.Exec("INSERT INTO background_tasks (task_type, data) VALUES (?, JSON_OBJECT('msgid', ?, 'outcome', ?, 'happiness', ?, 'comment', ?, 'userid', ?, 'byuser', ?, 'message', ?))",
		"message_outcome", req.ID, req.Outcome, happiness, comment, userid, myid, messageForOthers)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// canModifyMessage checks if the user is the message poster or a moderator/owner of a group the message is on.
func canModifyMessage(db *gorm.DB, myid uint64, msgid uint64) bool {
	var msgUserid uint64
	db.Raw("SELECT fromuser FROM messages WHERE id = ?", msgid).Scan(&msgUserid)
	if msgUserid == myid {
		return true
	}

	// Check if user is a moderator/owner of any group the message is on.
	var modCount int64
	db.Raw("SELECT COUNT(*) FROM messages_groups mg JOIN memberships m ON mg.groupid = m.groupid WHERE mg.msgid = ? AND m.userid = ? AND m.role IN ('Moderator', 'Owner')",
		msgid, myid).Scan(&modCount)
	return modCount > 0
}

// handleAddBy records who is taking items from a message.
func handleAddBy(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if !canModifyMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not allowed to modify this message")
	}

	count := 1
	if req.Count != nil {
		count = *req.Count
	}

	userid := uint64(0)
	if req.Userid != nil {
		userid = *req.Userid
	}

	// Check if this user already has an entry.
	type byEntry struct {
		ID    uint64
		Count int
	}
	var existing byEntry
	db.Raw("SELECT id, count FROM messages_by WHERE msgid = ? AND userid = ?",
		req.ID, userid).Scan(&existing)
	existingID := existing.ID
	existingCount := existing.Count

	if existingID > 0 {
		// Restore old count before updating.
		db.Exec("UPDATE messages SET availablenow = LEAST(availableinitially, availablenow + ?) WHERE id = ?",
			existingCount, req.ID)
		db.Exec("UPDATE messages_by SET count = ? WHERE id = ?", count, existingID)
	} else {
		db.Exec("INSERT INTO messages_by (userid, msgid, count) VALUES (?, ?, ?)",
			userid, req.ID, count)
	}

	// Reduce available count.
	db.Exec("UPDATE messages SET availablenow = GREATEST(LEAST(availableinitially, availablenow - ?), 0) WHERE id = ?",
		count, req.ID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleRemoveBy removes a taker and restores available count.
func handleRemoveBy(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if !canModifyMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not allowed to modify this message")
	}

	userid := uint64(0)
	if req.Userid != nil {
		userid = *req.Userid
	}

	// Find the entry.
	type byEntry struct {
		ID    uint64
		Count int
	}
	var entry byEntry
	db.Raw("SELECT id, count FROM messages_by WHERE msgid = ? AND userid = ?",
		req.ID, userid).Scan(&entry)
	entryID := entry.ID
	entryCount := entry.Count

	if entryID > 0 {
		// Restore count and delete entry.
		db.Exec("UPDATE messages SET availablenow = LEAST(availableinitially, availablenow + ?) WHERE id = ?",
			entryCount, req.ID)
		db.Exec("DELETE FROM messages_by WHERE id = ?", entryID)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleView records a message view, de-duplicating within 30 minutes.
func handleView(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	// Check for a recent view within 30 minutes to avoid redundant writes.
	var recentCount int64
	db.Raw("SELECT COUNT(*) FROM messages_likes WHERE msgid = ? AND userid = ? AND type = 'View' AND timestamp >= DATE_SUB(NOW(), INTERVAL 30 MINUTE)",
		req.ID, myid).Scan(&recentCount)

	if recentCount == 0 {
		db.Exec("INSERT INTO messages_likes (msgid, userid, type) VALUES (?, ?, 'View') ON DUPLICATE KEY UPDATE timestamp = NOW(), count = count + 1",
			req.ID, myid)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// createSystemChatMessage creates a system chat message between two users for a message.
// If no chat room exists between the users, one is created (matching PHP ChatRoom::createConversation).
func createSystemChatMessage(db *gorm.DB, fromUser uint64, toUser uint64, refmsgid uint64, msgType string) {
	// Find existing chat room between these users.
	var chatID uint64
	db.Raw("SELECT id FROM chat_rooms WHERE (user1 = ? AND user2 = ?) OR (user1 = ? AND user2 = ?) LIMIT 1",
		fromUser, toUser, toUser, fromUser).Scan(&chatID)

	if chatID == 0 {
		// Create a User2User chat room. ON DUPLICATE KEY handles race conditions
		// (unique key on user1, user2, chattype).
		// Use raw database/sql to get LastInsertId() from the same result —
		// avoids the GORM connection-pool race.
		sqlDB, err := db.DB()
		if err != nil {
			return
		}
		sqlResult, err := sqlDB.Exec("INSERT INTO chat_rooms (user1, user2, chattype, latestmessage) VALUES (?, ?, 'User2User', NOW()) ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id), latestmessage = NOW()",
			fromUser, toUser)
		if err != nil {
			return
		}
		chatIDInt, err := sqlResult.LastInsertId()
		if err != nil || chatIDInt == 0 {
			return
		}
		chatID = uint64(chatIDInt)
	}

	// Insert chat message.
	db.Exec("INSERT INTO chat_messages (chatid, userid, type, refmsgid, date, message, processingrequired) VALUES (?, ?, ?, ?, ?, '', 1)",
		chatID, fromUser, msgType, refmsgid, time.Now())
}

// handleMove moves a message from its current group to a different group.
// The user must be a moderator/owner of both the source and target groups.
// The message is placed into Pending collection on the target group.
func handleMove(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if req.Groupid == nil || *req.Groupid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "groupid is required")
	}

	// Must be mod of the source group (i.e. a group the message is currently on).
	if !isModForMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	// Must also be mod of the target group.
	if !user.IsModOfGroup(myid, *req.Groupid) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator on the target group")
	}

	// Use a transaction to ensure DELETE + INSERT are atomic (matching V1 Message::move).
	// Without this, a failure after DELETE would orphan the message.
	err := db.Transaction(func(tx *gorm.DB) error {
		result := tx.Exec("DELETE FROM messages_groups WHERE msgid = ?", req.ID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("message not found in any group")
		}

		result = tx.Exec("INSERT INTO messages_groups (msgid, groupid, collection, arrival, msgtype) VALUES (?, ?, 'Pending', NOW(), (SELECT type FROM messages WHERE id = ?))",
			req.ID, *req.Groupid, req.ID)
		return result.Error
	})

	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to move message: "+err.Error())
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
