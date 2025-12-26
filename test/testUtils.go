package test

import (
	json2 "encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/freegle/iznik-server-go/database"
	user2 "github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/golang-jwt/jwt/v4"
	"gorm.io/gorm"
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



// =============================================================================
// FACTORY FUNCTIONS - Create isolated test data for each test
// =============================================================================

// uniquePrefix generates a unique prefix for test data to avoid collisions
func uniquePrefix(testName string) string {
	return fmt.Sprintf("%s_%d", testName, time.Now().UnixNano())
}

// CreateTestGroup creates a new group for testing and returns its ID
func CreateTestGroup(t *testing.T, prefix string) uint64 {
	db := database.DBConn
	name := fmt.Sprintf("TestGroup_%s", prefix)


	result := db.Exec(fmt.Sprintf("INSERT INTO `groups` (nameshort, namefull, type, onhere, polyindex, lat, lng) "+
		"VALUES (?, ?, 'Freegle', 1, ST_GeomFromText('POINT(-3.1883 55.9533)', %d), 55.9533, -3.1883)", utils.SRID),
		name, "Test Group "+prefix)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create group: %v", result.Error)
	}

	var groupID uint64
	db.Raw("SELECT id FROM `groups` WHERE nameshort = ? ORDER BY id DESC LIMIT 1", name).Scan(&groupID)

	if groupID == 0 {
		t.Fatalf("ERROR: Group was created but ID not found for name=%s", name)
	}

	return groupID
}

// CreateTestUser creates a new user for testing and returns its ID
func CreateTestUser(t *testing.T, prefix string, role string) uint64 {
	db := database.DBConn
	email := fmt.Sprintf("%s@test.com", prefix)
	fullname := fmt.Sprintf("Test User %s", prefix)


	// Create user - use NULL for lastlocation to avoid foreign key issues
	settings := `{"mylocation": {"lat": 55.9533, "lng": -3.1883}}`
	result := db.Exec("INSERT INTO users (firstname, lastname, fullname, systemrole, lastlocation, settings) "+
		"VALUES ('Test', ?, ?, ?, NULL, ?)",
		prefix, fullname, role, settings)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create user: %v", result.Error)
	}

	var userID uint64
	db.Raw("SELECT id FROM users WHERE fullname = ? ORDER BY id DESC LIMIT 1", fullname).Scan(&userID)

	if userID == 0 {
		t.Fatalf("ERROR: User was created but ID not found for fullname=%s", fullname)
	}

	// Add email
	db.Exec("INSERT INTO users_emails (userid, email) VALUES (?, ?)", userID, email)

	return userID
}

// CreateDeletedTestUser creates a user that has been deleted (for TestDeleted)
func CreateDeletedTestUser(t *testing.T, prefix string) uint64 {
	db := database.DBConn
	fullname := fmt.Sprintf("Deleted User %s", prefix)


	result := db.Exec("INSERT INTO users (firstname, lastname, fullname, systemrole, deleted) "+
		"VALUES ('Deleted', ?, ?, 'User', NOW())",
		prefix, fullname)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create deleted user: %v", result.Error)
	}

	var userID uint64
	db.Raw("SELECT id FROM users WHERE fullname = ? ORDER BY id DESC LIMIT 1", fullname).Scan(&userID)

	if userID == 0 {
		t.Fatalf("ERROR: Deleted user was created but ID not found")
	}

	return userID
}

// CreateTestUserWithEmail creates a user with a specific email for testing
func CreateTestUserWithEmail(t *testing.T, prefix string, email string) uint64 {
	db := database.DBConn
	fullname := fmt.Sprintf("Test User %s", prefix)


	result := db.Exec("INSERT INTO users (firstname, lastname, fullname, systemrole) "+
		"VALUES ('Test', ?, ?, 'User')",
		prefix, fullname)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create user: %v", result.Error)
	}

	var userID uint64
	db.Raw("SELECT id FROM users WHERE fullname = ? ORDER BY id DESC LIMIT 1", fullname).Scan(&userID)

	if userID == 0 {
		t.Fatalf("ERROR: User was created but ID not found")
	}

	// Add email
	db.Exec("INSERT INTO users_emails (userid, email) VALUES (?, ?)", userID, email)

	return userID
}

