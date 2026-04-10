package test

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/chat"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	url2 "net/url"
	"os"
	"testing"
	"time"
)

func TestListChats(t *testing.T) {
	// Create a full test user with chats
	prefix := uniquePrefix("chat")
	userID, token := CreateFullTestUser(t, prefix)

	// Logged out - should return 401
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/chat?includeClosed=true", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Get chats for user
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var chats []chat.ChatRoomListEntry
	json2.Unmarshal(rsp(resp), &chats)

	// Should find chats
	assert.Greater(t, len(chats), 0)

	// Find a chat with a snippet
	found := (uint64)(0)
	for _, c := range chats {
		if len(c.Snippet) > 0 {
			found = c.ID
		}
	}
	assert.Greater(t, found, (uint64)(0), "Should find a chat with a snippet for user %d", userID)

	// Get with since param
	url := "/api/chat?jwt=" + token + "&since=" + url2.QueryEscape(time.Now().Format(time.RFC3339))
	resp, _ = getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Get with search param
	url = "/api/chat?jwt=" + token + "&search=test"
	resp, _ = getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Get the chat
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/"+fmt.Sprint(found)+"?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var c chat.ChatRoomListEntry
	json2.Unmarshal(rsp(resp), &c)
	assert.Equal(t, found, c.ID)

	// Get the messages
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/"+fmt.Sprint(found)+"/message?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var messages []chat.ChatMessage
	json2.Unmarshal(rsp(resp), &messages)
	assert.Equal(t, found, messages[0].Chatid)

	// Get an invalid chat - no auth
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/"+fmt.Sprint(found), nil))
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	// Invalid chat ID format
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/z?jwt="+token, nil))
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// Non-existent chat
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/1?jwt="+token, nil))
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	// Get invalid chat messages - no auth
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/1/message", nil))
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	// Non-existent chat messages
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/1/message?jwt="+token, nil))
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	// Invalid chat ID format for messages
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/z/message?jwt="+token, nil))
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestListChatsDefaultIncludesUser2Mod(t *testing.T) {
	prefix := uniquePrefix("U2MList")
	db := database.DBConn

	// Create a member and a mod on a group.
	memberID, memberToken := CreateFullTestUser(t, prefix+"_member")
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	groupID := CreateTestGroup(t, prefix+"_group")
	CreateTestMembership(t, memberID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")

	// Create a User2Mod chat with a message.
	chatID, err := chat.GetOrCreateUser2ModChat(db, memberID, groupID)
	assert.NoError(t, err)

	db.Exec("INSERT INTO chat_messages (chatid, userid, message, type, date, reviewrequired, processingrequired, processingsuccessful) VALUES (?, ?, 'test modmail', 'ModMail', NOW(), 0, 0, 1)",
		chatID, modID)

	// Default request (no chattypes param) should include the User2Mod chat.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/chat?jwt="+memberToken, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var chats []chat.ChatRoomListEntry
	json2.Unmarshal(rsp(resp), &chats)

	found := false
	for _, c := range chats {
		if c.ID == chatID {
			found = true
		}
	}
	assert.True(t, found, "Default chat list should include User2Mod chat %d for member %d", chatID, memberID)

	// Clean up.
	db.Exec("DELETE FROM chat_messages WHERE chatid = ?", chatID)
	db.Exec("DELETE FROM chat_roster WHERE chatid = ?", chatID)
	db.Exec("DELETE FROM chat_rooms WHERE id = ?", chatID)
}

func TestFreegleHidesModeratorUser2ModChats(t *testing.T) {
	// A moderator should NOT see other members' User2Mod chats on the Freegle
	// endpoint (/api/chat), only on the ModTools endpoint (/api/chat/rooms).
	prefix := uniquePrefix("U2MHide")
	db := database.DBConn

	// Create a member and a mod on a group.
	memberID := CreateTestUser(t, prefix+"_member", "Member")
	modID, modToken := CreateFullTestUser(t, prefix+"_mod")
	groupID := CreateTestGroup(t, prefix+"_group")
	CreateTestMembership(t, memberID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")

	// Create a User2Mod chat from the member (not the mod) with a message.
	chatID, err := chat.GetOrCreateUser2ModChat(db, memberID, groupID)
	assert.NoError(t, err)

	db.Exec("INSERT INTO chat_messages (chatid, userid, message, type, date, reviewrequired, processingrequired, processingsuccessful) VALUES (?, ?, 'help me', 'Default', NOW(), 0, 0, 1)",
		chatID, memberID)

	// Freegle endpoint (/api/chat) should NOT include this chat for the mod.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/chat?chattypes[]=User2Mod&jwt="+modToken, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var fdChats []chat.ChatRoomListEntry
	json2.Unmarshal(rsp(resp), &fdChats)

	foundOnFD := false
	for _, c := range fdChats {
		if c.ID == chatID {
			foundOnFD = true
		}
	}
	assert.False(t, foundOnFD, "Freegle /api/chat should NOT show User2Mod chat %d to mod %d (they are not the member)", chatID, modID)

	// ModTools endpoint (/api/chat/rooms) SHOULD include this chat for the mod.
	resp2, _ := getApp().Test(httptest.NewRequest("GET", "/api/chat/rooms?chattypes[]=User2Mod&jwt="+modToken, nil))
	assert.Equal(t, 200, resp2.StatusCode)

	var wrapper struct {
		Chatrooms []chat.ChatRoomListEntry `json:"chatrooms"`
	}
	json2.Unmarshal(rsp(resp2), &wrapper)

	foundOnMT := false
	for _, c := range wrapper.Chatrooms {
		if c.ID == chatID {
			foundOnMT = true
		}
	}
	assert.True(t, foundOnMT, "ModTools /api/chat/rooms should show User2Mod chat %d to mod %d", chatID, modID)

	// Clean up.
	db.Exec("DELETE FROM chat_messages WHERE chatid = ?", chatID)
	db.Exec("DELETE FROM chat_roster WHERE chatid = ?", chatID)
	db.Exec("DELETE FROM chat_rooms WHERE id = ?", chatID)
}

func TestKeepChatIncludesOldChat(t *testing.T) {
	// keepChat should return a chat even when its latestmessage is older than
	// the active lookback window (CHAT_ACTIVE_LIMIT days).
	prefix := uniquePrefix("keepChat")
	db := database.DBConn

	user1ID := CreateTestUser(t, prefix+"_u1", "Member")
	_, user1Token := CreateTestSession(t, user1ID)
	user2ID := CreateTestUser(t, prefix+"_u2", "Member")

	// Create a User2User chat and backdate it to 90 days ago (past the 31-day cutoff).
	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, reviewrequired, processingrequired, processingsuccessful) VALUES (?, ?, 'old message', DATE_SUB(NOW(), INTERVAL 90 DAY), 0, 0, 1)",
		chatID, user1ID)
	db.Exec("UPDATE chat_rooms SET latestmessage = DATE_SUB(NOW(), INTERVAL 90 DAY) WHERE id = ?", chatID)

	// Without keepChat (User2User only — 31-day cutoff, 90-day-old chat is outside the window).
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/chat?jwt="+user1Token+"&chattypes[]=User2User", nil))
	assert.Equal(t, 200, resp.StatusCode)
	var chatsWithout []chat.ChatRoomListEntry
	json2.Unmarshal(rsp(resp), &chatsWithout)
	foundWithout := false
	for _, c := range chatsWithout {
		if c.ID == chatID {
			foundWithout = true
		}
	}
	assert.False(t, foundWithout, "Old chat %d should NOT appear without keepChat", chatID)

	// With keepChat: old chat SHOULD appear regardless of date cutoff.
	url := fmt.Sprintf("/api/chat?jwt=%s&chattypes[]=User2User&keepChat=%d", user1Token, chatID)
	resp, _ = getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var chatsWith []chat.ChatRoomListEntry
	json2.Unmarshal(rsp(resp), &chatsWith)
	foundWith := false
	for _, c := range chatsWith {
		if c.ID == chatID {
			foundWith = true
		}
	}
	assert.True(t, foundWith, "Old chat %d SHOULD appear with keepChat=%d", chatID, chatID)

	// Clean up.
	db.Exec("DELETE FROM chat_messages WHERE chatid = ?", chatID)
	db.Exec("DELETE FROM chat_roster WHERE chatid = ?", chatID)
	db.Exec("DELETE FROM chat_rooms WHERE id = ?", chatID)
}

func TestGroupChatIconUsesNewestImage(t *testing.T) {
	// When a group has multiple images, the chat icon should use the newest
	// (highest-ID) one — matching V1 behaviour where "newest wins" via
	// $newroom[$room['id']] = $room overwriting with the last MySQL result.
	prefix := uniquePrefix("gicon")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	memberID := CreateTestUser(t, prefix+"_m", "Member")
	_, memberToken := CreateTestSession(t, memberID)
	CreateTestMembership(t, memberID, groupID, "Member")

	// Insert two images for the same group. Auto-increment guarantees img2ID > img1ID.
	var img1ID, img2ID uint64
	db.Exec("INSERT INTO groups_images (groupid, contenttype) VALUES (?, 'image/jpeg')", groupID)
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&img1ID)
	db.Exec("INSERT INTO groups_images (groupid, contenttype) VALUES (?, 'image/jpeg')", groupID)
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&img2ID)

	chatID := CreateTestChatRoom(t, memberID, nil, &groupID, "User2Mod")

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/chat?jwt="+memberToken, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var chats []chat.ChatRoomListEntry
	json2.Unmarshal(rsp(resp), &chats)

	var found *chat.ChatRoomListEntry
	for i := range chats {
		if chats[i].ID == chatID {
			found = &chats[i]
			break
		}
	}

	assert.NotNil(t, found, "User2Mod chat %d should appear in chat list", chatID)
	if found != nil {
		img2Suffix := fmt.Sprintf("gimg_%d.jpg", img2ID)
		img1Suffix := fmt.Sprintf("gimg_%d.jpg", img1ID)
		assert.Contains(t, found.Icon, img2Suffix, "Icon should use newest (highest-ID) group image")
		assert.NotContains(t, found.Icon, img1Suffix, "Icon should NOT use oldest (lowest-ID) group image")
	}

	// Clean up.
	db.Exec("DELETE FROM chat_roster WHERE chatid = ?", chatID)
	db.Exec("DELETE FROM chat_rooms WHERE id = ?", chatID)
	db.Exec("DELETE FROM groups_images WHERE id IN (?, ?)", img1ID, img2ID)
}

func TestCreateChatMessage(t *testing.T) {
	// Invalid chat id
	resp, _ := getApp().Test(httptest.NewRequest("POST", "/api/chat/-1/message", nil))
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// Create a mod user with a User2Mod chat for testing
	prefix := uniquePrefix("chatmsg")
	groupID := CreateTestGroup(t, prefix)
	modUserID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modUserID, groupID, "Moderator")
	chatid := CreateTestChatRoom(t, modUserID, nil, &groupID, "User2Mod")
	CreateTestChatMessage(t, chatid, modUserID, "Initial message")
	_, token := CreateTestSession(t, modUserID)

	// Logged out
	resp, _ = getApp().Test(httptest.NewRequest("POST", "/api/chat/"+fmt.Sprint(chatid)+"/message", nil))
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	// Undecodable payload
	request := httptest.NewRequest("POST", "/api/chat/"+fmt.Sprint(chatid)+"/message?jwt="+token, bytes.NewBuffer([]byte("Test")))
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// Invalid payload
	var payload chat.ChatMessage
	s, _ := json2.Marshal(payload)
	b := bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/"+fmt.Sprint(chatid)+"/message?jwt="+token, b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// Valid payload
	chatrsp := struct {
		Id uint64 `json:"id"`
	}{}

	str := "Test basic message"
	payload.Message = str
	s, _ = json2.Marshal(payload)
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/"+fmt.Sprint(chatid)+"/message?jwt="+token, b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &chatrsp)
	assert.Greater(t, chatrsp.Id, (uint64)(1))
}

