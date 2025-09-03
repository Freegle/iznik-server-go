package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/group"
	"github.com/freegle/iznik-server-go/message"
	user2 "github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"io"
	"net/http"
	"strings"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func rsp(response *http.Response) []byte {
	buf := new(strings.Builder)
	io.Copy(buf, response.Body)
	return []byte(buf.String())
}

func GetToken(id uint64, sessionid uint64) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":        fmt.Sprint(id),
		"sessionid": fmt.Sprint(sessionid),
		"exp":       time.Now().Unix() + 30*60,
	})

	// Sign and get the complete encoded token as a string using the secret
	tokenString, _ := token.SignedString([]byte(os.Getenv("JWT_SECRET")))

	return tokenString
}

func GetUserWithToken(t *testing.T, systemrole ...string) (user2.User, string) {
	db := database.DBConn

	// Default to "User" if no systemrole specified
	role := "User"
	if len(systemrole) > 0 {
		role = systemrole[0]
	}

	var ids []uint64

	if role == "Support" || role == "Admin" {
		// For Support/Admin users, we don't need all the complex JOINs
		// Just find any user with the required role
		db.Raw("SELECT id FROM users WHERE systemrole = ? AND deleted IS NULL LIMIT 1", role).Pluck("id", &ids)
	} else {
		// For regular users, find a user with:
		// - an isochrone
		// - an address
		// - a user chat
		// - a mod chat
		// - a group membership
		// - a volunteer opportunity
		//
		// This should have been set up in testenv.php.
		start := time.Now().AddDate(0, 0, -utils.CHAT_ACTIVE_LIMIT).Format("2006-01-02")

		db.Raw("SELECT users.id FROM users "+
			"INNER JOIN isochrones_users ON isochrones_users.userid = users.id "+
			"INNER JOIN chat_messages ON chat_messages.userid = users.id AND chat_messages.message IS NOT NULL "+
			"INNER JOIN chat_rooms c1 ON c1.user1 = users.id AND c1.chattype = ? AND c1.latestmessage > ? "+
			"INNER JOIN chat_rooms c2 ON c2.user1 = users.id AND c2.chattype = ? AND c2.latestmessage > ? "+
			"INNER JOIN users_addresses ON users_addresses.userid = users.id "+
			"INNER JOIN memberships ON memberships.userid = users.id "+
			"INNER JOIN volunteering_groups ON volunteering_groups.groupid = memberships.groupid "+
			"INNER JOIN communityevents_groups ON communityevents_groups.groupid = memberships.groupid "+
			"WHERE users.systemrole = ? "+
			"LIMIT 1", utils.CHAT_TYPE_USER2USER, start, utils.CHAT_TYPE_USER2MOD, start, role).Pluck("id", &ids)
	}

	user := user2.GetUserById(ids[0], 0)

	token := getToken(t, user.ID)

	return user, token
}

func getToken(t *testing.T, userid uint64) string {
	// Get their JWT. This matches the PHP code.  We need to insert a fake session and retrieve the id.
	db := database.DBConn
	assert.Greater(t, userid, uint64(0))
	var sessionid uint64
	db.Exec("INSERT INTO sessions (userid, series, token, date, lastactive)  VALUES (?, ?, 1, NOW(), NOW())", userid, userid)
	db.Raw("SELECT id FROM sessions WHERE userid = ?", userid).Scan(&sessionid)
	token := GetToken(userid, sessionid)
	assert.Greater(t, len(token), 0)
	return token
}

func GetPersistentToken() string {
	db := database.DBConn

	var t user2.PersistentToken

	db.Raw("SELECT id, series, token FROM sessions ORDER BY lastactive DESC LIMIT 1").Scan(&t)

	enc, _ := json2.Marshal(t)

	return string(enc)
}