// CreateTestMembership creates a membership linking a user to a group
func CreateTestMembership(t *testing.T, userID uint64, groupID uint64, role string) uint64 {
	db := database.DBConn


	result := db.Exec("INSERT INTO memberships (userid, groupid, role) VALUES (?, ?, ?)",
		userID, groupID, role)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create membership: %v", result.Error)
	}

	var membershipID uint64
	db.Raw("SELECT id FROM memberships WHERE userid = ? AND groupid = ? ORDER BY id DESC LIMIT 1",
		userID, groupID).Scan(&membershipID)

	return membershipID
}

// CreateTestSession creates a session for a user and returns (sessionID, token)
func CreateTestSession(t *testing.T, userID uint64) (uint64, string) {
	db := database.DBConn


	db.Exec("INSERT INTO sessions (userid, series, token, date, lastactive) VALUES (?, ?, 1, NOW(), NOW())",
		userID, userID)

	var sessionID uint64
	db.Raw("SELECT id FROM sessions WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&sessionID)

	if sessionID == 0 {
		t.Fatalf("ERROR: Session was created but ID not found for user=%d", userID)
	}

	token := GetToken(userID, sessionID)

	return sessionID, token
}

// CreatePersistentToken creates the old-style Authorization2 persistent token format
func CreatePersistentToken(t *testing.T, userID uint64, sessionID uint64) string {
	db := database.DBConn

	// Get session details matching what PHP uses
	// Note: The auth code expects PersistentToken.ID to be the SESSION ID, not user ID
	var series uint64
	var token string
	db.Raw("SELECT series, token FROM sessions WHERE id = ?", sessionID).Row().Scan(&series, &token)

	pt := user2.PersistentToken{
		ID:     sessionID, // Session ID, not user ID - this is what auth.go expects
		Series: series,
		Token:  token,
	}

	enc, _ := json2.Marshal(pt)
	return string(enc)
}

// getToken creates a session for a user and returns a JWT token
// This is a simple helper for tests that create their own users inline
func getToken(t *testing.T, userID uint64) string {
	db := database.DBConn
	var sessionID uint64
	db.Exec("INSERT INTO sessions (userid, series, token, date, lastactive) VALUES (?, ?, 1, NOW(), NOW())", userID, userID)
	db.Raw("SELECT id FROM sessions WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&sessionID)
	return GetToken(userID, sessionID)
}

// CreateTestJob creates a job at specified coordinates
func CreateTestJob(t *testing.T, lat float64, lng float64) uint64 {
	db := database.DBConn

	// Use string interpolation for geometry, not parameterized query
	// This matches PHP behavior and avoids GORM transforming the WKT
	result := db.Exec(fmt.Sprintf("INSERT INTO jobs (title, geometry, cpc, visible, category) "+
		"VALUES ('Test Job', ST_GeomFromText('POINT(%f %f)', %d), 0.10, 1, 'General')",
		lng, lat, utils.SRID))

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create job: %v", result.Error)
	}

	var jobID uint64
	db.Raw("SELECT id FROM jobs ORDER BY id DESC LIMIT 1").Scan(&jobID)

	if jobID == 0 {
		t.Fatalf("ERROR: Job was created but ID not found")
	}

	return jobID
}

// CreateTestAddress creates an address for a user
func CreateTestAddress(t *testing.T, userID uint64) uint64 {
	db := database.DBConn


	// Get an existing paf_addresses ID
	var pafID uint64
	db.Raw("SELECT id FROM paf_addresses LIMIT 1").Scan(&pafID)

	if pafID == 0 {
		t.Fatalf("ERROR: No paf_addresses found in database")
	}

	result := db.Exec("INSERT INTO users_addresses (userid, pafid) VALUES (?, ?)", userID, pafID)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create address: %v", result.Error)
	}

	var addressID uint64
	db.Raw("SELECT id FROM users_addresses WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&addressID)

	return addressID
}

// CreateTestIsochrone creates an isochrone entry for a user
func CreateTestIsochrone(t *testing.T, userID uint64, lat float64, lng float64) uint64 {
	db := database.DBConn


	// Create a test isochrone with a simple polygon around the test location
	// The polygon is a small square around the given coordinates
	polygon := fmt.Sprintf("POLYGON((%f %f, %f %f, %f %f, %f %f, %f %f))",
		lng-0.1, lat-0.1,
		lng+0.1, lat-0.1,
		lng+0.1, lat+0.1,
		lng-0.1, lat+0.1,
		lng-0.1, lat-0.1)

	result := db.Exec(fmt.Sprintf("INSERT INTO isochrones (transport, minutes, source, polygon) VALUES ('Walk', 30, 'ORS', ST_GeomFromText(?, %d))", utils.SRID), polygon)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create isochrone: %v", result.Error)
	}

	var isochroneID uint64
	db.Raw("SELECT id FROM isochrones ORDER BY id DESC LIMIT 1").Scan(&isochroneID)

	if isochroneID == 0 {
		t.Fatalf("ERROR: Isochrone was created but ID not found")
	}

	// Link user to isochrone
	result = db.Exec("INSERT INTO isochrones_users (userid, isochroneid) VALUES (?, ?)", userID, isochroneID)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create isochrone link: %v", result.Error)
	}

	return isochroneID
}