func TestCreateChatMessageModnote(t *testing.T) {
	// modnote=true should create a ModMail type message.
	prefix := uniquePrefix("chatmodnote")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	modUserID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modUserID, groupID, "Moderator")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	CreateTestMembership(t, user1ID, groupID, "Member")
	CreateTestMembership(t, user2ID, groupID, "Member")

	// Create a User2User chat between user1 and user2.
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	CreateTestChatMessage(t, chatid, user1ID, "Hello")

	// The mod also needs access — for User2User chats the handler checks
	// if the sender is a mod of either user's group.
	_, modToken := CreateTestSession(t, modUserID)

	// Send a regular message (no modnote) — should be Default type.
	body := fmt.Sprintf(`{"message":"Regular note"}`)
	req := httptest.NewRequest("POST", "/api/chat/"+fmt.Sprint(chatid)+"/message?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var rspData struct {
		Id uint64 `json:"id"`
	}
	json2.NewDecoder(resp.Body).Decode(&rspData)
	regularMsgID := rspData.Id
	assert.Greater(t, regularMsgID, uint64(0))

	var regularType string
	db.Raw("SELECT type FROM chat_messages WHERE id = ?", regularMsgID).Scan(&regularType)
	assert.Equal(t, "Default", regularType, "Regular message should be Default type")

	// Send with modnote=true — should be ModMail type.
	body = fmt.Sprintf(`{"message":"Mod note from volunteer","modnote":true}`)
	req = httptest.NewRequest("POST", "/api/chat/"+fmt.Sprint(chatid)+"/message?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err = getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	json2.NewDecoder(resp.Body).Decode(&rspData)
	modnoteMsgID := rspData.Id
	assert.Greater(t, modnoteMsgID, uint64(0))

	var modnoteType string
	db.Raw("SELECT type FROM chat_messages WHERE id = ?", modnoteMsgID).Scan(&modnoteType)
	assert.Equal(t, "ModMail", modnoteType, "Modnote message should be ModMail type")
}

func TestCreateChatMessageLoveJunk(t *testing.T) {
	// Create test data for LoveJunk integration test
	prefix := uniquePrefix("lovejunk")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")

	// Create a message with spaces in subject (required for LoveJunk)
	msgID := CreateTestMessage(t, userID, groupID, "Test Offer Item", 55.9533, -3.1883)

	var payload chat.ChatMessageLovejunk

	payload.Refmsgid = &msgID
	firstname := "Test"
	payload.Firstname = &firstname
	lastname := "User"
	payload.Lastname = &lastname

	// Use longer timeout for LoveJunk tests - DB lookups can be slow under CI load.
	timeout := 5000

	// Without ljuserid
	s, _ := json2.Marshal(payload)
	b := bytes.NewBuffer(s)
	request := httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request, timeout)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// Without partnerkey
	ljuserid := uint64(time.Now().UnixNano())
	payload.Ljuserid = &ljuserid
	s, _ = json2.Marshal(payload)
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request, timeout)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// With invalid partnerkey
	payload.Partnerkey = "invalid"
	s, _ = json2.Marshal(payload)
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request, timeout)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	// Remaining tests require a valid LOVEJUNK_PARTNER_KEY env var.
	partnerKey := os.Getenv("LOVEJUNK_PARTNER_KEY")
	if partnerKey == "" {
		t.Log("LOVEJUNK_PARTNER_KEY not set, skipping integration tests")
		return
	}

	// With valid partnerkey but no message
	payload.Partnerkey = partnerKey
	s, _ = json2.Marshal(payload)
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request, timeout)
	if !assert.NotNil(t, resp, "expected response for valid partnerkey with no message") {
		return
	}
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// Valid
	payload.Message = "Test message"
	loc := "EH3 6SS"
	payload.PostcodePrefix = &loc
	s, _ = json2.Marshal(payload)
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request, timeout)
	if !assert.NotNil(t, resp, "expected response for valid LoveJunk request") {
		return
	}
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var ret chat.ChatMessageLovejunkResponse
	json2.Unmarshal(rsp(resp), &ret)
	assert.Greater(t, ret.Id, (uint64)(0))
	assert.Greater(t, ret.Chatid, (uint64)(0))

	// Initial reply
	payload.Message = "Test initial reply"
	payload.Initialreply = true
	offerid := uint64(123)
	payload.Offerid = &offerid
	s, _ = json2.Marshal(payload)
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request, timeout)
	if !assert.NotNil(t, resp, "expected response for initial reply") {
		return
	}
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &ret)
	assert.Greater(t, ret.Id, (uint64)(0))
	assert.Greater(t, ret.Chatid, (uint64)(0))
	assert.Greater(t, ret.Userid, (uint64)(0))

	// Fake a ban of the LJ user on the group
	var ban user.UserBanned
	ban.Userid = ret.Userid
	ban.Groupid = groupID
	ban.Byuser = ret.Userid
	db := database.DBConn
	db.Create(&ban)

	// Shouldn't be able to reply to a message on this group
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request, timeout)
	if !assert.NotNil(t, resp, "expected response for banned user") {
		return
	}
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestCreateChatMessageLoveJunkWithProfileUrl(t *testing.T) {
	// LoveJunk user creation should store the profile URL as an avatar in users_images.
	partnerKey := os.Getenv("LOVEJUNK_PARTNER_KEY")
	if partnerKey == "" {
		t.Log("LOVEJUNK_PARTNER_KEY not set, skipping integration test")
		return
	}

	prefix := uniquePrefix("ljprofile")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	msgID := CreateTestMessage(t, userID, groupID, "Test Offer Profile", 55.9533, -3.1883)

	ljuserid := uint64(time.Now().UnixNano())
	firstname := "Profile"
	lastname := "Test"
	profileurl := "https://example.com/profile.jpg"

	var payload chat.ChatMessageLovejunk
	payload.Refmsgid = &msgID
	payload.Ljuserid = &ljuserid
	payload.Partnerkey = partnerKey
	payload.Firstname = &firstname
	payload.Lastname = &lastname
	payload.Profileurl = &profileurl
	payload.Message = "Test with profile"

	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chat/lovejunk", bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request, 5000)
	if !assert.NotNil(t, resp) {
		return
	}
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var ret chat.ChatMessageLovejunkResponse
	json2.Unmarshal(rsp(resp), &ret)
	assert.Greater(t, ret.Userid, uint64(0))

	// Verify avatar was stored in users_images.
	db := database.DBConn
	var imageURL string
	db.Raw("SELECT url FROM users_images WHERE userid = ? ORDER BY id DESC LIMIT 1", ret.Userid).Scan(&imageURL)
	assert.Equal(t, "https://example.com/profile.jpg", imageURL)
}

func TestCreateChatMessageLoveJunkWithImageid(t *testing.T) {
	// LoveJunk chat messages with an imageid should link the image to the message.
	partnerKey := os.Getenv("LOVEJUNK_PARTNER_KEY")
	if partnerKey == "" {
		t.Log("LOVEJUNK_PARTNER_KEY not set, skipping integration test")
		return
	}

	prefix := uniquePrefix("ljimage")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	msgID := CreateTestMessage(t, userID, groupID, "Test Offer Image", 55.9533, -3.1883)

	// Create a chat_images row to link.
	db := database.DBConn
	db.Exec("INSERT INTO chat_images (externaluid) VALUES (?)", "test-lj-image-uid")
	var imageID uint64
	db.Raw("SELECT id FROM chat_images WHERE externaluid = 'test-lj-image-uid' ORDER BY id DESC LIMIT 1").Scan(&imageID)
	assert.Greater(t, imageID, uint64(0))

	ljuserid := uint64(time.Now().UnixNano())
	firstname := "Image"
	lastname := "Test"

	var payload chat.ChatMessageLovejunk
	payload.Refmsgid = &msgID
	payload.Ljuserid = &ljuserid
	payload.Partnerkey = partnerKey
	payload.Firstname = &firstname
	payload.Lastname = &lastname
	payload.Message = "Test with image"
	payload.Imageid = &imageID

	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chat/lovejunk", bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request, 5000)
	if !assert.NotNil(t, resp) {
		return
	}
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var ret chat.ChatMessageLovejunkResponse
	json2.Unmarshal(rsp(resp), &ret)
	assert.Greater(t, ret.Id, uint64(0))

	// Verify image was linked to the chat message.
	var linkedMsgID uint64
	db.Raw("SELECT chatmsgid FROM chat_images WHERE id = ?", imageID).Scan(&linkedMsgID)
	assert.Equal(t, ret.Id, linkedMsgID)
}