func GetGroup(app *fiber.App, name string) group.GroupEntry {
	resp, _ := app.Test(httptest.NewRequest("GET", "/api/group", nil))

	var groups []group.GroupEntry
	json2.Unmarshal(rsp(resp), &groups)

	// Get the playground
	gix := 0

	for ix, g := range groups {
		if g.Nameshort == name {
			gix = ix
		}
	}

	return groups[gix]
}

func GetUserWithMessage(t *testing.T) uint64 {
	db := database.DBConn

	type users struct {
		Fromuser uint64
	}

	var u []users

	db.Raw("SELECT fromuser FROM messages_groups INNER JOIN messages ON messages.id = messages_groups.msgid WHERE fromuser IS NOT NULL AND fromuser > 0 ORDER BY messages.id DESC LIMIT 1").Scan(&u)

	return u[0].Fromuser
}

func GetMessage(t *testing.T) message.Message {
	db := database.DBConn

	var mids []uint64

	db.Raw("SELECT msgid FROM messages_spatial INNER JOIN messages ON messages.id = messages_spatial.msgid WHERE LOCATE(' ', subject) ORDER BY msgid DESC LIMIT 1").Pluck("msgid", &mids)

	// Convert mids to strings
	var smids []string
	for _, mid := range mids {
		smids = append(smids, fmt.Sprintf("%d", mid))
	}

	messages := message.GetMessagesByIds(0, smids)
	return messages[0]
}

func GetChatFromModToGroup(t *testing.T) (uint64, uint64, string) {
	db := database.DBConn

	type chats struct {
		Userid uint64
		Chatid uint64
	}

	var c []chats

	// Get a chat from a mod to a group where the mod is still a member of the group.
	db.Raw("SELECT memberships.userid, chatid FROM chat_messages "+
		"INNER JOIN chat_rooms ON chat_rooms.id = chat_messages.chatid "+
		"INNER JOIN users ON chat_messages.userid = users.id "+
		"INNER JOIN memberships ON memberships.userid = users.id AND memberships.groupid = chat_rooms.groupid "+
		"WHERE users.systemrole != 'User' AND chat_rooms.chattype = ? AND chat_rooms.user1 = users.id "+
		"ORDER BY userid DESC LIMIT 1;", utils.CHAT_TYPE_USER2MOD).Scan(&c)

	token := getToken(t, c[0].Userid)
	return c[0].Chatid, c[0].Userid, token
}

// SetupTestEnvironment supplements the existing testenv.php data with any missing test data
// NOTE: All this test data code is generated by Claude and will in time be replaced with real API calls,
// as we expand the v2 API.
func SetupTestEnvironment() error {
	db := database.DBConn
	
	// Clean up any previous Go test additions while preserving testenv.php data
	db.Exec("DELETE FROM newsfeed WHERE message LIKE '%Go tests%'")
	
	// Check if testenv.php has run by looking for characteristic data
	var playgroundExists uint64
	db.Raw("SELECT COUNT(*) FROM `groups` WHERE nameshort = 'FreeglePlayground'").Scan(&playgroundExists)
	
	if playgroundExists == 0 {
		// testenv.php hasn't run - we need to create basic test data
		// For now, just ensure we don't break anything
		return nil
	}
	
	// testenv.php has run - supplement with missing newsfeed entries
	// Get Test users from testenv.php that have the required relationships for newsfeed
	var testUsers []uint64
	db.Raw("SELECT DISTINCT users.id FROM users WHERE users.firstname = 'Test' AND users.systemrole = 'User' LIMIT 3").Pluck("id", &testUsers)
	
	// Create newsfeed entries for testenv.php users (matching the testenv.php pattern)
	for i, userID := range testUsers {
		var existingNewsfeed uint64
		message := fmt.Sprintf("This is a test post from Go tests %d", i+1)
		db.Raw("SELECT id FROM newsfeed WHERE message = ? AND userid = ? LIMIT 1", message, userID).Scan(&existingNewsfeed)
		
		if existingNewsfeed == 0 {
			// Create primary newsfeed entry (matches testenv.php: TYPE_MESSAGE)
			db.Exec(`INSERT INTO newsfeed (userid, message, type, timestamp, deleted, reviewrequired, replyto, position, hidden, pinned) 
				VALUES (?, ?, 'Message', NOW(), NULL, 0, NULL, ST_GeomFromText('POINT(-3.1883 55.9533)', 3857), NULL, 0)`, userID, message)
			
			// Get the ID of the first post for the reply
			var newsfeedID uint64
			db.Raw("SELECT id FROM newsfeed WHERE message = ? AND userid = ? ORDER BY id DESC LIMIT 1", message, userID).Scan(&newsfeedID)
			
			// Create a reply newsfeed entry (matches testenv.php pattern)
			if newsfeedID > 0 {
				replyMessage := fmt.Sprintf("This is a test reply from Go tests %d", i+1)
				db.Exec(`INSERT INTO newsfeed (userid, message, type, timestamp, deleted, reviewrequired, replyto, position, hidden, pinned) 
					VALUES (?, ?, 'Message', DATE_SUB(NOW(), INTERVAL 30 MINUTE), NULL, 0, ?, ST_GeomFromText('POINT(-3.1883 55.9533)', 3857), NULL, 0)`, 
					userID, replyMessage, newsfeedID)
			}
		}
	}
	
	// The rest of the comprehensive setup is no longer needed since testenv.php provides the foundation
	return nil
}