// CreateTestChatRoom creates a chat room and returns its ID
// chatType should be "User2User" or "User2Mod"
func CreateTestChatRoom(t *testing.T, user1ID uint64, user2ID *uint64, groupID *uint64, chatType string) uint64 {
	db := database.DBConn


	if chatType == "User2User" && user2ID != nil {
		result := db.Exec("INSERT INTO chat_rooms (user1, user2, chattype, latestmessage) VALUES (?, ?, ?, NOW())",
			user1ID, *user2ID, utils.CHAT_TYPE_USER2USER)
		if result.Error != nil {
			t.Fatalf("ERROR: Failed to create chat room: %v", result.Error)
		}
	} else if chatType == "User2Mod" && groupID != nil {
		result := db.Exec("INSERT INTO chat_rooms (user1, groupid, chattype, latestmessage) VALUES (?, ?, ?, NOW())",
			user1ID, *groupID, utils.CHAT_TYPE_USER2MOD)
		if result.Error != nil {
			t.Fatalf("ERROR: Failed to create chat room: %v", result.Error)
		}
	} else {
		t.Fatalf("ERROR: Invalid chat room configuration - User2User needs user2ID, User2Mod needs groupID")
	}

	var chatID uint64
	db.Raw("SELECT id FROM chat_rooms WHERE user1 = ? ORDER BY id DESC LIMIT 1", user1ID).Scan(&chatID)

	if chatID == 0 {
		t.Fatalf("ERROR: Chat room was created but ID not found")
	}

	return chatID
}

// CreateTestChatMessage creates a message in a chat room
func CreateTestChatMessage(t *testing.T, chatID uint64, userID uint64, message string) uint64 {
	db := database.DBConn


	result := db.Exec("INSERT INTO chat_messages (chatid, userid, message, date) VALUES (?, ?, ?, NOW())",
		chatID, userID, message)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create chat message: %v", result.Error)
	}

	var messageID uint64
	db.Raw("SELECT id FROM chat_messages WHERE chatid = ? AND userid = ? ORDER BY id DESC LIMIT 1",
		chatID, userID).Scan(&messageID)

	// Update chat room's latestmessage
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", chatID)

	return messageID
}