func TestPatchChatMessageNotLoggedIn(t *testing.T) {
	payload := map[string]interface{}{
		"id":            1,
		"roomid":        1,
		"replyexpected": true,
	}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("PATCH", "/api/chatmessages", bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestPatchChatMessageReplyExpected(t *testing.T) {
	db := database.DBConn

	// Create two users with a User2User chat
	prefix := uniquePrefix("patchmsg")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	msgID := CreateTestChatMessage(t, chatID, user1ID, "Test message for RSVP")
	_, token := CreateTestSession(t, user1ID)

	// Set replyexpected = true
	payload := map[string]interface{}{
		"id":            msgID,
		"roomid":        chatID,
		"replyexpected": true,
	}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("PATCH", "/api/chatmessages?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Verify in DB
	var replyexpected *bool
	db.Raw("SELECT replyexpected FROM chat_messages WHERE id = ?", msgID).Scan(&replyexpected)
	assert.NotNil(t, replyexpected)
	assert.True(t, *replyexpected)

	// Set replyexpected = false
	payload["replyexpected"] = false
	s, _ = json2.Marshal(payload)
	request = httptest.NewRequest("PATCH", "/api/chatmessages?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Verify in DB
	db.Raw("SELECT replyexpected FROM chat_messages WHERE id = ?", msgID).Scan(&replyexpected)
	assert.NotNil(t, replyexpected)
	assert.False(t, *replyexpected)
}

func TestPatchChatMessageNotYourMessage(t *testing.T) {
	// Create two users with a User2User chat
	prefix := uniquePrefix("patchnoturs")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	msgID := CreateTestChatMessage(t, chatID, user1ID, "User1's message")

	// Log in as user2 and try to patch user1's message
	_, token2 := CreateTestSession(t, user2ID)
	payload := map[string]interface{}{
		"id":            msgID,
		"roomid":        chatID,
		"replyexpected": true,
	}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("PATCH", "/api/chatmessages?jwt="+token2, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestPatchChatMessageNotFound(t *testing.T) {
	prefix := uniquePrefix("patchnf")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	payload := map[string]interface{}{
		"id":            999999999,
		"roomid":        999999999,
		"replyexpected": true,
	}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("PATCH", "/api/chatmessages?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestDeleteChatMessageNotLoggedIn(t *testing.T) {
	request := httptest.NewRequest("DELETE", "/api/chatmessages?id=1", nil)
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestDeleteChatMessage(t *testing.T) {
	db := database.DBConn

	// Create two users with a User2User chat
	prefix := uniquePrefix("delmsg")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	msgID := CreateTestChatMessage(t, chatID, user1ID, "Message to delete")
	_, token := CreateTestSession(t, user1ID)

	// Delete the message
	request := httptest.NewRequest("DELETE", fmt.Sprintf("/api/chatmessages?id=%d&jwt=%s", msgID, token), nil)
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Verify in DB - should be soft-deleted
	var deleted int
	var msgType string
	db.Raw("SELECT deleted, type FROM chat_messages WHERE id = ?", msgID).Row().Scan(&deleted, &msgType)
	assert.Equal(t, 1, deleted)
	assert.Equal(t, "Default", msgType)
}

func TestDeleteChatMessageNotYours(t *testing.T) {
	// Create two users with a User2User chat
	prefix := uniquePrefix("delnoturs")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	msgID := CreateTestChatMessage(t, chatID, user1ID, "User1's message")

	// Log in as user2 and try to delete user1's message
	_, token2 := CreateTestSession(t, user2ID)
	request := httptest.NewRequest("DELETE", fmt.Sprintf("/api/chatmessages?id=%d&jwt=%s", msgID, token2), nil)
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestDeleteChatMessageNotFound(t *testing.T) {
	prefix := uniquePrefix("delnf")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	request := httptest.NewRequest("DELETE", fmt.Sprintf("/api/chatmessages?id=999999999&jwt=%s", token), nil)
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestDeleteChatMessageMissingId(t *testing.T) {
	prefix := uniquePrefix("delmissid")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	request := httptest.NewRequest("DELETE", "/api/chatmessages?jwt="+token, nil)
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestDeleteChatMessageWithImage(t *testing.T) {
	db := database.DBConn

	// Create two users with a User2User chat
	prefix := uniquePrefix("delimgmsg")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	msgID := CreateTestChatMessage(t, chatID, user1ID, "Message with image")

	// Insert a chat_image linked to this message
	db.Exec("INSERT INTO chat_images (chatmsgid, contenttype) VALUES (?, 'image/jpeg')", msgID)
	var imageID uint64
	db.Raw("SELECT id FROM chat_images WHERE chatmsgid = ? ORDER BY id DESC LIMIT 1", msgID).Scan(&imageID)
	assert.Greater(t, imageID, uint64(0))

	// Link the image to the message
	db.Exec("UPDATE chat_messages SET imageid = ?, type = 'Image' WHERE id = ?", imageID, msgID)

	_, token := CreateTestSession(t, user1ID)

	// Delete the message
	request := httptest.NewRequest("DELETE", fmt.Sprintf("/api/chatmessages?id=%d&jwt=%s", msgID, token), nil)
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Verify message is soft-deleted with imageid cleared
	var deleted int
	var imageid *uint64
	var msgType string
	db.Raw("SELECT deleted, imageid, type FROM chat_messages WHERE id = ?", msgID).Row().Scan(&deleted, &imageid, &msgType)
	assert.Equal(t, 1, deleted)
	assert.Nil(t, imageid)
	assert.Equal(t, "Default", msgType)

	// Verify chat_images row is deleted
	var imgCount int64
	db.Raw("SELECT COUNT(*) FROM chat_images WHERE chatmsgid = ?", msgID).Scan(&imgCount)
	assert.Equal(t, int64(0), imgCount)
}

func TestUserBanned(t *testing.T) {
	db := database.DBConn

	// Create test user and group for ban test
	prefix := uniquePrefix("banned")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")

	// Create a ban
	var ban user.UserBanned
	ban.Userid = userID
	ban.Groupid = groupID
	ban.Byuser = userID
	db.Create(&ban)
}

func TestPostChatRoomNotLoggedIn(t *testing.T) {
	// POST without auth should return 401
	payload := map[string]interface{}{"id": 1, "action": "Nudge"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms", bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestPostChatRoomNudge(t *testing.T) {
	prefix := uniquePrefix("nudge")

	// Create two users and a User2User chat
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	CreateTestChatMessage(t, chatid, user1ID, "Hello")
	_, token := CreateTestSession(t, user1ID)

	// Nudge
	payload := map[string]interface{}{"id": chatid, "action": "Nudge"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Greater(t, result["id"], float64(0))

	// Nudge again - should return existing nudge ID (not create a new one)
	s, _ = json2.Marshal(payload)
	request = httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result2 map[string]interface{}
	json2.Unmarshal(rsp(resp), &result2)
	assert.Equal(t, float64(0), result2["ret"])
	assert.Equal(t, result["id"], result2["id"])
}

func TestPostChatRoomNudgeNotMember(t *testing.T) {
	prefix := uniquePrefix("nudgenm")

	// Create chat between user1 and user2, but try to nudge as user3
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	user3ID := CreateTestUser(t, prefix+"_u3", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	_, token3 := CreateTestSession(t, user3ID)

	payload := map[string]interface{}{"id": chatid, "action": "Nudge"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token3, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestPostChatRoomTyping(t *testing.T) {
	prefix := uniquePrefix("typing")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	_, token := CreateTestSession(t, user1ID)

	// Create roster entry first (typing updates existing roster entry)
	db := database.DBConn
	db.Exec("INSERT INTO chat_roster (chatid, userid, status) VALUES (?, ?, 'Online')", chatid, user1ID)

	// Create a recent unmailed chat message to verify date bump
	msgid := CreateTestChatMessage(t, chatid, user1ID, "Recent typing msg "+prefix)
	// Set date to 10 seconds ago and mailedtoall = 0 (within the 30s window)
	db.Exec("UPDATE chat_messages SET date = DATE_SUB(NOW(), INTERVAL 10 SECOND), mailedtoall = 0 WHERE id = ?", msgid)

	var dateBefore string
	db.Raw("SELECT date FROM chat_messages WHERE id = ?", msgid).Scan(&dateBefore)

	payload := map[string]interface{}{"id": chatid, "action": "Typing"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify date was bumped (should be newer than before)
	var dateAfter string
	db.Raw("SELECT date FROM chat_messages WHERE id = ?", msgid).Scan(&dateAfter)
	assert.NotEqual(t, dateBefore, dateAfter, "Typing should bump recent unmailed message dates")
}

func TestPostChatRoomRosterUpdate(t *testing.T) {
	prefix := uniquePrefix("roster")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	msgid := CreateTestChatMessage(t, chatid, user2ID, "Hello from user2")
	_, token := CreateTestSession(t, user1ID)

	// Mark as read (roster update with lastmsgseen)
	payload := map[string]interface{}{"id": chatid, "lastmsgseen": msgid}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.NotNil(t, result["roster"])
	// Unseen should be 0 since we just marked the message as seen
	assert.Equal(t, float64(0), result["unseen"])
}

func TestPostChatRoomHideChat(t *testing.T) {
	prefix := uniquePrefix("hide")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	_, token := CreateTestSession(t, user1ID)

	// Hide chat (status=Closed)
	payload := map[string]interface{}{"id": chatid, "status": "Closed"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify roster entry has Closed status
	roster := result["roster"].([]interface{})
	found := false
	for _, r := range roster {
		entry := r.(map[string]interface{})
		if uint64(entry["userid"].(float64)) == user1ID {
			assert.Equal(t, "Closed", entry["status"])
			found = true
		}
	}
	assert.True(t, found, "Should find user in roster")
}

func TestPostChatRoomBlockChat(t *testing.T) {
	prefix := uniquePrefix("block")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	_, token := CreateTestSession(t, user1ID)

	// Block chat
	payload := map[string]interface{}{"id": chatid, "status": "Blocked"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Now try to close (should remain Blocked since BLOCKED takes precedence)
	payload = map[string]interface{}{"id": chatid, "status": "Closed"}
	s, _ = json2.Marshal(payload)
	request = httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	roster := result["roster"].([]interface{})
	for _, r := range roster {
		entry := r.(map[string]interface{})
		if uint64(entry["userid"].(float64)) == user1ID {
			// Should still be Blocked, not Closed
			assert.Equal(t, "Blocked", entry["status"])
		}
	}
}

func TestPostChatRoomRosterUpdateNonMember(t *testing.T) {
	prefix := uniquePrefix("rosternm")

	// Create chat between user1 and user2
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	user3ID := CreateTestUser(t, prefix+"_u3", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	_, token3 := CreateTestSession(t, user3ID)

	// User3 tries to update roster for a chat they're not in - should fail
	payload := map[string]interface{}{"id": chatid}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token3, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestPostChatRoomUnhide(t *testing.T) {
	prefix := uniquePrefix("unhide")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	_, token := CreateTestSession(t, user1ID)

	// Hide chat first
	payload := map[string]interface{}{"id": chatid, "status": "Closed"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Unhide chat (status=Online)
	payload = map[string]interface{}{"id": chatid, "status": "Online"}
	s, _ = json2.Marshal(payload)
	request = httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	roster := result["roster"].([]interface{})
	for _, r := range roster {
		entry := r.(map[string]interface{})
		if uint64(entry["userid"].(float64)) == user1ID {
			assert.Equal(t, "Online", entry["status"])
		}
	}
}

// --- Adversarial tests ---

func TestPostChatRoomDoubleNudge(t *testing.T) {
	// Second nudge should be idempotent (returns existing nudge ID).
	prefix := uniquePrefix("nudge_dbl")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	CreateTestChatMessage(t, chatid, user2ID, "Hello")
	_, token := CreateTestSession(t, user1ID)

	payload := map[string]interface{}{"id": chatid, "action": "Nudge"}
	s, _ := json2.Marshal(payload)

	// First nudge.
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	var r1 map[string]interface{}
	json2.Unmarshal(rsp(resp), &r1)
	firstID := r1["id"]

	// Second nudge - should return same ID (not create a new one).
	s, _ = json2.Marshal(payload)
	request = httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	var r2 map[string]interface{}
	json2.Unmarshal(rsp(resp), &r2)
	assert.Equal(t, firstID, r2["id"], "Double nudge should return the same ID")
}

func TestPostChatRoomHideAlreadyHidden(t *testing.T) {
	// Hiding an already-hidden chat should be idempotent.
	prefix := uniquePrefix("hide_dbl")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	_, token := CreateTestSession(t, user1ID)

	payload := map[string]interface{}{"id": chatid, "status": "Closed"}
	s, _ := json2.Marshal(payload)

	// First hide.
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Second hide - should succeed without error.
	s, _ = json2.Marshal(payload)
	request = httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestPostChatRoomBlockThenClose(t *testing.T) {
	// Block then Close should NOT downgrade to Closed (Block takes precedence).
	prefix := uniquePrefix("blk_cls")
	db := database.DBConn

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	_, token := CreateTestSession(t, user1ID)

	// Block.
	payload := map[string]interface{}{"id": chatid, "status": "Blocked"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Try to Close (should NOT override Blocked).
	payload = map[string]interface{}{"id": chatid, "status": "Closed"}
	s, _ = json2.Marshal(payload)
	request = httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Status should still be Blocked.
	var status string
	db.Raw("SELECT status FROM chat_roster WHERE chatid = ? AND userid = ?", chatid, user1ID).Scan(&status)
	assert.Equal(t, "Blocked", status, "Closed should not override Blocked")
}

func TestPostChatRoomNonExistentChat(t *testing.T) {
	prefix := uniquePrefix("chat_ne")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	// Roster update on non-existent chat.
	payload := map[string]interface{}{"id": 999999999, "status": "Online"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)

	// Should return ret=2 (not visible), not crash.
	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestPostChatRoomEmptyBody(t *testing.T) {
	prefix := uniquePrefix("chat_empty")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer([]byte("{}")))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)

	// Empty body with no action and no id should return 400.
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestPostChatRoomTypingNonExistentChat(t *testing.T) {
	prefix := uniquePrefix("type_ne")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	// Typing on non-existent chat should return not found.
	payload := map[string]interface{}{"id": 999999999, "action": "Typing"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

// =============================================================================
// PutChatRoom tests
// =============================================================================

func TestPutChatRoom(t *testing.T) {
	prefix := uniquePrefix("putchat")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token := CreateTestSession(t, user1ID)

	// Create a new chat room.
	payload := map[string]interface{}{"userid": user2ID}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/chat/rooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])
	chatID := uint64(result["id"].(float64))
	assert.Greater(t, chatID, uint64(0))

	// Verify chat room was created in DB.
	db := database.DBConn
	var chattype string
	var u1, u2 uint64
	db.Raw("SELECT chattype, user1, user2 FROM chat_rooms WHERE id = ?", chatID).Row().Scan(&chattype, &u1, &u2)
	assert.Equal(t, utils.CHAT_TYPE_USER2USER, chattype)
	assert.Equal(t, user1ID, u1)
	assert.Equal(t, user2ID, u2)

	// Verify roster entries were created for both users.
	var rosterCount int64
	db.Raw("SELECT COUNT(*) FROM chat_roster WHERE chatid = ?", chatID).Scan(&rosterCount)
	assert.Equal(t, int64(2), rosterCount)
}

func TestPutChatRoomAlreadyExists(t *testing.T) {
	prefix := uniquePrefix("putchat_exists")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token := CreateTestSession(t, user1ID)

	existingChatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")

	payload := map[string]interface{}{"userid": user2ID}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/chat/rooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, float64(existingChatID), result["id"])
}

func TestPutChatRoomAlreadyExistsReversed(t *testing.T) {
	prefix := uniquePrefix("putchat_rev")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token2 := CreateTestSession(t, user2ID)

	existingChatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")

	payload := map[string]interface{}{"userid": user1ID}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/chat/rooms?jwt="+token2, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, float64(existingChatID), result["id"])
}

func TestPutChatRoomSelf(t *testing.T) {
	prefix := uniquePrefix("putchat_self")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	payload := map[string]interface{}{"userid": user1ID}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/chat/rooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestPutChatRoomNotLoggedIn(t *testing.T) {
	payload := map[string]interface{}{"userid": 12345}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/chat/rooms", bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestPutChatRoomConcurrent(t *testing.T) {
	prefix := uniquePrefix("putchat_conc")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token := CreateTestSession(t, user1ID)

	done := make(chan uint64, 2)
	for i := 0; i < 2; i++ {
		go func() {
			payload := map[string]interface{}{"userid": user2ID}
			s, _ := json2.Marshal(payload)
			request := httptest.NewRequest("PUT", "/api/chat/rooms?jwt="+token, bytes.NewBuffer(s))
			request.Header.Set("Content-Type", "application/json")
			resp, err := getApp().Test(request, -1)
			if err != nil || resp.StatusCode != 200 {
				done <- 0
				return
			}
			var result map[string]interface{}
			json2.NewDecoder(resp.Body).Decode(&result)
			if id, ok := result["id"].(float64); ok {
				done <- uint64(id)
			} else {
				done <- 0
			}
		}()
	}

	id1 := <-done
	id2 := <-done

	assert.Greater(t, id1, uint64(0), "First concurrent request should succeed")
	assert.Greater(t, id2, uint64(0), "Second concurrent request should succeed")
	assert.Equal(t, id1, id2, "Both requests should return the same chat room ID")

	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM chat_rooms WHERE ((user1 = ? AND user2 = ?) OR (user1 = ? AND user2 = ?)) AND chattype = 'User2User'",
		user1ID, user2ID, user2ID, user1ID).Scan(&count)
	assert.Equal(t, int64(1), count, "Only one chat room should exist")
}

func TestPutChatRoomMissingUserid(t *testing.T) {
	prefix := uniquePrefix("putchat_noid")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	payload := map[string]interface{}{}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/chat/rooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestPutChatRoomInvalidBody(t *testing.T) {
	prefix := uniquePrefix("putchat_bad")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	request := httptest.NewRequest("PUT", "/api/chat/rooms?jwt="+token, bytes.NewBuffer([]byte("not json")))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestPutChatRoomUpdateRosterUnblocks(t *testing.T) {
	prefix := uniquePrefix("putchat_roster")
	db := database.DBConn
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token := CreateTestSession(t, user1ID)

	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	// Create roster entries (CreateTestChatRoom only creates the room, not roster).
	db.Exec("INSERT INTO chat_roster (chatid, userid, status, date) VALUES (?, ?, 'Online', NOW())", chatID, user1ID)
	db.Exec("INSERT INTO chat_roster (chatid, userid, status, date) VALUES (?, ?, 'Online', NOW())", chatID, user2ID)
	db.Exec("UPDATE chat_roster SET status = 'Blocked' WHERE chatid = ? AND userid = ?", chatID, user1ID)

	var statusBefore string
	db.Raw("SELECT status FROM chat_roster WHERE chatid = ? AND userid = ?", chatID, user1ID).Scan(&statusBefore)
	assert.Equal(t, "Blocked", statusBefore)

	updateRoster := true
	payload := map[string]interface{}{"userid": user2ID, "updateRoster": updateRoster}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/chat/rooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, float64(chatID), result["id"])

	var statusAfter string
	db.Raw("SELECT status FROM chat_roster WHERE chatid = ? AND userid = ?", chatID, user1ID).Scan(&statusAfter)
	assert.Equal(t, "Online", statusAfter, "UpdateRoster should unblock the chat")
}

func TestPutChatRoomWithoutUpdateRosterDoesNotUnblock(t *testing.T) {
	prefix := uniquePrefix("putchat_noroster")
	db := database.DBConn
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token := CreateTestSession(t, user1ID)

	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	// Create roster entries then block.
	db.Exec("INSERT INTO chat_roster (chatid, userid, status, date) VALUES (?, ?, 'Online', NOW())", chatID, user1ID)
	db.Exec("INSERT INTO chat_roster (chatid, userid, status, date) VALUES (?, ?, 'Online', NOW())", chatID, user2ID)
	db.Exec("UPDATE chat_roster SET status = 'Blocked' WHERE chatid = ? AND userid = ?", chatID, user1ID)

	payload := map[string]interface{}{"userid": user2ID}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/chat/rooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var statusAfter string
	db.Raw("SELECT status FROM chat_roster WHERE chatid = ? AND userid = ?", chatID, user1ID).Scan(&statusAfter)
	assert.Equal(t, "Blocked", statusAfter, "Without updateRoster, chat should remain blocked")
}

// =============================================================================
// AllSeen tests
// =============================================================================

func TestAllSeen(t *testing.T) {
	prefix := uniquePrefix("allseen")
	db := database.DBConn
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	user3ID := CreateTestUser(t, prefix+"_u3", "User")
	_, token := CreateTestSession(t, user1ID)

	chat1ID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	msg1ID := CreateTestChatMessage(t, chat1ID, user2ID, "Hello from u2 in chat1")
	msg2ID := CreateTestChatMessage(t, chat1ID, user2ID, "Second msg in chat1")
	chat2ID := CreateTestChatRoom(t, user1ID, &user3ID, nil, "User2User")
	msg3ID := CreateTestChatMessage(t, chat2ID, user3ID, "Hello from u3 in chat2")

	db.Exec("INSERT INTO chat_roster (chatid, userid, status, lastmsgseen, date) VALUES (?, ?, 'Online', 0, NOW()) ON DUPLICATE KEY UPDATE lastmsgseen = 0", chat1ID, user1ID)
	db.Exec("INSERT INTO chat_roster (chatid, userid, status, lastmsgseen, date) VALUES (?, ?, 'Online', 0, NOW()) ON DUPLICATE KEY UPDATE lastmsgseen = 0", chat2ID, user1ID)

	var unseenBefore1, unseenBefore2 uint64
	db.Raw("SELECT lastmsgseen FROM chat_roster WHERE chatid = ? AND userid = ?", chat1ID, user1ID).Scan(&unseenBefore1)
	db.Raw("SELECT lastmsgseen FROM chat_roster WHERE chatid = ? AND userid = ?", chat2ID, user1ID).Scan(&unseenBefore2)
	assert.Equal(t, uint64(0), unseenBefore1)
	assert.Equal(t, uint64(0), unseenBefore2)

	payload := map[string]interface{}{"action": "AllSeen"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])

	var lastSeen1, lastSeen2 uint64
	db.Raw("SELECT lastmsgseen FROM chat_roster WHERE chatid = ? AND userid = ?", chat1ID, user1ID).Scan(&lastSeen1)
	db.Raw("SELECT lastmsgseen FROM chat_roster WHERE chatid = ? AND userid = ?", chat2ID, user1ID).Scan(&lastSeen2)
	assert.Equal(t, msg2ID, lastSeen1, "chat1 lastmsgseen should be max message ID")
	assert.Equal(t, msg3ID, lastSeen2, "chat2 lastmsgseen should be max message ID")

	db.Exec("INSERT INTO chat_roster (chatid, userid, status, lastmsgseen, date) VALUES (?, ?, 'Online', 0, NOW()) ON DUPLICATE KEY UPDATE lastmsgseen = lastmsgseen", chat1ID, user2ID)
	var user2LastSeen uint64
	db.Raw("SELECT lastmsgseen FROM chat_roster WHERE chatid = ? AND userid = ?", chat1ID, user2ID).Scan(&user2LastSeen)
	assert.NotEqual(t, msg2ID, user2LastSeen, "user2's lastmsgseen should not have changed")

	_ = msg1ID
}

func TestAllSeenNotLoggedIn(t *testing.T) {
	payload := map[string]interface{}{"action": "AllSeen"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms", bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestAllSeenNoChats(t *testing.T) {
	prefix := uniquePrefix("allseen_empty")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	payload := map[string]interface{}{"action": "AllSeen"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])
}

// =============================================================================
// ReferToSupport tests
// =============================================================================

func TestReferToSupport(t *testing.T) {
	prefix := uniquePrefix("refersupport")
	db := database.DBConn
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	CreateTestChatMessage(t, chatid, user1ID, "Hello")
	_, token := CreateTestSession(t, user1ID)

	payload := map[string]interface{}{"id": chatid, "action": "ReferToSupport"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])

	var taskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = 'refer_to_support' AND JSON_EXTRACT(data, '$.chatid') = ?", chatid).Scan(&taskCount)
	assert.Greater(t, taskCount, int64(0))
}

func TestReferToSupportNotMember(t *testing.T) {
	prefix := uniquePrefix("refersupport_nm")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	user3ID := CreateTestUser(t, prefix+"_u3", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	_, token3 := CreateTestSession(t, user3ID)

	payload := map[string]interface{}{"id": chatid, "action": "ReferToSupport"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token3, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestReferToSupportNotLoggedIn(t *testing.T) {
	payload := map[string]interface{}{"id": 1, "action": "ReferToSupport"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms", bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestReferToSupportNonExistentChat(t *testing.T) {
	prefix := uniquePrefix("refersupport_ne")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	payload := map[string]interface{}{"id": 999999999, "action": "ReferToSupport"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestReferToSupportMissingChatID(t *testing.T) {
	prefix := uniquePrefix("refersupport_noid")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	payload := map[string]interface{}{"action": "ReferToSupport"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}


// Helper to set up a moderator with a group and User2Mod chat containing messages.
func setupModChatData(t *testing.T, prefix string) (modID uint64, userID uint64, groupID uint64, chatID uint64, token string) {
	modID = CreateTestUser(t, prefix+"_mod", "Moderator")
	userID = CreateTestUser(t, prefix+"_user", "User")
	groupID = CreateTestGroup(t, prefix+"_group")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, userID, groupID, "Member")

	chatID = CreateTestChatRoom(t, userID, nil, &groupID, "User2Mod")

	db := database.DBConn
	// Create messages that are visible (processingsuccessful=1).
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Hello from user', NOW(), 1, 0, 0)",
		chatID, userID)
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Another message', NOW(), 1, 0, 0)",
		chatID, userID)
	// Update latestmessage on room.
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", chatID)

	_, token = CreateTestSession(t, modID)
	return
}

func TestUnseenCountMT(t *testing.T) {
	prefix := uniquePrefix("UnseenMT")
	modID, _, _, _, token := setupModChatData(t, prefix)
	_ = modID

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatrooms?count=true&chattypes=User2Mod,Mod2Mod&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "count")
	// Should have at least 2 unseen messages (no roster entry = all unseen).
	assert.GreaterOrEqual(t, result["count"].(float64), float64(2))
}

func TestUnseenCountMTNotLoggedIn(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/chatrooms?count=true&chattypes=User2Mod,Mod2Mod", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(1), result["ret"])
}

func TestUnseenCountMTZeroWhenSeen(t *testing.T) {
	prefix := uniquePrefix("UnseenSeen")
	modID, _, _, chatID, token := setupModChatData(t, prefix)

	db := database.DBConn
	// Mark all as seen by creating/updating roster entry with lastmsgseen higher than any auto-increment ID.
	// schema.sql sets chat_messages AUTO_INCREMENT ~1.75 billion, so use 9999999999 (10 billion).
	db.Exec("INSERT INTO chat_roster (chatid, userid, lastmsgseen, status, date) VALUES (?, ?, 9999999999, 'Online', NOW()) ON DUPLICATE KEY UPDATE lastmsgseen = 9999999999",
		chatID, modID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatrooms?count=true&chattypes=User2Mod,Mod2Mod&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, float64(0), result["count"])
}

func TestFetchChatMT(t *testing.T) {
	prefix := uniquePrefix("FetchMT")
	_, _, _, chatID, token := setupModChatData(t, prefix)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/%d?jwt=%s", chatID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var chatroom map[string]interface{}
	json2.Unmarshal(rsp(resp), &chatroom)
	assert.Equal(t, float64(chatID), chatroom["id"])
	assert.Equal(t, "User2Mod", chatroom["chattype"])
	assert.Contains(t, chatroom, "unseen")
}

func TestFetchChatMTPermissionDenied(t *testing.T) {
	prefix := uniquePrefix("FetchPerm")
	_, _, _, chatID, _ := setupModChatData(t, prefix)

	// Create a different user who is NOT a moderator of the group.
	otherID := CreateTestUser(t, prefix+"_other", "User")
	_, otherToken := CreateTestSession(t, otherID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/%d?jwt=%s", chatID, otherToken), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestListChatsMT(t *testing.T) {
	prefix := uniquePrefix("ListMT")
	_, _, _, _, token := setupModChatData(t, prefix)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/rooms?chattypes=User2Mod,Mod2Mod&summary=true&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "chatrooms")

	chatrooms := result["chatrooms"].([]interface{})
	assert.GreaterOrEqual(t, len(chatrooms), 1)

	first := chatrooms[0].(map[string]interface{})
	assert.Contains(t, first, "id")
	assert.Contains(t, first, "chattype")
	assert.Contains(t, first, "name")
	assert.Contains(t, first, "unseen")
}

func TestListChatsMTMod2Mod(t *testing.T) {
	prefix := uniquePrefix("ListM2M")
	mod1ID := CreateTestUser(t, prefix+"_mod1", "Moderator")
	mod2ID := CreateTestUser(t, prefix+"_mod2", "Moderator")
	groupID := CreateTestGroup(t, prefix+"_group")
	CreateTestMembership(t, mod1ID, groupID, "Moderator")
	CreateTestMembership(t, mod2ID, groupID, "Moderator")

	chatID := CreateTestChatRoom(t, mod1ID, &mod2ID, &groupID, "Mod2Mod")

	db := database.DBConn
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Mod message', NOW(), 1, 0, 0)",
		chatID, mod1ID)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", chatID)

	_, token := CreateTestSession(t, mod2ID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/rooms?chattypes=User2Mod,Mod2Mod&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	chatrooms := result["chatrooms"].([]interface{})
	// Should contain the Mod2Mod chat.
	found := false
	for _, cr := range chatrooms {
		room := cr.(map[string]interface{})
		if room["id"].(float64) == float64(chatID) {
			found = true
			assert.Equal(t, "Mod2Mod", room["chattype"])
			break
		}
	}
	assert.True(t, found, "Should find the Mod2Mod chat in the list")
}

func TestListChatsMTEmpty(t *testing.T) {
	prefix := uniquePrefix("ListEmpty")
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	// User has no moderator memberships, so should get empty list.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/rooms?chattypes=User2Mod,Mod2Mod&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	chatrooms := result["chatrooms"].([]interface{})
	assert.Equal(t, 0, len(chatrooms))
}

func TestListChatsMTSearch(t *testing.T) {
	prefix := uniquePrefix("ListSearch")
	_, _, _, _, token := setupModChatData(t, prefix)

	// Search for message content.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/rooms?chattypes=User2Mod,Mod2Mod&search=Hello&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "chatrooms")
}

func TestReviewChatMessages(t *testing.T) {
	prefix := uniquePrefix("ReviewMsgs")
	modID, userID, groupID, _, token := setupModChatData(t, prefix)
	_ = modID

	// Create a User2User chat between users where one is in the mod's group.
	user2ID := CreateTestUser(t, prefix+"_user2", "User")
	u2uChatID := CreateTestChatRoom(t, userID, &user2ID, nil, "User2User")

	db := database.DBConn
	// Create a message pending review.
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Spam message', NOW(), 1, 1, 0)",
		u2uChatID, user2ID)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", u2uChatID)

	// Also create a User2Mod review message.
	u2mChatID := CreateTestChatRoom(t, user2ID, nil, &groupID, "User2Mod")
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Review this', NOW(), 1, 1, 0)",
		u2mChatID, user2ID)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", u2mChatID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?limit=10&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "chatmessages")
	assert.Contains(t, result, "context")

	msgs := result["chatmessages"].([]interface{})
	assert.GreaterOrEqual(t, len(msgs), 1)

	// Each message should have a chatroom embedded.
	first := msgs[0].(map[string]interface{})
	assert.Contains(t, first, "chatroom")
	chatroom := first["chatroom"].(map[string]interface{})
	assert.Contains(t, chatroom, "chattype")
}

func TestReviewChatMessagesWithImage(t *testing.T) {
	prefix := uniquePrefix("ReviewImg")
	modID, userID, groupID, _, token := setupModChatData(t, prefix)
	_ = modID

	// Create a User2User chat with an Image-type message pending review.
	user2ID := CreateTestUser(t, prefix+"_user2", "User")
	u2uChatID := CreateTestChatRoom(t, userID, &user2ID, nil, "User2User")

	db := database.DBConn
	// Create an Image-type message pending review.
	var msgID uint64
	db.Exec("INSERT INTO chat_messages (chatid, userid, type, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Image', '', NOW(), 1, 1, 0)",
		u2uChatID, user2ID)
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&msgID)

	// Create a chat_images entry for this message.
	db.Exec("INSERT INTO chat_images (chatmsgid, externaluid, externalmods) VALUES (?, 'test-uid-123', '{}')", msgID)
	// Update the message to point to the image.
	db.Exec("UPDATE chat_messages SET imageid = (SELECT id FROM chat_images WHERE chatmsgid = ?) WHERE id = ?", msgID, msgID)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", u2uChatID)

	_ = groupID

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?limit=10&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	msgs := result["chatmessages"].([]interface{})
	// Find the Image message.
	found := false
	for _, m := range msgs {
		msg := m.(map[string]interface{})
		if msg["type"] == "Image" && msg["image"] != nil {
			found = true
			image := msg["image"].(map[string]interface{})
			assert.Contains(t, image, "path")
			assert.Contains(t, image, "ouruid")
			assert.Equal(t, "test-uid-123", image["ouruid"])
			break
		}
	}
	assert.True(t, found, "Should find an Image message with image data in review queue")
}

func TestReviewChatMessagesNotModerator(t *testing.T) {
	prefix := uniquePrefix("ReviewNonMod")
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	// Non-moderator should get empty list.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	msgs := result["chatmessages"].([]interface{})
	assert.Equal(t, 0, len(msgs))
}

func TestReviewChatMessagesSenderOnlyExcluded(t *testing.T) {
	// When the SENDER is in the mod's group but the RECIPIENT is in a different
	// group, the message should NOT appear in chat review. Only messages where
	// the recipient is in the mod's group should appear.
	prefix := uniquePrefix("ReviewSenderOnly")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	otherGroupID := CreateTestGroup(t, prefix + "_other")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	senderID := CreateTestUser(t, prefix+"_sender", "User")
	recipientID := CreateTestUser(t, prefix+"_recip", "User")
	CreateTestMembership(t, senderID, groupID, "Member")    // sender in mod's group
	CreateTestMembership(t, recipientID, otherGroupID, "Member") // recipient in different group

	chatID := CreateTestChatRoom(t, senderID, &recipientID, nil, "User2User")
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Spam from group member', NOW(), 1, 1, 0)",
		chatID, senderID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?limit=100&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	msgs := result["chatmessages"].([]interface{})

	// Should NOT find the message — recipient is not in mod's group
	for _, m := range msgs {
		msg := m.(map[string]interface{})
		chatroom := msg["chatroom"].(map[string]interface{})
		assert.NotEqual(t, float64(chatID), chatroom["id"],
			"Message where only sender is in mod's group should not appear in chat review")
	}
}

func TestReviewChatMessagesOrphanRecipient(t *testing.T) {
	// When the recipient has NO memberships at all and the sender is in the
	// mod's group, the message SHOULD appear (orphan safety net,).
	prefix := uniquePrefix("ReviewOrphan")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	senderID := CreateTestUser(t, prefix+"_sender", "User")
	orphanID := CreateTestUser(t, prefix+"_orphan", "User")
	CreateTestMembership(t, senderID, groupID, "Member") // sender in mod's group
	// orphanID has NO memberships

	chatID := CreateTestChatRoom(t, senderID, &orphanID, nil, "User2User")
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Spam to orphan', NOW(), 1, 1, 0)",
		chatID, senderID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?limit=100&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	msgs := result["chatmessages"].([]interface{})

	// Should find the message — recipient has no memberships, sender is in mod's group
	found := false
	for _, m := range msgs {
		msg := m.(map[string]interface{})
		chatroom := msg["chatroom"].(map[string]interface{})
		if chatroom["id"] == float64(chatID) {
			found = true
			break
		}
	}
	assert.True(t, found, "Message to orphan recipient (no memberships) should appear in chat review")
}

func TestChatMessagesForRoom(t *testing.T) {
	prefix := uniquePrefix("RoomMsgs")
	_, _, _, chatID, token := setupModChatData(t, prefix)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?roomid=%d&jwt=%s", chatID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "chatmessages")

	msgs := result["chatmessages"].([]interface{})
	assert.GreaterOrEqual(t, len(msgs), 2)
}

func TestChatMessagesForRoomPermissionDenied(t *testing.T) {
	prefix := uniquePrefix("RoomPerm")
	_, _, _, chatID, _ := setupModChatData(t, prefix)

	otherID := CreateTestUser(t, prefix+"_other", "User")
	_, otherToken := CreateTestSession(t, otherID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?roomid=%d&jwt=%s", chatID, otherToken), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestGetChatRoomsMTV2Path(t *testing.T) {
	req := httptest.NewRequest("GET", "/apiv2/chatrooms", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestGetChatRoomsMTByIdParam(t *testing.T) {
	// GET /chatrooms?id=X should return the chat, not 400.
	prefix := uniquePrefix("ChatRoomsMTById")
	db := database.DBConn

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token := CreateTestSession(t, user1ID)

	// Create a User2User chat.
	db.Exec("INSERT INTO chat_rooms (user1, user2, chattype) VALUES (?, ?, 'User2User')", user1ID, user2ID)
	var chatID uint64
	db.Raw("SELECT id FROM chat_rooms WHERE user1 = ? AND user2 = ? ORDER BY id DESC LIMIT 1", user1ID, user2ID).Scan(&chatID)
	assert.NotZero(t, chatID)

	// Fetch via /chatrooms?id=X — this is the V1 pattern used by master frontend.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatrooms?id=%d&jwt=%s", chatID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode, "GET /chatrooms?id=X should return 200, not 400")

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(chatID), result["id"], "Should return the requested chat")

	t.Cleanup(func() {
		db.Exec("DELETE FROM chat_rooms WHERE id = ?", chatID)
	})
}

func TestListChatRoomsMTV2Path(t *testing.T) {
	req := httptest.NewRequest("GET", "/apiv2/chat/rooms", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestReviewChatMessageV2Path(t *testing.T) {
	req := httptest.NewRequest("GET", "/apiv2/chatmessages", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestReviewChatOwnGroupFirst(t *testing.T) {
	// Own-group messages should appear before wider review messages in the
	// review queue, so mods see their own groups' work first.
	prefix := uniquePrefix("ReviewOrder")
	db := database.DBConn

	ownGroupID := CreateTestGroup(t, prefix+"_own")
	widerGroupID := CreateTestGroup(t, prefix+"_wider")

	// Set wider group to have widerchatreview enabled.
	db.Exec("UPDATE `groups` SET settings = JSON_SET(COALESCE(settings, '{}'), '$.widerchatreview', 1) WHERE id = ?", widerGroupID)

	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, ownGroupID, "Moderator")
	// Mod must also be on a group with widerchatreview enabled to see wider messages.
	widerModGroupID := CreateTestGroup(t, prefix+"_wmod")
	db.Exec("UPDATE `groups` SET settings = JSON_SET(COALESCE(settings, '{}'), '$.widerchatreview', 1) WHERE id = ?", widerModGroupID)
	CreateTestMembership(t, modID, widerModGroupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Create a wider review message (recipient on wider group, not mod's group).
	widerSender := CreateTestUser(t, prefix+"_wsender", "User")
	CreateTestMembership(t, widerSender, widerGroupID, "Member")
	widerRecipient := CreateTestUser(t, prefix+"_wrecip", "User")
	CreateTestMembership(t, widerRecipient, widerGroupID, "Member")
	widerChatID := CreateTestChatRoom(t, widerSender, &widerRecipient, nil, "User2User")
	widerMsgID := CreateTestChatMessage(t, widerChatID, widerSender, "Wider spam")
	db.Exec("UPDATE chat_messages SET processingsuccessful = 1, reviewrequired = 1, reviewrejected = 0, reportreason = 'Spam' WHERE id = ?", widerMsgID)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", widerChatID)

	// Create an own-group message (recipient on mod's group).
	ownSender := CreateTestUser(t, prefix+"_osender", "User")
	CreateTestMembership(t, ownSender, ownGroupID, "Member")
	ownRecipient := CreateTestUser(t, prefix+"_orecip", "User")
	CreateTestMembership(t, ownRecipient, ownGroupID, "Member")
	ownChatID := CreateTestChatRoom(t, ownSender, &ownRecipient, nil, "User2User")
	ownMsgID := CreateTestChatMessage(t, ownChatID, ownSender, "Own group spam")
	db.Exec("UPDATE chat_messages SET processingsuccessful = 1, reviewrequired = 1, reviewrejected = 0, reportreason = 'Spam' WHERE id = ?", ownMsgID)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", ownChatID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?limit=10&jwt=%s", modToken), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	msgs := result["chatmessages"].([]interface{})

	// Find positions of own-group and wider messages.
	ownIdx := -1
	widerIdx := -1
	for i, m := range msgs {
		msg := m.(map[string]interface{})
		id := uint64(msg["id"].(float64))
		if id == ownMsgID {
			ownIdx = i
		}
		if id == widerMsgID {
			widerIdx = i
		}
	}

	assert.GreaterOrEqual(t, ownIdx, 0, "Should find own-group message")
	assert.GreaterOrEqual(t, widerIdx, 0, "Should find wider message")
	assert.Less(t, ownIdx, widerIdx, "Own-group message should appear before wider review message")
}

func TestReviewChatMessagesNoDuplicates(t *testing.T) {
	// The review queue should return each message exactly once, even when the
	// recipient is a member of multiple groups that the mod moderates.
	prefix := uniquePrefix("ReviewDedup")
	db := database.DBConn

	group1ID := CreateTestGroup(t, prefix+"_g1")
	group2ID := CreateTestGroup(t, prefix+"_g2")

	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, group1ID, "Moderator")
	CreateTestMembership(t, modID, group2ID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Sender on group1, recipient on BOTH groups → query could produce duplicates.
	senderID := CreateTestUser(t, prefix+"_sender", "User")
	CreateTestMembership(t, senderID, group1ID, "Member")
	recipientID := CreateTestUser(t, prefix+"_recipient", "User")
	CreateTestMembership(t, recipientID, group1ID, "Member")
	CreateTestMembership(t, recipientID, group2ID, "Member")

	chatID := CreateTestChatRoom(t, senderID, &recipientID, nil, "User2User")
	msgID := CreateTestChatMessage(t, chatID, senderID, "Possible spam")
	db.Exec("UPDATE chat_messages SET processingsuccessful = 1, reviewrequired = 1, reviewrejected = 0, reportreason = 'Spam' WHERE id = ?", msgID)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", chatID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?limit=10&jwt=%s", modToken), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	msgs := result["chatmessages"].([]interface{})

	// Count how many times our message appears — should be exactly 1.
	count := 0
	for _, m := range msgs {
		msg := m.(map[string]interface{})
		if uint64(msg["id"].(float64)) == msgID {
			count++
		}
	}
	assert.Equal(t, 1, count, "Review queue should return each message exactly once, not duplicated across groups")
}

func TestModReplyToUser2ModChat(t *testing.T) {
	// A moderator who is NOT user1/user2 on a User2Mod chat should be able
	// to send a reply if they moderate the chat's group.
	prefix := uniquePrefix("ModReplyU2M")
	modID, _, _, chatID, token := setupModChatData(t, prefix)

	// Send a message as the mod to the User2Mod chat
	payload := `{"message":"Mod reply to user"}`
	req := httptest.NewRequest("POST", fmt.Sprintf("/apiv2/chat/%d/message?jwt=%s", chatID, token),
		bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	assert.Equal(t, 200, resp.StatusCode, "Mod should be able to reply to User2Mod chat they moderate")
	assert.NotNil(t, result["id"], "Should return new message id")

	// Verify the message was actually created
	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM chat_messages WHERE chatid = ? AND userid = ? AND message = 'Mod reply to user'",
		chatID, modID).Scan(&count)
	assert.Equal(t, int64(1), count, "Message should exist in DB")
}

func TestModReplyToUser2ModChatNotMod(t *testing.T) {
	// A non-moderator who is NOT user1/user2 should NOT be able to send messages.
	prefix := uniquePrefix("ModReplyNon")
	_, _, _, chatID, _ := setupModChatData(t, prefix)

	otherID := CreateTestUser(t, prefix+"_other", "User")
	_, otherToken := CreateTestSession(t, otherID)

	payload := `{"message":"I am not a mod"}`
	req := httptest.NewRequest("POST", fmt.Sprintf("/apiv2/chat/%d/message?jwt=%s", chatID, otherToken),
		bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)

	assert.Equal(t, 404, resp.StatusCode, "Non-mod non-participant should not be able to send to User2Mod chat")
}

func TestGetChatMessagesModAccess(t *testing.T) {
	// A moderator who is NOT a participant in a User2Mod chat should still
	// be able to read messages if they moderate the chat's group.
	prefix := uniquePrefix("ChatMsgMod")
	groupID := CreateTestGroup(t, prefix)

	// Create a regular user who starts a User2Mod chat.
	db := database.DBConn
	memberID := CreateTestUser(t, prefix+"_member", "User")
	CreateTestMembership(t, memberID, groupID, "Member")
	chatID := CreateTestChatRoom(t, memberID, nil, &groupID, "User2Mod")
	msgID := CreateTestChatMessage(t, chatID, memberID, "Help please")
	db.Exec("UPDATE chat_messages SET processingsuccessful = 1, reviewrequired = 0, reviewrejected = 0 WHERE id = ?", msgID)

	// Create a moderator who is NOT user1 or user2 of this chat.
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// The mod should be able to read messages via the /chat/:id/message endpoint.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/%d/message?jwt=%s", chatID, modToken), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var messages []map[string]interface{}
	json2.Unmarshal(rsp(resp), &messages)
	assert.GreaterOrEqual(t, len(messages), 1)

	// A random non-mod user should NOT be able to read.
	otherID := CreateTestUser(t, prefix+"_other", "User")
	_, otherToken := CreateTestSession(t, otherID)
	req2 := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/%d/message?jwt=%s", chatID, otherToken), nil)
	resp2, _ := getApp().Test(req2)
	assert.Equal(t, 404, resp2.StatusCode)
}

func TestGetChatMessagesAdminAccess(t *testing.T) {
	// An admin/support user should be able to read any chat's messages.
	prefix := uniquePrefix("ChatMsgAdmin")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	memberID := CreateTestUser(t, prefix+"_member", "User")
	CreateTestMembership(t, memberID, groupID, "Member")
	chatID := CreateTestChatRoom(t, memberID, nil, &groupID, "User2Mod")
	msgID := CreateTestChatMessage(t, chatID, memberID, "Help please")
	db.Exec("UPDATE chat_messages SET processingsuccessful = 1, reviewrequired = 0, reviewrejected = 0 WHERE id = ?", msgID)

	// Create an admin user (not on this group).
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/%d/message?jwt=%s", chatID, adminToken), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var messages []map[string]interface{}
	json2.Unmarshal(rsp(resp), &messages)
	assert.GreaterOrEqual(t, len(messages), 1)
}

func TestModSeesReviewMessagesInChat(t *testing.T) {
	// when a moderator views a User2User chat via /chat/:id/message,
	// messages held for review (reviewrequired=1) should be visible.
	// Non-mod participants should NOT see other users' review messages.
	prefix := uniquePrefix("ChatRevVis")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	// Create two regular users in the same group with a User2User chat.
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	CreateTestMembership(t, user1ID, groupID, "Member")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	CreateTestMembership(t, user2ID, groupID, "Member")
	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")

	// Create a normal approved message.
	normalMsgID := CreateTestChatMessage(t, chatID, user1ID, "Hello there")
	db.Exec("UPDATE chat_messages SET processingsuccessful = 1, reviewrequired = 0, reviewrejected = 0 WHERE id = ?", normalMsgID)

	// Create a message held for review.
	reviewMsgID := CreateTestChatMessage(t, chatID, user2ID, "Suspicious message")
	db.Exec("UPDATE chat_messages SET processingsuccessful = 1, reviewrequired = 1, reviewrejected = 0, reportreason = 'Spam' WHERE id = ?", reviewMsgID)

	// Create a moderator for the group (NOT a participant in this chat).
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Mod should see BOTH messages (normal + review).
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/%d/message?jwt=%s", chatID, modToken), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var modMessages []map[string]interface{}
	json2.Unmarshal(rsp(resp), &modMessages)
	assert.Equal(t, 2, len(modMessages), "Mod should see both normal and review messages")

	// Verify the review message has reviewrequired=true for the mod.
	foundReview := false
	for _, m := range modMessages {
		if uint64(m["id"].(float64)) == reviewMsgID {
			foundReview = true
			assert.Equal(t, true, m["reviewrequired"], "Mod should see reviewrequired=true")
		}
	}
	assert.True(t, foundReview, "Mod should see the review message")

	// User1 (participant, not the sender of the review message) should NOT see it.
	_, user1Token := CreateTestSession(t, user1ID)
	req2 := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/%d/message?jwt=%s", chatID, user1Token), nil)
	resp2, _ := getApp().Test(req2)
	assert.Equal(t, 200, resp2.StatusCode)

	var user1Messages []map[string]interface{}
	json2.Unmarshal(rsp(resp2), &user1Messages)
	assert.Equal(t, 1, len(user1Messages), "User1 should only see the normal message, not the review message")

	// User2 (participant AND sender of the review message) SHOULD see it.
	_, user2Token := CreateTestSession(t, user2ID)
	req3 := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/%d/message?jwt=%s", chatID, user2Token), nil)
	resp3, _ := getApp().Test(req3)
	assert.Equal(t, 200, resp3.StatusCode)

	var user2Messages []map[string]interface{}
	json2.Unmarshal(rsp(resp3), &user2Messages)
	assert.Equal(t, 2, len(user2Messages), "User2 should see both messages (they sent the review message)")

	// Verify review fields are stripped for non-mod users.
	for _, m := range user2Messages {
		assert.Equal(t, false, m["reviewrequired"], "Non-mod should not see reviewrequired=true")
	}
}

func TestModSeesReviewMessagesInUser2ModChat(t *testing.T) {
	// mods see review messages in User2Mod chats too.
	prefix := uniquePrefix("ChatRevU2M")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	memberID := CreateTestUser(t, prefix+"_member", "User")
	CreateTestMembership(t, memberID, groupID, "Member")
	chatID := CreateTestChatRoom(t, memberID, nil, &groupID, "User2Mod")

	// Normal message from member.
	normalMsgID := CreateTestChatMessage(t, chatID, memberID, "Help me please")
	db.Exec("UPDATE chat_messages SET processingsuccessful = 1, reviewrequired = 0, reviewrejected = 0 WHERE id = ?", normalMsgID)

	// Review message from member.
	reviewMsgID := CreateTestChatMessage(t, chatID, memberID, "Check this link out")
	db.Exec("UPDATE chat_messages SET processingsuccessful = 1, reviewrequired = 1, reviewrejected = 0, reportreason = 'Link' WHERE id = ?", reviewMsgID)

	// Mod is NOT user1 or user2 but moderates the group.
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/%d/message?jwt=%s", chatID, modToken), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var messages []map[string]interface{}
	json2.Unmarshal(rsp(resp), &messages)
	assert.Equal(t, 2, len(messages), "Mod should see both normal and review messages in User2Mod chat")

	// Verify review fields are present for mod.
	for _, m := range messages {
		if uint64(m["id"].(float64)) == reviewMsgID {
			assert.Equal(t, true, m["reviewrequired"])
		}
	}
}

func TestListChatsMTChattypesArray(t *testing.T) {
	// Test that chattypes[] array format works (how the JS client sends it).
	prefix := uniquePrefix("ChattypesArr")
	_, _, _, _, token := setupModChatData(t, prefix)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/rooms?chattypes[]=User2Mod&chattypes[]=Mod2Mod&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "chatrooms")

	chatrooms := result["chatrooms"].([]interface{})
	assert.GreaterOrEqual(t, len(chatrooms), 1)
}

func TestListChatsMTGroupidReturned(t *testing.T) {
	// Verify that groupid is returned in the chat list response.
	prefix := uniquePrefix("Groupid")
	_, _, groupID, _, token := setupModChatData(t, prefix)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/rooms?chattypes=User2Mod,Mod2Mod&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	chatrooms := result["chatrooms"].([]interface{})
	assert.GreaterOrEqual(t, len(chatrooms), 1)

	// Find our chat and check groupid.
	found := false
	for _, cr := range chatrooms {
		room := cr.(map[string]interface{})
		if room["groupid"] != nil && room["groupid"].(float64) == float64(groupID) {
			found = true
			break
		}
	}
	assert.True(t, found, "Should find a chat with the expected groupid %d", groupID)
}

func TestFetchSingleChatSnippet(t *testing.T) {
	// Create a User2Mod chat with a message, then fetch via GET /chat/:id
	// and verify snippet and lastdate are present.
	prefix := uniquePrefix("SnipMT")
	modID, _, _, chatID, token := setupModChatData(t, prefix)
	_ = modID

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/%d?jwt=%s", chatID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var chatroom map[string]interface{}
	json2.Unmarshal(rsp(resp), &chatroom)

	// snippet should be present and non-empty (setupModChatData creates messages).
	assert.Contains(t, chatroom, "snippet")
	snippet, ok := chatroom["snippet"].(string)
	assert.True(t, ok, "snippet should be a string")
	assert.NotEmpty(t, snippet, "snippet should not be empty")

	// lastdate should be present and non-nil.
	assert.Contains(t, chatroom, "lastdate")
	assert.NotNil(t, chatroom["lastdate"], "lastdate should not be nil")
}

func TestListChatsMTMod2ModName(t *testing.T) {
	// Verify Mod2Mod chats get the correct "GroupName Mods" name.
	prefix := uniquePrefix("M2MName")
	mod1ID := CreateTestUser(t, prefix+"_mod1", "Moderator")
	mod2ID := CreateTestUser(t, prefix+"_mod2", "Moderator")
	groupID := CreateTestGroup(t, prefix+"_group")
	CreateTestMembership(t, mod1ID, groupID, "Moderator")
	CreateTestMembership(t, mod2ID, groupID, "Moderator")

	chatID := CreateTestChatRoom(t, mod1ID, &mod2ID, &groupID, "Mod2Mod")

	db := database.DBConn
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Mod chat message', NOW(), 1, 0, 0)",
		chatID, mod1ID)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", chatID)

	_, token := CreateTestSession(t, mod2ID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/rooms?chattypes=Mod2Mod&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	chatrooms := result["chatrooms"].([]interface{})

	// Find the Mod2Mod chat.
	found := false
	for _, cr := range chatrooms {
		room := cr.(map[string]interface{})
		if room["id"].(float64) == float64(chatID) {
			found = true
			name := room["name"].(string)
			assert.Contains(t, name, "Mods", "Mod2Mod chat name should contain 'Mods'")
			break
		}
	}
	assert.True(t, found, "Should find the Mod2Mod chat")
}

func TestListChatsMTUser2ModSnippet(t *testing.T) {
	// Verify User2Mod chats return a snippet from the latest message.
	prefix := uniquePrefix("U2MSnippet")
	_, _, _, _, token := setupModChatData(t, prefix)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/rooms?chattypes=User2Mod&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	chatrooms := result["chatrooms"].([]interface{})
	assert.GreaterOrEqual(t, len(chatrooms), 1)

	// At least one chat should have a snippet from the test messages.
	hasSnippet := false
	for _, cr := range chatrooms {
		room := cr.(map[string]interface{})
		if snippet, ok := room["snippet"]; ok && snippet != nil && snippet.(string) != "" {
			hasSnippet = true
			break
		}
	}
	assert.True(t, hasSnippet, "At least one chat should have a snippet")
}

func TestCompletedSnippetOfferNoMessage(t *testing.T) {
	// When a Completed chat message references an Offer and has no message text,
	// the snippet should say "Item is no longer available" (not "Item marked as TAKEN").
	prefix := uniquePrefix("CompSnip")
	user1ID := CreateTestUser(t, prefix+"_u1", "User1")
	user2ID := CreateTestUser(t, prefix+"_u2", "User2")
	groupID := CreateTestGroup(t, prefix+"_grp")
	CreateTestMembership(t, user1ID, groupID, "Member")
	CreateTestMembership(t, user2ID, groupID, "Member")

	// Create an Offer message using the test helper.
	msgID := CreateTestMessage(t, user1ID, groupID, prefix+" Offer item", 51.5, -0.1)

	// Create User2User chat with a Completed message (no text) referencing the Offer.
	db := database.DBConn
	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, type, refmsgid, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, NULL, 'Completed', ?, NOW(), 1, 0, 0)",
		chatID, user1ID, msgID)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", chatID)

	// Fetch chats as user2 and check the snippet.
	_, token := CreateTestSession(t, user2ID)
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/rooms?chattypes=User2User&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	chatrooms := result["chatrooms"].([]interface{})

	found := false
	for _, cr := range chatrooms {
		room := cr.(map[string]interface{})
		if uint64(room["id"].(float64)) == chatID {
			assert.Equal(t, "Item is no longer available", room["snippet"])
			found = true
			break
		}
	}
	assert.True(t, found, "Should find the chat with Completed snippet")
}

func TestCompletedSnippetWithMessage(t *testing.T) {
	// When a Completed message has user-provided text, snippet should show that text.
	prefix := uniquePrefix("CompSnipMsg")
	user1ID := CreateTestUser(t, prefix+"_u1", "User1")
	user2ID := CreateTestUser(t, prefix+"_u2", "User2")
	groupID := CreateTestGroup(t, prefix+"_grp")
	CreateTestMembership(t, user1ID, groupID, "Member")
	CreateTestMembership(t, user2ID, groupID, "Member")

	msgID := CreateTestMessage(t, user1ID, groupID, prefix+" Offer item", 51.5, -0.1)

	db := database.DBConn
	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, type, refmsgid, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Sorry, gone to someone else', 'Completed', ?, NOW(), 1, 0, 0)",
		chatID, user1ID, msgID)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", chatID)

	_, token := CreateTestSession(t, user2ID)
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/rooms?chattypes=User2User&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	chatrooms := result["chatrooms"].([]interface{})

	found := false
	for _, cr := range chatrooms {
		room := cr.(map[string]interface{})
		if uint64(room["id"].(float64)) == chatID {
			assert.Equal(t, "Sorry, gone to someone else", room["snippet"])
			found = true
			break
		}
	}
	assert.True(t, found, "Should find the chat with custom snippet")
}

func TestListChatsMTNotLoggedIn(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/chat/rooms?chattypes=User2Mod,Mod2Mod", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestListChatsMTSearchUser2Mod(t *testing.T) {
	// Verify that search works for User2Mod chats.
	prefix := uniquePrefix("SearchU2M")
	_, _, _, _, token := setupModChatData(t, prefix)

	// The setupModChatData creates messages with "Hello from user" and "Another message".
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/rooms?chattypes=User2Mod&search=Hello&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "chatrooms")

	chatrooms := result["chatrooms"].([]interface{})
	assert.GreaterOrEqual(t, len(chatrooms), 1, "Search should find the User2Mod chat with 'Hello' message")
}

func TestUnseenCountMTBackupModExcluded(t *testing.T) {
	// Backup mods (active:0 in membership settings) should NOT see unseen counts for those groups.
	prefix := uniquePrefix("BackupMod")
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	userID := CreateTestUser(t, prefix+"_user", "User")
	groupID := CreateTestGroup(t, prefix+"_group")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, userID, groupID, "Member")

	// Set the mod as a backup mod on this group (active:0).
	db := database.DBConn
	db.Exec("UPDATE memberships SET settings = ? WHERE userid = ? AND groupid = ?",
		`{"active":0}`, modID, groupID)

	chatID := CreateTestChatRoom(t, userID, nil, &groupID, "User2Mod")
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Backup test', NOW(), 1, 0, 0)",
		chatID, userID)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", chatID)

	_, token := CreateTestSession(t, modID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatrooms?count=true&chattypes=User2Mod,Mod2Mod&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	// Backup mod should have 0 unseen since their group chats are excluded.
	assert.Equal(t, float64(0), result["count"])
}

func TestUnseenCountMTActiveModIncluded(t *testing.T) {
	// Active mods (active:1 or no settings) SHOULD see unseen counts.
	prefix := uniquePrefix("ActiveMod")
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	userID := CreateTestUser(t, prefix+"_user", "User")
	groupID := CreateTestGroup(t, prefix+"_group")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, userID, groupID, "Member")

	// Set the mod as active (explicit active:1).
	db := database.DBConn
	db.Exec("UPDATE memberships SET settings = ? WHERE userid = ? AND groupid = ?",
		`{"active":1}`, modID, groupID)

	chatID := CreateTestChatRoom(t, userID, nil, &groupID, "User2Mod")
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Active test', NOW(), 1, 0, 0)",
		chatID, userID)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", chatID)

	_, token := CreateTestSession(t, modID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatrooms?count=true&chattypes=User2Mod,Mod2Mod&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	// Active mod should see unseen messages.
	assert.GreaterOrEqual(t, result["count"].(float64), float64(1))
}

func TestUnseenCountMTAllSpamChatExcluded(t *testing.T) {
	// Mod2Mod chats where all messages are invalid (all spam) should be excluded.
	prefix := uniquePrefix("AllSpamMod")
	mod1ID := CreateTestUser(t, prefix+"_mod1", "Moderator")
	mod2ID := CreateTestUser(t, prefix+"_mod2", "Moderator")
	groupID := CreateTestGroup(t, prefix+"_group")
	CreateTestMembership(t, mod1ID, groupID, "Moderator")
	CreateTestMembership(t, mod2ID, groupID, "Moderator")

	chatID := CreateTestChatRoom(t, mod1ID, &mod2ID, &groupID, "Mod2Mod")

	db := database.DBConn
	// Create a message and mark the chat as having only invalid messages.
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Spam', NOW(), 1, 0, 0)",
		chatID, mod1ID)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW(), msgvalid = 0, msginvalid = 1 WHERE id = ?", chatID)

	_, token := CreateTestSession(t, mod2ID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatrooms?count=true&chattypes=Mod2Mod&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	// All-spam chat should be excluded from count.
	assert.Equal(t, float64(0), result["count"])
}

func TestListChatsMTSearchMod2Mod(t *testing.T) {
	// Verify that search works for Mod2Mod chats.
	prefix := uniquePrefix("SearchM2M")
	mod1ID := CreateTestUser(t, prefix+"_mod1", "Moderator")
	mod2ID := CreateTestUser(t, prefix+"_mod2", "Moderator")
	groupID := CreateTestGroup(t, prefix+"_group")
	CreateTestMembership(t, mod1ID, groupID, "Moderator")
	CreateTestMembership(t, mod2ID, groupID, "Moderator")

	chatID := CreateTestChatRoom(t, mod1ID, &mod2ID, &groupID, "Mod2Mod")

	db := database.DBConn
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'UniqueModSearch123', NOW(), 1, 0, 0)",
		chatID, mod1ID)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", chatID)

	_, token := CreateTestSession(t, mod2ID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/rooms?chattypes=Mod2Mod&search=UniqueModSearch123&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	chatrooms := result["chatrooms"].([]interface{})
	assert.GreaterOrEqual(t, len(chatrooms), 1, "Search should find the Mod2Mod chat with 'UniqueModSearch123'")
}

func TestFetchUser2UserChatAsGroupMod(t *testing.T) {
	// A moderator who isn't a participant should be able to view a User2User chat
	// if either participant is a member of a group the mod moderates.
	// This matches PHP ChatRoom::canSee() behavior.
	prefix := uniquePrefix("U2UModView")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, groupID, "Moderator")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	CreateTestMembership(t, user1ID, groupID, "Member")
	CreateTestMembership(t, user2ID, groupID, "Member")

	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Test msg', NOW(), 1, 0, 0)",
		chatID, user1ID)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", chatID)

	_, modToken := CreateTestSession(t, modID)

	// Mod should be able to view this chat.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/%d?jwt=%s", chatID, modToken), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var chatroom map[string]interface{}
	json2.Unmarshal(rsp(resp), &chatroom)
	assert.Equal(t, float64(chatID), chatroom["id"])
}

func TestFetchUser2UserChatDeniedNonMod(t *testing.T) {
	// A non-moderator who isn't a participant should NOT be able to view the chat.
	prefix := uniquePrefix("U2UNonMod")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	CreateTestMembership(t, user1ID, groupID, "Member")
	CreateTestMembership(t, user2ID, groupID, "Member")

	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Test msg', NOW(), 1, 0, 0)",
		chatID, user1ID)

	// Create a non-mod user on the same group.
	otherID := CreateTestUser(t, prefix+"_other", "User")
	CreateTestMembership(t, otherID, groupID, "Member")
	_, otherToken := CreateTestSession(t, otherID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/%d?jwt=%s", chatID, otherToken), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestGetChatNameUser2Mod(t *testing.T) {
	// Test getChatName for User2Mod chats:
	// - When the member (user1) fetches, name should be "GroupName Volunteers"
	// - When a mod fetches, name should be "MemberName on GroupName"
	prefix := uniquePrefix("chatname_u2m")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	memberID := CreateTestUser(t, prefix+"_member", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, memberID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")

	// Create User2Mod chat room (member → group volunteers).
	chatID := CreateTestChatRoom(t, memberID, nil, &groupID, "User2Mod")
	CreateTestChatMessage(t, chatID, memberID, "Hello volunteers")

	_, memberToken := CreateTestSession(t, memberID)
	_, modToken := CreateTestSession(t, modID)

	// Look up expected values from DB.
	// The listing endpoint uses nameshort (preferred) or namefull for group part.
	var groupNameShort string
	db.Raw("SELECT nameshort FROM `groups` WHERE id = ?", groupID).Scan(&groupNameShort)
	var groupNameFull string
	db.Raw("SELECT namefull FROM `groups` WHERE id = ?", groupID).Scan(&groupNameFull)
	groupNameForChat := groupNameShort
	if groupNameForChat == "" {
		groupNameForChat = groupNameFull
	}
	var memberFullname string
	db.Raw("SELECT fullname FROM users WHERE id = ?", memberID).Scan(&memberFullname)

	// 1. Member fetches — should see "GroupName Volunteers".
	// The listing uses namefull or nameshort for the Volunteers suffix.
	resp, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/chat/%d?jwt=%s", chatID, memberToken), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var chatroom map[string]interface{}
	json2.NewDecoder(resp.Body).Decode(&chatroom)
	name := chatroom["name"].(string)
	assert.Contains(t, name, "Volunteers",
		"Member should see 'Volunteers' in chat name")

	// 2. Mod fetches — should see "MemberName (GroupName)".
	// The listing format uses parentheses, not "on".
	resp2, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/chat/%d?jwt=%s", chatID, modToken), nil))
	assert.Equal(t, 200, resp2.StatusCode)

	var chatroom2 map[string]interface{}
	json2.NewDecoder(resp2.Body).Decode(&chatroom2)
	name2 := chatroom2["name"].(string)
	assert.Contains(t, name2, memberFullname,
		"Mod should see member's name in chat name")
	assert.Contains(t, name2, groupNameForChat,
		"Mod should see group name in chat name")
}

func TestPutChatRoomUser2Mod(t *testing.T) {
	// PUT /chat/rooms with chattype=User2Mod should create a User2Mod chat.
	prefix := uniquePrefix("putchat_u2m")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	memberID := CreateTestUser(t, prefix+"_member", "User")
	CreateTestMembership(t, memberID, groupID, "Member")
	_, token := CreateTestSession(t, memberID)

	payload := map[string]interface{}{
		"chattype": "User2Mod",
		"groupid":  groupID,
	}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/chat/rooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])
	chatID := uint64(result["id"].(float64))
	assert.Greater(t, chatID, uint64(0))

	// Verify in DB: chattype and groupid.
	var chattype string
	var gid uint64
	db.Raw("SELECT chattype, COALESCE(groupid, 0) FROM chat_rooms WHERE id = ?", chatID).Row().Scan(&chattype, &gid)
	assert.Equal(t, utils.CHAT_TYPE_USER2MOD, chattype)
	assert.Equal(t, groupID, gid)

	// Verify idempotency — same PUT returns existing chat.
	s2, _ := json2.Marshal(payload)
	request2 := httptest.NewRequest("PUT", "/api/chat/rooms?jwt="+token, bytes.NewBuffer(s2))
	request2.Header.Set("Content-Type", "application/json")
	resp2, _ := getApp().Test(request2)
	assert.Equal(t, 200, resp2.StatusCode)

	var result2 map[string]interface{}
	json2.NewDecoder(resp2.Body).Decode(&result2)
	assert.Equal(t, float64(chatID), result2["id"], "Should return existing chat on re-PUT")
}

func TestPutChatRoomUser2ModAllowsNonMember(t *testing.T) {
	// any logged-in user can contact a group's volunteers,
	// even without being a member. This is intentional.
	prefix := uniquePrefix("putchat_u2m_nomem")

	groupID := CreateTestGroup(t, prefix)
	nonMemberID := CreateTestUser(t, prefix+"_nomem", "User")
	// Deliberately NOT creating a membership.
	_, token := CreateTestSession(t, nonMemberID)

	payload := map[string]interface{}{
		"chattype": "User2Mod",
		"groupid":  groupID,
	}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/chat/rooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, 200, resp.StatusCode, "Non-member should be able to contact group volunteers")
}

func TestChatIconUsesProfileSetPath(t *testing.T) {
	// Verify that chat listing icons use ProfileSetPath (via buildUserIcon) rather than
	// constructing raw uimg_ URLs directly. This ensures chat icons match user profile URLs.
	db := database.DBConn
	prefix := uniquePrefix("chaticon")

	// Create two users.
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")

	// Give user2 a profile image with a freegletusd- externaluid (simulates Uploadcare).
	// This triggers the delivery-service URL path in ProfileSetPath.
	fakeExternalUID := "freegletusd-abc123testimage"
	fakeMods := `{"rotate":90}`
	db.Exec("INSERT INTO users_images (userid, contenttype, externaluid, externalmods) VALUES (?, 'image/jpeg', ?, ?)",
		user2ID, fakeExternalUID, fakeMods)

	// Ensure user2's settings have useprofile=1 (default, but be explicit).
	db.Exec("UPDATE users SET settings = JSON_SET(COALESCE(settings, '{}'), '$.useprofile', 1) WHERE id = ?", user2ID)

	// Create a User2User chat room between user1 and user2 with a message so it appears in listing.
	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	CreateTestChatMessage(t, chatID, user1ID, "Hello from user1")

	// Get a session for user1 (user1 is "me", so the other user is user2 — icon should be user2's).
	_, token := CreateTestSession(t, user1ID)

	// Fetch chat listing via the standard endpoint.
	resp, err := getApp().Test(httptest.NewRequest("GET", "/api/chat?jwt="+token, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var chats []chat.ChatRoomListEntry
	json2.Unmarshal(rsp(resp), &chats)

	// Find our chat in the listing.
	var foundChat *chat.ChatRoomListEntry
	for i, c := range chats {
		if c.ID == chatID {
			foundChat = &chats[i]
			break
		}
	}
	assert.NotNil(t, foundChat, "Should find our chat in listing")

	// The icon should NOT be a raw uimg_ URL — it should use the delivery service.
	assert.NotEmpty(t, foundChat.Icon, "Chat icon should not be empty")
	assert.NotContains(t, foundChat.Icon, "uimg_",
		"Chat icon should NOT use raw uimg_ URL; should use ProfileSetPath delivery URL")

	// The icon should contain the delivery service pattern.
	// ProfileSetPath for freegletusd- UIDs calls GetImageDeliveryUrl which produces a URL like:
	//   https://delivery.ilovefreegle.org?url=https://uploads.ilovefreegle.org:8080/abc123testimage&ro=90
	// (or uses IMAGE_DELIVERY / UPLOADS env vars if set)
	assert.Contains(t, foundChat.Icon, "abc123testimage",
		"Chat icon should contain the external UID (minus freegletusd- prefix)")

	// Now fetch the user profile via the user API and verify the icon matches.
	userResp, err := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/user/%d?jwt=%s", user2ID, token), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, userResp.StatusCode)

	var u user.User
	json2.Unmarshal(rsp(userResp), &u)

	// The user profile's paththumb should match the chat icon exactly.
	assert.NotEmpty(t, u.Profile.Paththumb, "User profile paththumb should not be empty")
	assert.Equal(t, u.Profile.Paththumb, foundChat.Icon,
		"Chat icon should match user.profile.paththumb from ProfileSetPath")
}

func TestListForUserFindsUser2UserAsUser2(t *testing.T) {
	// Reproduce the Playwright failure: User A posts, User B replies (creates
	// a User2User chat where B=user1, A=user2). Then User A lists chats and
	// should see the chat.
	prefix := uniquePrefix("ListU2U_u2")
	db := database.DBConn

	userA := CreateTestUser(t, prefix+"_userA", "User")
	userB := CreateTestUser(t, prefix+"_userB", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userA, groupID, "Member")
	CreateTestMembership(t, userB, groupID, "Member")

	// User B creates a User2User chat with User A (B=user1, A=user2).
	chatID := CreateTestChatRoom(t, userB, &userA, nil, "User2User")
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'I would love this item!', NOW(), 1, 0, 0)",
		chatID, userB)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", chatID)

	// User A lists chats via the API.
	_, tokenA := CreateTestSession(t, userA)
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat?includeClosed=true&jwt=%s", tokenA), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var chats []map[string]interface{}
	json2.Unmarshal(rsp(resp), &chats)

	// User A should see the chat.
	found := false
	for _, c := range chats {
		if uint64(c["id"].(float64)) == chatID {
			found = true
			break
		}
	}
	assert.True(t, found, "User A (user2) should see chat %d in listing, got %d chats", chatID, len(chats))
}

func TestChatSearchReturnsSearchFlag(t *testing.T) {
	// Verify that chats found via message content search have search=true
	// in the API response, so the frontend can distinguish them.
	prefix := uniquePrefix("SearchFlag")
	db := database.DBConn

	userA := CreateTestUser(t, prefix+"_userA", "User")
	userB := CreateTestUser(t, prefix+"_userB", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userA, groupID, "Member")
	CreateTestMembership(t, userB, groupID, "Member")

	chatID := CreateTestChatRoom(t, userA, &userB, nil, "User2User")
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'I have a wonderful xylophone for you', NOW(), 1, 0, 0)",
		chatID, userA)
	// Set latestmessage to 60 days ago so the chat only appears via the search
	// UNION branch (not the regular time-windowed branch). This ensures
	// GROUP BY picks the search row with search=1.
	db.Exec("UPDATE chat_rooms SET latestmessage = DATE_SUB(NOW(), INTERVAL 60 DAY) WHERE id = ?", chatID)

	_, tokenA := CreateTestSession(t, userA)

	// Search for "xylophone" — should find the old chat with search=true.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat?includeClosed=true&search=xylophone&jwt=%s", tokenA), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var chats []map[string]interface{}
	json2.Unmarshal(rsp(resp), &chats)

	found := false
	for _, c := range chats {
		if uint64(c["id"].(float64)) == chatID {
			found = true
			assert.Equal(t, true, c["search"], "Chat should have search=true")
			snippet, _ := c["snippet"].(string)
			assert.Contains(t, snippet, "xylophone", "Snippet should mention the search term")
			break
		}
	}
	assert.True(t, found, "Should find chat %d in search results", chatID)
}

func TestGetOrCreateUser2ModChat(t *testing.T) {
	prefix := uniquePrefix("U2MCreate")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	groupID := CreateTestGroup(t, prefix+"_group")
	CreateTestMembership(t, userID, groupID, "Member")

	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, groupID, "Moderator")

	// First call should create.
	chatID1, err := chat.GetOrCreateUser2ModChat(db, userID, groupID)
	assert.NoError(t, err)
	assert.Greater(t, chatID1, uint64(0))

	// Second call should return the same ID.
	chatID2, err := chat.GetOrCreateUser2ModChat(db, userID, groupID)
	assert.NoError(t, err)
	assert.Equal(t, chatID1, chatID2, "Should return existing chat, not create duplicate")

	// Verify only one row exists.
	var count int64
	db.Raw("SELECT COUNT(*) FROM chat_rooms WHERE user1 = ? AND groupid = ? AND chattype = 'User2Mod'",
		userID, groupID).Scan(&count)
	assert.Equal(t, int64(1), count, "Should have exactly one User2Mod chat")

	// Verify roster entries exist for user and mod.
	var userRoster, modRoster int64
	db.Raw("SELECT COUNT(*) FROM chat_roster WHERE chatid = ? AND userid = ?", chatID1, userID).Scan(&userRoster)
	db.Raw("SELECT COUNT(*) FROM chat_roster WHERE chatid = ? AND userid = ?", chatID1, modID).Scan(&modRoster)
	assert.Equal(t, int64(1), userRoster, "User should be in roster")
	assert.Equal(t, int64(1), modRoster, "Mod should be in roster")

	// Clean up.
	db.Exec("DELETE FROM chat_roster WHERE chatid = ?", chatID1)
	db.Exec("DELETE FROM chat_rooms WHERE id = ?", chatID1)
}

func TestGetOrCreateUser2ModChatAddsRosterForExistingChat(t *testing.T) {
	prefix := uniquePrefix("U2MRoster")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	groupID := CreateTestGroup(t, prefix+"_group")
	CreateTestMembership(t, userID, groupID, "Member")

	// Create the chat before any mods exist on the group.
	chatID1, err := chat.GetOrCreateUser2ModChat(db, userID, groupID)
	assert.NoError(t, err)

	// Now add a moderator and call again — should add them to roster.
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, groupID, "Moderator")

	chatID2, err := chat.GetOrCreateUser2ModChat(db, userID, groupID)
	assert.NoError(t, err)
	assert.Equal(t, chatID1, chatID2, "Should return existing chat")

	// Verify both user and new mod are in the roster.
	var userRoster, modRoster int64
	db.Raw("SELECT COUNT(*) FROM chat_roster WHERE chatid = ? AND userid = ?", chatID1, userID).Scan(&userRoster)
	db.Raw("SELECT COUNT(*) FROM chat_roster WHERE chatid = ? AND userid = ?", chatID1, modID).Scan(&modRoster)
	assert.Equal(t, int64(1), userRoster, "User should be in roster")
	assert.Equal(t, int64(1), modRoster, "New mod should be added to roster on second call")

	// Clean up.
	db.Exec("DELETE FROM chat_roster WHERE chatid = ?", chatID1)
	db.Exec("DELETE FROM chat_rooms WHERE id = ?", chatID1)
}

func TestUnseenCountExcludesOldMessages(t *testing.T) {
	// V1 parity: unseen count should only include messages from the last
	// CHAT_ACTIVE_LIMIT (31) days. Older unseen messages should not be counted.
	prefix := uniquePrefix("unseenOld")
	db := database.DBConn

	user1ID := CreateTestUser(t, prefix+"_u1", "Member")
	_, user1Token := CreateTestSession(t, user1ID)
	user2ID := CreateTestUser(t, prefix+"_u2", "Member")

	// Create a User2User chat with a recent latestmessage so it appears in the list.
	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", chatID)

	// Add user1 to roster with no messages seen.
	db.Exec("INSERT INTO chat_roster (chatid, userid) VALUES (?, ?) ON DUPLICATE KEY UPDATE userid=userid", chatID, user1ID)

	// Insert an old message (90 days ago) from user2 — should NOT count as unseen.
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, reviewrequired, processingrequired, processingsuccessful) VALUES (?, ?, 'old message', DATE_SUB(NOW(), INTERVAL 90 DAY), 0, 0, 1)",
		chatID, user2ID)

	// Insert a recent message from user2 — should count as unseen.
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, reviewrequired, processingrequired, processingsuccessful) VALUES (?, ?, 'new message', NOW(), 0, 0, 1)",
		chatID, user2ID)

	// Fetch chat list — unseen should be 1 (only the recent message), not 2.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/chat?jwt="+user1Token+"&chattypes[]=User2User", nil))
	assert.Equal(t, 200, resp.StatusCode)
	var chats []chat.ChatRoomListEntry
	json2.Unmarshal(rsp(resp), &chats)

	found := false
	for _, c := range chats {
		if c.ID == chatID {
			found = true
			assert.Equal(t, uint64(1), c.Unseen, "Unseen should be 1 (only recent message), not 2 (old message excluded by ACTIVELIM)")
		}
	}
	assert.True(t, found, "Chat %d should appear in list", chatID)

	// Also verify via roster POST — unseen count should match.
	postBody := fmt.Sprintf(`{"id":%d}`, chatID)
	req := httptest.NewRequest("POST", "/api/chatrooms?jwt="+user1Token, bytes.NewReader([]byte(postBody)))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
	var rosterResp map[string]interface{}
	json2.Unmarshal(rsp(resp), &rosterResp)
	unseenFloat, ok := rosterResp["unseen"].(float64)
	assert.True(t, ok, "unseen should be a number")
	assert.Equal(t, float64(1), unseenFloat, "Roster POST unseen should be 1 (old message excluded)")

	// Clean up.
	db.Exec("DELETE FROM chat_messages WHERE chatid = ?", chatID)
	db.Exec("DELETE FROM chat_roster WHERE chatid = ?", chatID)
	db.Exec("DELETE FROM chat_rooms WHERE id = ?", chatID)
}