// Legacy comprehensive setup - keeping for reference but not using
func setupComprehensiveTestDataLegacy() {
	db := database.DBConn
	
	// Create test users with all required relationships - both Users and Moderators
	for i := 1; i <= 3; i++ {
		// Create regular user with location reference and JSON settings
		var locationID uint64
		db.Raw("SELECT id FROM locations LIMIT 1").Scan(&locationID)
		
		settings := `{"mylocation": {"lat": 55.9533, "lng": -3.1883}}`
		db.Exec(`INSERT INTO users (firstname, lastname, fullname, systemrole, lastlocation, settings) 
			VALUES (?, ?, ?, 'User', ?, ?)`,
			"GoTestUser",
			fmt.Sprintf("TestLastName%d", i), 
			fmt.Sprintf("GoTestUser TestLastName%d", i),
			locationID, settings)
		
		// Create moderator user with location reference and JSON settings
		db.Exec(`INSERT INTO users (firstname, lastname, fullname, systemrole, lastlocation, settings) 
			VALUES (?, ?, ?, 'Moderator', ?, ?)`,
			"GoTestModerator",
			fmt.Sprintf("ModLastName%d", i), 
			fmt.Sprintf("GoTestModerator ModLastName%d", i),
			locationID, settings)
		
		// Create support user with location reference and JSON settings
		db.Exec(`INSERT INTO users (firstname, lastname, fullname, systemrole, lastlocation, settings) 
			VALUES (?, ?, ?, 'Support', ?, ?)`,
			"GoTestSupport",
			fmt.Sprintf("SuppLastName%d", i), 
			fmt.Sprintf("GoTestSupport SuppLastName%d", i),
			locationID, settings)
		
		var userID, modID, suppID uint64
		db.Raw("SELECT id FROM users WHERE firstname = 'GoTestUser' AND lastname = ? ORDER BY id DESC LIMIT 1", 
			fmt.Sprintf("TestLastName%d", i)).Scan(&userID)
		db.Raw("SELECT id FROM users WHERE firstname = 'GoTestModerator' AND lastname = ? ORDER BY id DESC LIMIT 1", 
			fmt.Sprintf("ModLastName%d", i)).Scan(&modID)
		db.Raw("SELECT id FROM users WHERE firstname = 'GoTestSupport' AND lastname = ? ORDER BY id DESC LIMIT 1", 
			fmt.Sprintf("SuppLastName%d", i)).Scan(&suppID)
		
		if userID == 0 {
			continue // Skip if user creation failed
		}
		
		// Create group for this user (using backticks for reserved word and required polyindex)
		groupName := fmt.Sprintf("GoTestGroup%d", i)
		var groupID uint64
		db.Raw("SELECT id FROM `groups` WHERE nameshort = ?", groupName).Scan(&groupID)
		
		if groupID == 0 {
			// Group doesn't exist, create it
			db.Exec("INSERT INTO `groups` (nameshort, namefull, type, onhere, polyindex) VALUES (?, ?, 'Freegle', 1, ST_GeomFromText('POINT(0 0)', 3857))", 
				groupName, fmt.Sprintf("Go Test Group %d", i))
			db.Raw("SELECT id FROM `groups` WHERE nameshort = ?", groupName).Scan(&groupID)
		}
		
		if groupID == 0 {
			continue // Skip if group creation still failed
		}
		
		// Create memberships for both user and moderator (check if not exists)
		var existingMembership uint64
		db.Raw("SELECT id FROM memberships WHERE userid = ? AND groupid = ?", userID, groupID).Scan(&existingMembership)
		if existingMembership == 0 {
			db.Exec(`INSERT INTO memberships (userid, groupid, role) VALUES (?, ?, 'Member')`, userID, groupID)
		}
		
		if modID > 0 {
			var existingModMembership uint64
			db.Raw("SELECT id FROM memberships WHERE userid = ? AND groupid = ?", modID, groupID).Scan(&existingModMembership)
			if existingModMembership == 0 {
				db.Exec(`INSERT INTO memberships (userid, groupid, role) VALUES (?, ?, 'Moderator')`, modID, groupID)
			}
		}
		
		if suppID > 0 {
			var existingSuppMembership uint64
			db.Raw("SELECT id FROM memberships WHERE userid = ? AND groupid = ?", suppID, groupID).Scan(&existingSuppMembership)
			if existingSuppMembership == 0 {
				db.Exec(`INSERT INTO memberships (userid, groupid, role) VALUES (?, ?, 'Support')`, suppID, groupID)
			}
		}
		
		// Create user address (use existing paf_addresses ID with correct column name)
		var pafID uint64
		db.Raw("SELECT id FROM paf_addresses LIMIT 1").Scan(&pafID)
		if pafID > 0 {
			db.Exec(`INSERT INTO users_addresses (userid, pafid) VALUES (?, ?)`, userID, pafID)
			if modID > 0 {
				db.Exec(`INSERT INTO users_addresses (userid, pafid) VALUES (?, ?)`, modID, pafID)
			}
			if suppID > 0 {
				db.Exec(`INSERT INTO users_addresses (userid, pafid) VALUES (?, ?)`, suppID, pafID)
			}
		}
		
		// Create isochrone entry (use correct column name)
		var isochroneID uint64
		db.Raw("SELECT id FROM isochrones LIMIT 1").Scan(&isochroneID)
		if isochroneID > 0 {
			db.Exec(`INSERT INTO isochrones_users (userid, isochroneid) VALUES (?, ?)`, userID, isochroneID)
			if modID > 0 {
				db.Exec(`INSERT INTO isochrones_users (userid, isochroneid) VALUES (?, ?)`, modID, isochroneID)
			}
			if suppID > 0 {
				db.Exec(`INSERT INTO isochrones_users (userid, isochroneid) VALUES (?, ?)`, suppID, isochroneID)
			}
		}
		
		// Create volunteering opportunity - create actual volunteering record
		db.Exec(`INSERT INTO volunteering (title, description, contactname, contactemail, contactphone) 
			VALUES (?, 'Test volunteering opportunity', 'Test Contact', 'test@example.com', '123456789')`, 
			fmt.Sprintf("Go Test Volunteer %d", i))
		
		var volunteeringID uint64
		db.Raw("SELECT id FROM volunteering ORDER BY id DESC LIMIT 1").Scan(&volunteeringID)
		if volunteeringID > 0 {
			// Link volunteering to our group
			db.Exec(`INSERT INTO volunteering_groups (volunteeringid, groupid) VALUES (?, ?)`, volunteeringID, groupID)
		}
		
		// Create community event - create actual event record
		db.Exec(`INSERT INTO communityevents (title, description, contactname, contactemail, contactphone) 
			VALUES (?, 'Test community event', 'Test Contact', 'test@example.com', '123456789')`, 
			fmt.Sprintf("Go Test Event %d", i))
		
		var eventID uint64
		db.Raw("SELECT id FROM communityevents ORDER BY id DESC LIMIT 1").Scan(&eventID)
		if eventID > 0 {
			// Link event to our group
			db.Exec(`INSERT INTO communityevents_groups (eventid, groupid) VALUES (?, ?)`, eventID, groupID)
		}
		
		// Create user-to-user chat room - ensure we have a valid user2
		// First, create another user to chat with if needed
		var existingUser2 uint64
		db.Raw("SELECT id FROM users WHERE id != ? AND firstname != 'GoTestUser' LIMIT 1", userID).Scan(&existingUser2)
		if existingUser2 == 0 {
			// Create a simple user to chat with
			db.Exec(`INSERT INTO users (firstname, lastname, fullname, systemrole) VALUES ('ChatPartner', 'User', 'ChatPartner User', 'User')`)
			db.Raw("SELECT id FROM users WHERE firstname = 'ChatPartner' ORDER BY id DESC LIMIT 1").Scan(&existingUser2)
		}
		
		if existingUser2 > 0 {
			db.Exec(`INSERT INTO chat_rooms (user1, user2, chattype, latestmessage) 
				VALUES (?, ?, ?, NOW())`, userID, existingUser2, utils.CHAT_TYPE_USER2USER)
		}
		
		var chatRoomID uint64
		db.Raw("SELECT id FROM chat_rooms WHERE user1 = ? AND chattype = ? ORDER BY id DESC LIMIT 1", 
			userID, utils.CHAT_TYPE_USER2USER).Scan(&chatRoomID)
		
		if chatRoomID > 0 {
			// Create chat message for user-to-user chat
			db.Exec(`INSERT INTO chat_messages (chatid, userid, message) VALUES (?, ?, 'Test user chat message')`, 
				chatRoomID, userID)
		}
		
		// Create user-to-mod chat room  
		db.Exec(`INSERT INTO chat_rooms (user1, groupid, chattype, latestmessage) 
			VALUES (?, ?, ?, NOW())`, userID, groupID, utils.CHAT_TYPE_USER2MOD)
		
		db.Raw("SELECT id FROM chat_rooms WHERE user1 = ? AND chattype = ? ORDER BY id DESC LIMIT 1", 
			userID, utils.CHAT_TYPE_USER2MOD).Scan(&chatRoomID)
		
		if chatRoomID > 0 {
			// Create chat message for user-to-mod chat
			db.Exec(`INSERT INTO chat_messages (chatid, userid, message) VALUES (?, ?, 'Test mod chat message')`, 
				chatRoomID, userID)
		}
		
		// Create additional User2Mod chat room where moderator is user1 (needed for GetChatFromModToGroup)
		if modID > 0 {
			db.Exec(`INSERT INTO chat_rooms (user1, groupid, chattype, latestmessage) 
				VALUES (?, ?, ?, NOW())`, modID, groupID, utils.CHAT_TYPE_USER2MOD)
			
			var modChatRoomID uint64
			db.Raw("SELECT id FROM chat_rooms WHERE user1 = ? AND chattype = ? ORDER BY id DESC LIMIT 1", 
				modID, utils.CHAT_TYPE_USER2MOD).Scan(&modChatRoomID)
			
			if modChatRoomID > 0 {
				// Create chat message from moderator
				db.Exec(`INSERT INTO chat_messages (chatid, userid, message) VALUES (?, ?, 'Test moderator message')`, 
					modChatRoomID, modID)
			}
		}
		
		// Create test messages with spaces in subject (needed for GetMessage function)
		if locationID > 0 {
			var existingMessage uint64
			messageSubject := fmt.Sprintf("Test Message %d", i)
			db.Raw("SELECT id FROM messages WHERE subject = ?", messageSubject).Scan(&existingMessage)
			
			if existingMessage == 0 {
				// Create message with correct column names (message not textbody, no successful column)
				db.Exec(`INSERT INTO messages (fromuser, subject, message, type, locationid) 
					VALUES (?, ?, 'Test message body', 'Offer', ?)`, 
					userID, messageSubject, locationID)
				
				// Get the message ID and add to messages_spatial and messages_groups
				var messageID uint64
				db.Raw("SELECT id FROM messages WHERE subject = ? ORDER BY id DESC LIMIT 1", messageSubject).Scan(&messageID)
				if messageID > 0 {
					// Add to messages_spatial with required point geometry (SRID 3857)
					db.Exec(`INSERT INTO messages_spatial (msgid, point, successful, groupid, arrival, msgtype) VALUES (?, ST_GeomFromText('POINT(-3.1883 55.9533)', 3857), 1, ?, NOW(), 'Offer')`, 
						messageID, groupID)
					
					// Add to messages_groups to create the message-group relationship needed for MessageGroups
					var existingMessageGroup uint64
					db.Raw("SELECT msgid FROM messages_groups WHERE msgid = ? AND groupid = ?", messageID, groupID).Scan(&existingMessageGroup)
					if existingMessageGroup == 0 {
						db.Exec(`INSERT INTO messages_groups (msgid, groupid, arrival, collection, autoreposts) VALUES (?, ?, NOW(), 'Approved', 0)`, 
							messageID, groupID)
					}
					
					// Also add this message to the first existing group (FreeglePlayground) for TestMessages
					var playgroundGroupID uint64 = 567905
					var existingPlaygroundMessage uint64
					db.Raw("SELECT msgid FROM messages_groups WHERE msgid = ? AND groupid = ?", messageID, playgroundGroupID).Scan(&existingPlaygroundMessage)
					if existingPlaygroundMessage == 0 {
						db.Exec(`INSERT INTO messages_groups (msgid, groupid, arrival, collection, autoreposts) VALUES (?, ?, NOW(), 'Approved', 0)`, 
							messageID, playgroundGroupID)
						db.Exec(`INSERT INTO messages_spatial (msgid, point, successful, groupid, arrival, msgtype) VALUES (?, ST_GeomFromText('POINT(-3.1883 55.9533)', 3857), 1, ?, NOW(), 'Offer')`, 
							messageID, playgroundGroupID)
					}
				}
			}
		}
	}
	
	// Create community events and dates for testing
	var existingEvent uint64
	db.Raw("SELECT id FROM communityevents WHERE title = 'GoTestEvent' LIMIT 1").Scan(&existingEvent)
	if existingEvent == 0 {
		// Get first test user and group created above
		var firstUserID, firstGroupID uint64
		db.Raw("SELECT id FROM users WHERE firstname = 'GoTestUser' ORDER BY id LIMIT 1").Scan(&firstUserID)
		db.Raw("SELECT id FROM `groups` WHERE nameshort LIKE 'GoTestGroup%' ORDER BY id LIMIT 1").Scan(&firstGroupID)
		
		if firstUserID > 0 && firstGroupID > 0 {
			// Create community event
			db.Exec(`INSERT INTO communityevents (userid, title, location, description, contactname, contactphone, contactemail, pending, deleted, heldby) 
				VALUES (?, 'GoTestEvent', 'Test Location', 'Test community event description', 'Test Contact', '01234567890', 'test@example.com', 0, 0, NULL)`, 
				firstUserID)
			
			// Get the event ID
			db.Raw("SELECT id FROM communityevents WHERE title = 'GoTestEvent' LIMIT 1").Scan(&existingEvent)
			
			if existingEvent > 0 {
				// Create community event date
				var existingEventDate uint64
				db.Raw("SELECT id FROM communityevents_dates WHERE eventid = ? LIMIT 1", existingEvent).Scan(&existingEventDate)
				if existingEventDate == 0 {
					db.Exec(`INSERT INTO communityevents_dates (eventid, start, end) VALUES (?, DATE_ADD(NOW(), INTERVAL 7 DAY), DATE_ADD(NOW(), INTERVAL 8 DAY))`, existingEvent)
				}
				
				// Create community event group association
				var existingEventGroup uint64
				db.Raw("SELECT eventid FROM communityevents_groups WHERE eventid = ? AND groupid = ? LIMIT 1", existingEvent, firstGroupID).Scan(&existingEventGroup)
				if existingEventGroup == 0 {
					db.Exec(`INSERT INTO communityevents_groups (eventid, groupid) VALUES (?, ?)`, existingEvent, firstGroupID)
				}
			}
		}
	}
	
	// Create newsfeed entries for all GoTestUser entries that have the necessary relationships
	// Based on testenv.php pattern, create TYPE_MESSAGE newsfeed entries for users with proper location data
	
	var testUserIDs []uint64
	db.Raw("SELECT DISTINCT users.id FROM users "+
		"INNER JOIN isochrones_users ON isochrones_users.userid = users.id "+
		"INNER JOIN chat_messages ON chat_messages.userid = users.id AND chat_messages.message IS NOT NULL "+
		"INNER JOIN users_addresses ON users_addresses.userid = users.id "+
		"INNER JOIN memberships ON memberships.userid = users.id "+
		"WHERE users.firstname = 'GoTestUser' AND users.systemrole = 'User' ORDER BY users.id").Pluck("id", &testUserIDs)
	
	for i, userID := range testUserIDs {
		// Create newsfeed entries matching testenv.php pattern: TYPE_MESSAGE with proper content
		var existingNewsfeed uint64
		message := fmt.Sprintf("This is a test post from Go tests %d", i+1)
		db.Raw("SELECT id FROM newsfeed WHERE message = ? AND userid = ? LIMIT 1", message, userID).Scan(&existingNewsfeed)
		
		if existingNewsfeed == 0 {
			// Create primary newsfeed entry (matches testenv.php: TYPE_MESSAGE)
			db.Exec(`INSERT INTO newsfeed (userid, message, type, timestamp, deleted, reviewrequired, replyto, position, hidden, pinned) 
				VALUES (?, ?, 'Message', NOW(), NULL, 0, NULL, ST_GeomFromText('POINT(-3.1883 55.9533)', 3857), NULL, 0)`, userID, message)
			
			// Get the ID of the first post for the reply
			var newsfeedID uint64
			db.Raw("SELECT id FROM newsfeed WHERE message = ? AND userid = ? ORDER BY id DESC LIMIT 1", message, userID).Scan(&newsfeedID)
			
			// Create a reply newsfeed entry (matches testenv.php pattern)
			if newsfeedID > 0 {
				replyMessage := fmt.Sprintf("This is a test reply from Go tests %d", i+1)
				db.Exec(`INSERT INTO newsfeed (userid, message, type, timestamp, deleted, reviewrequired, replyto, position, hidden, pinned) 
					VALUES (?, ?, 'Message', DATE_SUB(NOW(), INTERVAL 30 MINUTE), NULL, 0, ?, ST_GeomFromText('POINT(-3.1883 55.9533)', 3857), NULL, 0)`, 
					userID, replyMessage, newsfeedID)
			}
		}
	}
}