// CreateTestVolunteering creates a volunteering opportunity linked to a group
func CreateTestVolunteering(t *testing.T, userID uint64, groupID uint64) uint64 {
	db := database.DBConn


	result := db.Exec("INSERT INTO volunteering (userid, title, description, pending, deleted) "+
		"VALUES (?, 'Test Volunteering', 'Test volunteering opportunity', 0, 0)",
		userID)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create volunteering: %v", result.Error)
	}

	var volunteeringID uint64
	db.Raw("SELECT id FROM volunteering WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&volunteeringID)

	if volunteeringID == 0 {
		t.Fatalf("ERROR: Volunteering was created but ID not found")
	}

	// Link to group
	db.Exec("INSERT INTO volunteering_groups (volunteeringid, groupid) VALUES (?, ?)", volunteeringID, groupID)

	// Add dates
	db.Exec("INSERT INTO volunteering_dates (volunteeringid, start, end) "+
		"VALUES (?, DATE_ADD(NOW(), INTERVAL 7 DAY), DATE_ADD(NOW(), INTERVAL 14 DAY))", volunteeringID)

	return volunteeringID
}

// CreateTestCommunityEvent creates a community event linked to a group
func CreateTestCommunityEvent(t *testing.T, userID uint64, groupID uint64) uint64 {
	db := database.DBConn


	result := db.Exec("INSERT INTO communityevents (userid, title, description, pending, deleted) "+
		"VALUES (?, 'Test Event', 'Test community event', 0, 0)",
		userID)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create community event: %v", result.Error)
	}

	var eventID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&eventID)

	if eventID == 0 {
		t.Fatalf("ERROR: Community event was created but ID not found")
	}

	// Link to group
	db.Exec("INSERT INTO communityevents_groups (eventid, groupid) VALUES (?, ?)", eventID, groupID)

	// Add dates
	db.Exec("INSERT INTO communityevents_dates (eventid, start, end) "+
		"VALUES (?, DATE_ADD(NOW(), INTERVAL 7 DAY), DATE_ADD(NOW(), INTERVAL 8 DAY))", eventID)

	return eventID
}

// CreateTestMessage creates a message with spatial data and search index
func CreateTestMessage(t *testing.T, userID uint64, groupID uint64, subject string, lat float64, lng float64) uint64 {
	db := database.DBConn


	// Get a location ID
	var locationID uint64
	db.Raw("SELECT id FROM locations LIMIT 1").Scan(&locationID)

	result := db.Exec("INSERT INTO messages (fromuser, subject, textbody, type, locationid, arrival) "+
		"VALUES (?, ?, 'Test message body', 'Offer', ?, NOW())",
		userID, subject, locationID)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create message: %v", result.Error)
	}

	var messageID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? AND subject = ? ORDER BY id DESC LIMIT 1",
		userID, subject).Scan(&messageID)

	if messageID == 0 {
		t.Fatalf("ERROR: Message was created but ID not found")
	}

	// Add to messages_groups
	db.Exec("INSERT INTO messages_groups (msgid, groupid, arrival, collection, autoreposts) "+
		"VALUES (?, ?, NOW(), 'Approved', 0)", messageID, groupID)

	// Add to messages_spatial
	db.Exec(fmt.Sprintf("INSERT INTO messages_spatial (msgid, point, successful, groupid, arrival, msgtype) "+
		"VALUES (?, ST_GeomFromText(?, %d), 1, ?, NOW(), 'Offer')", utils.SRID),
		messageID, fmt.Sprintf("POINT(%f %f)", lng, lat), groupID)

	// Index words for search - extract words from subject and add to search index
	indexMessageWords(t, db, messageID, groupID, subject)

	return messageID
}

// indexMessageWords adds words from the subject to the search index
func indexMessageWords(t *testing.T, db *gorm.DB, messageID uint64, groupID uint64, subject string) {
	// Split subject into words and filter short/common words
	words := strings.Fields(strings.ToLower(subject))

	for _, word := range words {
		// Skip short words and common stop words
		if len(word) < 3 {
			continue
		}

		// Truncate to max 10 chars (words table limit)
		if len(word) > 10 {
			word = word[:10]
		}

		// Insert word if not exists
		firstThree := word
		if len(firstThree) > 3 {
			firstThree = firstThree[:3]
		}

		db.Exec("INSERT IGNORE INTO words (word, firstthree, soundex, popularity) VALUES (?, ?, SOUNDEX(?), 1)",
			word, firstThree, word)

		// Get the word ID
		var wordID uint64
		db.Raw("SELECT id FROM words WHERE word = ?", word).Scan(&wordID)

		if wordID > 0 {
			// Add to messages_index
			db.Exec("INSERT IGNORE INTO messages_index (msgid, wordid, arrival, groupid) VALUES (?, ?, UNIX_TIMESTAMP(), ?)",
				messageID, wordID, groupID)
		}
	}
}

// CreateTestNewsfeed creates a newsfeed entry for a user
func CreateTestNewsfeed(t *testing.T, userID uint64, lat float64, lng float64, message string) uint64 {
	db := database.DBConn


	result := db.Exec(fmt.Sprintf("INSERT INTO newsfeed (userid, message, type, timestamp, deleted, reviewrequired, position, hidden, pinned) "+
		"VALUES (?, ?, 'Message', NOW(), NULL, 0, ST_GeomFromText(?, %d), NULL, 0)", utils.SRID),
		userID, message, fmt.Sprintf("POINT(%f %f)", lng, lat))

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create newsfeed: %v", result.Error)
	}

	var newsfeedID uint64
	db.Raw("SELECT id FROM newsfeed WHERE userid = ? AND message = ? ORDER BY id DESC LIMIT 1",
		userID, message).Scan(&newsfeedID)

	if newsfeedID == 0 {
		t.Fatalf("ERROR: Newsfeed was created but ID not found")
	}

	return newsfeedID
}

// CreateTestItem creates an item in the items table
func CreateTestItem(t *testing.T, name string) uint64 {
	db := database.DBConn

	result := db.Exec("INSERT INTO items (name, popularity) VALUES (?, 1) ON DUPLICATE KEY UPDATE popularity = popularity + 1",
		name)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create item: %v", result.Error)
	}

	var itemID uint64
	db.Raw("SELECT id FROM items WHERE name = ?", name).Scan(&itemID)

	if itemID == 0 {
		t.Fatalf("ERROR: Item was created but ID not found for name=%s", name)
	}

	return itemID
}

// CreateTestMessageItem links a message to an item
func CreateTestMessageItem(t *testing.T, messageID uint64, itemID uint64) {
	db := database.DBConn

	result := db.Exec("INSERT INTO messages_items (msgid, itemid) VALUES (?, ?)", messageID, itemID)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create message_item link: %v", result.Error)
	}
}

// CreateTestAttachment creates an attachment for a message
func CreateTestAttachment(t *testing.T, messageID uint64) uint64 {
	db := database.DBConn

	result := db.Exec("INSERT INTO messages_attachments (msgid, `primary`) VALUES (?, 1)", messageID)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create attachment: %v", result.Error)
	}

	var attachmentID uint64
	db.Raw("SELECT id FROM messages_attachments WHERE msgid = ? ORDER BY id DESC LIMIT 1", messageID).Scan(&attachmentID)

	if attachmentID == 0 {
		t.Fatalf("ERROR: Attachment was created but ID not found")
	}

	return attachmentID
}

// CreateTestMessageWithArrival creates a message with a specific arrival date
func CreateTestMessageWithArrival(t *testing.T, userID uint64, groupID uint64, subject string, lat float64, lng float64, daysAgo int) uint64 {
	db := database.DBConn

	// Get a location ID
	var locationID uint64
	db.Raw("SELECT id FROM locations LIMIT 1").Scan(&locationID)

	result := db.Exec("INSERT INTO messages (fromuser, subject, textbody, type, locationid, arrival) "+
		"VALUES (?, ?, 'Test message body', 'Offer', ?, DATE_SUB(NOW(), INTERVAL ? DAY))",
		userID, subject, locationID, daysAgo)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create message: %v", result.Error)
	}

	var messageID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? AND subject = ? ORDER BY id DESC LIMIT 1",
		userID, subject).Scan(&messageID)

	if messageID == 0 {
		t.Fatalf("ERROR: Message was created but ID not found")
	}

	// Add to messages_groups with past arrival date
	db.Exec("INSERT INTO messages_groups (msgid, groupid, arrival, collection, autoreposts) "+
		"VALUES (?, ?, DATE_SUB(NOW(), INTERVAL ? DAY), 'Approved', 0)", messageID, groupID, daysAgo)

	// Add to messages_spatial
	db.Exec(fmt.Sprintf("INSERT INTO messages_spatial (msgid, point, successful, groupid, arrival, msgtype) "+
		"VALUES (?, ST_GeomFromText(?, %d), 1, ?, DATE_SUB(NOW(), INTERVAL ? DAY), 'Offer')", utils.SRID),
		messageID, fmt.Sprintf("POINT(%f %f)", lng, lat), groupID, daysAgo)

	return messageID
}

// MarkMessageAsViewed marks a message as viewed by a user (adds View like)
func MarkMessageAsViewed(t *testing.T, userID uint64, messageID uint64) {
	db := database.DBConn

	result := db.Exec("INSERT INTO messages_likes (msgid, userid, type) VALUES (?, ?, 'View') "+
		"ON DUPLICATE KEY UPDATE timestamp = NOW(), count = count + 1", messageID, userID)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to mark message as viewed: %v", result.Error)
	}
}

// CreateFullTestUser creates a user with all required relationships for complex tests
// Returns userID and JWT token
func CreateFullTestUser(t *testing.T, prefix string) (uint64, string) {
	// Create group first
	groupID := CreateTestGroup(t, prefix)

	// Create main user
	userID := CreateTestUser(t, prefix, "User")

	// Create another user for user-to-user chat
	otherUserID := CreateTestUser(t, prefix+"_other", "User")

	// Create membership
	CreateTestMembership(t, userID, groupID, "Member")

	// Create address and isochrone
	CreateTestAddress(t, userID)
	CreateTestIsochrone(t, userID, 55.9533, -3.1883)

	// Create User2User chat with message
	chatID := CreateTestChatRoom(t, userID, &otherUserID, nil, "User2User")
	CreateTestChatMessage(t, chatID, userID, "Test user message")

	// Create User2Mod chat with message
	modChatID := CreateTestChatRoom(t, userID, nil, &groupID, "User2Mod")
	CreateTestChatMessage(t, modChatID, userID, "Test mod message")

	// Create volunteering and community event
	CreateTestVolunteering(t, userID, groupID)
	CreateTestCommunityEvent(t, userID, groupID)

	// Create session and get token
	_, token := CreateTestSession(t, userID)

	return userID, token
}

// CreateTestNotification creates a notification for a user
// Returns the notification ID
func CreateTestNotification(t *testing.T, toUserID uint64, fromUserID uint64, notifType string) uint64 {
	db := database.DBConn

	result := db.Exec("INSERT INTO users_notifications (fromuser, touser, type, timestamp, seen) "+
		"VALUES (?, ?, ?, NOW(), 0)",
		fromUserID, toUserID, notifType)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create notification: %v", result.Error)
	}

	var notificationID uint64
	db.Raw("SELECT id FROM users_notifications WHERE touser = ? AND fromuser = ? ORDER BY id DESC LIMIT 1",
		toUserID, fromUserID).Scan(&notificationID)

	if notificationID == 0 {
		t.Fatalf("ERROR: Notification was created but ID not found")
	}

	return notificationID
}
