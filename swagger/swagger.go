// Package swagger Iznik API.
//
// This package provides Swagger API documentation for the Iznik API.
// The problem is that we need to explicitly define the API operations here for them to be detected.
//
// Version: 1.0
//
// Terms Of Service:
//
//	https://www.ilovefreegle.org/terms
//
// Contact: Freegle Geeks<geeks@ilovefreegle.org> https://www.ilovefreegle.org/help
//
// License: GPL v2 https://www.gnu.org/licenses/old-licenses/gpl-2.0.en.html
//
// Schemes: http, https
// Host: api.ilovefreegle.org
// BasePath: /api
// Consumes:
// - application/json
//
// Produces:
// - application/json
//
// security:
// - BearerAuth: []
//
// SecurityDefinitions:
// BearerAuth:
//   type: apiKey
//   name: Authorization
//   in: header
//
// swagger:meta
package swagger

import (
	"github.com/freegle/iznik-server-go/abtest"
	"github.com/freegle/iznik-server-go/address"
	"github.com/freegle/iznik-server-go/admin"
	"github.com/freegle/iznik-server-go/alert"
	"github.com/freegle/iznik-server-go/amp"
	"github.com/freegle/iznik-server-go/authority"
	"github.com/freegle/iznik-server-go/changes"
	"github.com/freegle/iznik-server-go/chat"
	"github.com/freegle/iznik-server-go/clientlog"
	"github.com/freegle/iznik-server-go/comment"
	"github.com/freegle/iznik-server-go/communityevent"
	"github.com/freegle/iznik-server-go/config"
	"github.com/freegle/iznik-server-go/donations"
	"github.com/freegle/iznik-server-go/group"
	"github.com/freegle/iznik-server-go/image"
	"github.com/freegle/iznik-server-go/isochrone"
	"github.com/freegle/iznik-server-go/job"
	"github.com/freegle/iznik-server-go/location"
	"github.com/freegle/iznik-server-go/membership"
	"github.com/freegle/iznik-server-go/message"
	"github.com/freegle/iznik-server-go/microvolunteering"
	"github.com/freegle/iznik-server-go/misc"
	"github.com/freegle/iznik-server-go/modconfig"
	"github.com/freegle/iznik-server-go/newsfeed"
	"github.com/freegle/iznik-server-go/noticeboard"
	"github.com/freegle/iznik-server-go/notification"
	"github.com/freegle/iznik-server-go/shortlink"
	"github.com/freegle/iznik-server-go/src"
	"github.com/freegle/iznik-server-go/stdmsg"
	"github.com/freegle/iznik-server-go/story"
	"github.com/freegle/iznik-server-go/systemlogs"
	"github.com/freegle/iznik-server-go/team"
	"github.com/freegle/iznik-server-go/tryst"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/visualise"
	"github.com/freegle/iznik-server-go/volunteering"
	"github.com/gofiber/fiber/v2"
)

// NOTE: The following structs and methods are not meant to be implemented,
// they exist solely for Swagger to generate documentation.

// ============================================================================
// A/B Test
// ============================================================================

// swagger:route GET /abtest abtest getABTest
// Get A/B test variant
//
// Returns the best-performing variant for a test UID using epsilon-greedy bandit
//
// Parameters:
//   + name: uid
//     in: query
//     description: Test UID
//     required: true
//     type: string
//
// Responses:
//
//	200: abTestResponse

// abTestResponse is the response for A/B test variant
// swagger:response abTestResponse
type abTestResponse struct {
	// A/B test variant
	// in:body
	Body abtest.ABTestVariant
}

// swagger:route POST /abtest abtest postABTest
// Track A/B test event
//
// Records a shown or action event for a variant
//
// Responses:
//
//	200: successResponse
//	400: errorResponse

// swagger:parameters postABTest
type postABTestParams struct {
	// A/B test event
	// in: body
	// required: true
	Body abtest.PostABTestRequest `json:"body"`
}

// ============================================================================
// Activity
// ============================================================================

// swagger:route GET /activity message getActivity
// Get recent activity
//
// Returns the most recent activity in groups
//
// responses:
//
//	200: activityResponse
//
// activityResponse is the response for the activity endpoint
// swagger:response activityResponse
type activityResponse struct {
	// Activity data
	// in:body
	Body []message.Activity
}

// ============================================================================
// Address
// ============================================================================

// swagger:route GET /address address listAddresses
// List addresses for user
//
// Returns all addresses for the authenticated user
//
// security:
// - BearerAuth: []
//
// responses:
//
//	200: addressesResponse
//
// addressesResponse is the response for the addresses endpoint
// swagger:response addressesResponse
type addressesResponse struct {
	// List of addresses
	// in:body
	Body []address.Address
}

// swagger:route GET /address/{id} address getAddress
// Get address by ID
//
// Returns a single address by ID
//
// Parameters:
//   + name: id
//     in: path
//     description: Address ID
//     required: true
//     type: integer
//     format: int64
//
// Responses:
//
//	200: addressResponse
//	404: errorResponse
//
// addressResponse is the response for a single address
// swagger:response addressResponse
type addressResponse struct {
	// Address data
	// in:body
	Body address.Address
}

// swagger:route POST /address address createAddress
// Create a new address
//
// Creates a new address for the authenticated user
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PATCH /address address updateAddress
// Update an existing address
//
// Updates an address for the authenticated user
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route DELETE /address/{id} address deleteAddress
// Delete an address
//
// Deletes an address by ID
//
// Parameters:
//   + name: id
//     in: path
//     description: Address ID
//     required: true
//     type: integer
//     format: int64
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	404: errorResponse

// ============================================================================
// Admin
// ============================================================================

// swagger:route GET /modtools/admin modtools listAdmins
// List admins
//
// Returns admin records for groups moderated by the authenticated user
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: adminsResponse
//	401: errorResponse
//
// adminsResponse is the response for admin list
// swagger:response adminsResponse
type adminsResponse struct {
	// List of admin records
	// in:body
	Body []admin.Admin
}

// swagger:route GET /modtools/admin/{id} modtools getAdmin
// Get admin by ID
//
// Returns a single admin record by ID
//
// Parameters:
//   + name: id
//     in: path
//     description: Admin ID
//     required: true
//     type: integer
//     format: int64
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: adminResponse
//	404: errorResponse
//
// adminResponse is the response for a single admin record
// swagger:response adminResponse
type adminResponse struct {
	// Admin data
	// in:body
	Body admin.Admin
}

// swagger:route POST /modtools/admin modtools postAdmin
// Create admin record
//
// Creates a new admin record (mod email)
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PATCH /modtools/admin modtools patchAdmin
// Update admin record
//
// Updates an existing admin record
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route DELETE /modtools/admin modtools deleteAdmin
// Delete admin record
//
// Deletes an admin record
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// ============================================================================
// Alert
// ============================================================================

// swagger:route GET /modtools/alert modtools listAlerts
// List all alerts
//
// Returns all alerts (Admin/Support only)
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: alertsResponse
//	401: errorResponse
//
// alertsResponse is the response for alert list
// swagger:response alertsResponse
type alertsResponse struct {
	// List of alerts
	// in:body
	Body []alert.Alert
}

// swagger:route GET /modtools/alert/{id} modtools getAlert
// Get alert by ID
//
// Returns a single alert by ID (public access)
//
// Parameters:
//   + name: id
//     in: path
//     description: Alert ID
//     required: true
//     type: integer
//     format: int64
//
// Responses:
//
//	200: alertResponse
//	404: errorResponse
//
// alertResponse is the response for a single alert
// swagger:response alertResponse
type alertResponse struct {
	// Alert data
	// in:body
	Body alert.Alert
}

// swagger:route PUT /modtools/alert modtools createAlert
// Create a new alert
//
// Creates a new alert (Admin/Support only)
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse
//	401: errorResponse
//	403: errorResponse

// swagger:route POST /modtools/alert modtools recordAlert
// Record alert click
//
// Records a click on an alert tracking entry (public access)
//
// Responses:
//
//	200: successResponse

// ============================================================================
// AMP Email
// ============================================================================

// swagger:route GET /amp/chat/{id} amp getAMPChatMessages
// Get chat messages for AMP email
//
// Returns the last 5 chat messages for the earlier conversation section in AMP emails
//
// Parameters:
//   + name: id
//     in: path
//     description: Chat ID
//     required: true
//     type: integer
//     format: int64
//   + name: rt
//     in: query
//     description: Read token (HMAC)
//     required: true
//     type: string
//   + name: uid
//     in: query
//     description: User ID
//     required: true
//     type: integer
//   + name: exp
//     in: query
//     description: Token expiry timestamp
//     required: true
//     type: integer
//
// Responses:
//
//	200: ampChatResponse
//	400: errorResponse
//	403: errorResponse
//
// ampChatResponse is the response for AMP chat messages
// swagger:response ampChatResponse
type ampChatResponse struct {
	// AMP chat messages
	// in:body
	Body amp.AMPChatResponse
}

// swagger:route POST /amp/chat/{id}/reply amp postAMPChatReply
// Post reply from AMP email
//
// Submits an inline reply from AMP email using a one-time write token
//
// Parameters:
//   + name: id
//     in: path
//     description: Chat ID
//     required: true
//     type: integer
//     format: int64
//   + name: wt
//     in: query
//     description: Write token (one-time nonce)
//     required: true
//     type: string
//
// Responses:
//
//	200: ampReplyResponse
//	400: errorResponse
//	403: errorResponse
//
// ampReplyResponse is the response for AMP reply
// swagger:response ampReplyResponse
type ampReplyResponse struct {
	// Reply response
	// in:body
	Body amp.ReplyResponse
}

// ============================================================================
// Authority
// ============================================================================

// swagger:route GET /authority authority searchAuthorities
// Search authorities
//
// Searches authorities by name
//
// Parameters:
//   + name: search
//     in: query
//     description: Search term
//     required: true
//     type: string
//   + name: limit
//     in: query
//     description: Maximum results (default 10)
//     required: false
//     type: integer
//
// Responses:
//
//	200: authoritySearchResponse
//
// authoritySearchResponse is the response for authority search
// swagger:response authoritySearchResponse
type authoritySearchResponse struct {
	// Search results
	// in:body
	Body []authority.SearchResult
}

// swagger:route GET /authority/{id} authority getAuthority
// Get authority by ID
//
// Returns a single authority by ID with polygon, centre, and overlapping groups
//
// Parameters:
//   + name: id
//     in: path
//     description: Authority ID
//     required: true
//     type: integer
//     format: int64
//
// Responses:
//
//	200: authorityResponse
//	404: errorResponse
//
// authorityResponse is the response for a single authority
// swagger:response authorityResponse
type authorityResponse struct {
	// Authority data
	// in:body
	Body authority.Authority
}

// swagger:route GET /authority/{id}/message authority getAuthorityMessages
// Get messages for authority
//
// Returns messages for a specific authority
//
// Parameters:
//   + name: id
//     in: path
//     description: Authority ID
//     required: true
//     type: integer
//     format: int64
//
// Responses:
//
//	200: messagesResponse

// ============================================================================
// Chat
// ============================================================================

// swagger:route GET /chat chat listChats
// List chats for user
//
// Returns all chats for the authenticated user
//
// security:
// - BearerAuth: []
//
// responses:
//
//	200: chatsResponse
//
// chatsResponse is the response for the chats endpoint
// swagger:response chatsResponse
type chatsResponse struct {
	// List of chats
	// in:body
	Body []chat.ChatRoom
}

// swagger:route GET /chat/{id} chat getChat
// Get chat by ID
//
// Returns a single chat by ID
//
// Parameters:
//   + name: id
//     in: path
//     description: Chat ID
//     required: true
//     type: integer
//     format: int64
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: chatResponse
//	404: errorResponse
//
// chatResponse is the response for a single chat
// swagger:response chatResponse
type chatResponse struct {
	// Chat data
	// in:body
	Body chat.ChatRoom
}

// swagger:route GET /chat/{id}/message chat getChatMessages
// Get chat messages
//
// Returns all messages for a specific chat room
//
// Parameters:
//   + name: id
//     in: path
//     description: Chat Room ID
//     required: true
//     type: integer
//     format: int64
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: chatMessagesResponse
//	400: errorResponse
//	401: errorResponse
//	404: errorResponse
//
// chatMessagesResponse is the response for chat messages
// swagger:response chatMessagesResponse
type chatMessagesResponse struct {
	// List of chat messages
	// in:body
	Body []chat.ChatMessage
}

// swagger:route POST /chat/{id}/message chat createChatMessage
// Create chat message
//
// Creates a new message in a chat room
//
// Parameters:
//   + name: id
//     in: path
//     description: Chat Room ID
//     required: true
//     type: integer
//     format: int64
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: chatMessageCreatedResponse
//	400: errorResponse
//	401: errorResponse

// swagger:parameters createChatMessage
type createChatMessageParams struct {
	// Chat message to create
	// in: body
	// required: true
	Body chat.ChatMessage `json:"body"`
}

// chatMessageCreatedResponse is the response for creating a chat message
// swagger:response chatMessageCreatedResponse
type chatMessageCreatedResponse struct {
	// Created message ID
	// in:body
	Body struct {
		ID int64 `json:"id"`
	}
}

// swagger:route GET /chat/rooms chat listChatRoomsMT
// List chat rooms for moderator
//
// Returns chat rooms wrapped in {chatrooms: [...]} for ModTools compatibility
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: chatRoomsMTResponse
//	401: errorResponse
//
// chatRoomsMTResponse is the response for moderator chat rooms
// swagger:response chatRoomsMTResponse
type chatRoomsMTResponse struct {
	// Chat rooms list
	// in:body
	Body struct {
		ChatRooms []chat.ChatRoomListEntry `json:"chatrooms"`
	}
}

// swagger:route PUT /chat/rooms chat createChatRoom
// Open or create a chat room
//
// Creates a new User2User chat room with another user, or returns existing one
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: chatRoomCreatedResponse
//	400: errorResponse
//	401: errorResponse

// swagger:parameters createChatRoom
type createChatRoomParams struct {
	// User to chat with
	// in: body
	// required: true
	Body chat.PutChatRoomRequest `json:"body"`
}

// chatRoomCreatedResponse is the response for creating/opening a chat room
// swagger:response chatRoomCreatedResponse
type chatRoomCreatedResponse struct {
	// in:body
	Body struct {
		Ret    int    `json:"ret"`
		Status string `json:"status"`
		ID     uint64 `json:"id"`
	}
}

// swagger:route PATCH /chatmessages chat patchChatMessage
// Update chat message
//
// Updates a chat message (e.g. replyexpected flag)
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:parameters patchChatMessage
type patchChatMessageParams struct {
	// Fields to update
	// in: body
	// required: true
	Body chat.PatchChatMessageRequest `json:"body"`
}

// swagger:route DELETE /chatmessages chat deleteChatMessage
// Delete chat message
//
// Soft-deletes a chat message owned by the logged-in user
//
// Parameters:
//   + name: id
//     in: query
//     description: Chat Message ID
//     required: true
//     type: integer
//     format: int64
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// swagger:route GET /chatrooms chat getChatRoomsMT
// Get chatrooms for moderator
//
// Returns unseen count, single room, or list of chat rooms for moderator view
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: chatRoomsMTResponse
//	401: errorResponse

// swagger:route POST /chatrooms chat postChatRoom
// Chatroom actions
//
// Handles roster updates, nudge messages, and typing indicators for chat rooms
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:parameters postChatRoom
type postChatRoomParams struct {
	// Action request
	// in: body
	// required: true
	Body chat.ChatRoomPostRequest `json:"body"`
}

// swagger:route GET /chatmessages chat getReviewChatMessages
// Get chat messages for review
//
// Returns review queue messages or messages from a specific chat room
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: chatMessagesResponse
//	401: errorResponse

// swagger:route POST /chatmessages chat postChatMessageModeration
// Moderate chat message
//
// Approve, reject, hold, release, or redact a chat message
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:parameters postChatMessageModeration
type postChatMessageModerationParams struct {
	// Moderation action
	// in: body
	// required: true
	Body chat.ModerationRequest `json:"body"`
}

// swagger:route POST /chat/lovejunk chat createChatMessageLoveJunk
// Create LoveJunk chat message
//
// Creates a new chat message from a LoveJunk user via partner key
//
// Responses:
//
//	200: chatMessageCreatedResponse
//	400: errorResponse

// ============================================================================
// Changes
// ============================================================================

// swagger:route GET /changes changes getChanges
// Get changes since timestamp
//
// Returns message changes (deleted, edited, promised, reneged, outcomes, approved/reposted),
// user changes, and ratings since a given time. Requires partner key authentication.
//
// Parameters:
//   + name: since
//     in: query
//     description: ISO8601 or MySQL datetime timestamp (defaults to 1 hour ago)
//     required: false
//     type: string
//   + name: partner
//     in: query
//     description: Partner API key
//     required: true
//     type: string
//
// Responses:
//
//	200: changesResponse
//	400: errorResponse
//	403: errorResponse
//
// changesResponse is the response for the changes endpoint
// swagger:response changesResponse
type changesResponse struct {
	// Changes data
	// in:body
	Body changes.ChangesResponse
}

// ============================================================================
// Client Log
// ============================================================================

// swagger:route POST /clientlog logging receiveClientLogs
// Receive client logs
//
// Accepts client-side log entries for distributed tracing
//
// Responses:
//
//	204: noContentResponse

// swagger:parameters receiveClientLogs
type receiveClientLogsParams struct {
	// Client log entries
	// in: body
	// required: true
	Body clientlog.ClientLogRequest `json:"body"`
}

// ============================================================================
// Comment
// ============================================================================

// swagger:route GET /comment comment getComments
// List or get comments
//
// Returns comments for moderated groups with pagination. Pass id for a single comment.
//
// Parameters:
//   + name: id
//     in: query
//     description: Comment ID for single fetch
//     required: false
//     type: integer
//   + name: groupid
//     in: query
//     description: Filter by group ID
//     required: false
//     type: integer
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: commentsResponse
//	401: errorResponse
//
// commentsResponse is the response for comments
// swagger:response commentsResponse
type commentsResponse struct {
	// Comments data
	// in:body
	Body []comment.CommentItem
}

// swagger:route POST /comment comment createComment
// Create a comment on a user
//
// Moderators can add comments to users in their groups
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PATCH /comment comment editComment
// Edit a comment
//
// Moderators can edit comments on users in their groups
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route DELETE /comment/{id} comment deleteComment
// Delete a comment
//
// Moderators can delete comments on users in their groups
//
// Parameters:
//   + name: id
//     in: path
//     description: Comment ID
//     required: true
//     type: integer
//     format: int64
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// ============================================================================
// Community Event
// ============================================================================

// swagger:route GET /communityevent communityevent listCommunityEvents
// List community events
//
// Returns all community events
//
// Responses:
//
//	200: communityEventsResponse
//
// communityEventsResponse is the response for community events list
// swagger:response communityEventsResponse
type communityEventsResponse struct {
	// Community events
	// in:body
	Body []communityevent.CommunityEvent
}

// swagger:route GET /communityevent/group/{id} communityevent listCommunityEventsForGroup
// List community events for group
//
// Returns all community events for a specific group
//
// Parameters:
//   + name: id
//     in: path
//     description: Group ID
//     required: true
//     type: integer
//     format: int64
//
// Responses:
//
//	200: communityEventsResponse

// swagger:route GET /communityevent/{id} communityevent getCommunityEvent
// Get community event by ID
//
// Returns a single community event by ID
//
// Parameters:
//   + name: id
//     in: path
//     description: Community Event ID
//     required: true
//     type: integer
//     format: int64
//
// Responses:
//
//	200: communityEventResponse
//	404: errorResponse
//
// communityEventResponse is the response for a single community event
// swagger:response communityEventResponse
type communityEventResponse struct {
	// Community event data
	// in:body
	Body communityevent.CommunityEvent
}

// swagger:route POST /communityevent communityevent createCommunityEvent
// Create a community event
//
// Creates a new community event
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PATCH /communityevent communityevent updateCommunityEvent
// Update a community event
//
// Updates an existing community event
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route DELETE /communityevent/{id} communityevent deleteCommunityEvent
// Delete a community event
//
// Deletes a community event by ID
//
// Parameters:
//   + name: id
//     in: path
//     description: Community Event ID
//     required: true
//     type: integer
//     format: int64
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// ============================================================================
// Config
// ============================================================================

// swagger:route GET /config/{key} config getConfig
// Get configuration
//
// Returns configuration by key
//
// Parameters:
//   + name: key
//     in: path
//     description: Configuration key
//     required: true
//     type: string
//
// Responses:
//
//	200: configResponse
//
// configResponse is the response for a single config
// swagger:response configResponse
type configResponse struct {
	// Config data
	// in:body
	Body config.ConfigItem
}

// swagger:route GET /config/admin/spam_keywords config listSpamKeywords
// List spam keywords
//
// Returns all spam keywords (Support/Admin only)
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: spamKeywordsResponse
//	401: errorResponse
//	403: errorResponse
//
// spamKeywordsResponse is the response for spam keywords
// swagger:response spamKeywordsResponse
type spamKeywordsResponse struct {
	// List of spam keywords
	// in:body
	Body []config.SpamKeyword
}

// swagger:route POST /config/admin/spam_keywords config createSpamKeyword
// Create spam keyword
//
// Creates a new spam keyword (Support/Admin only)
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: spamKeywordResponse
//	400: errorResponse
//	401: errorResponse
//	403: errorResponse

// swagger:parameters createSpamKeyword
type createSpamKeywordParams struct {
	// Spam keyword object
	// in: body
	// required: true
	Body CreateSpamKeywordRequest `json:"body"`
}
//
// spamKeywordResponse is the response for a single spam keyword
// swagger:response spamKeywordResponse
type spamKeywordResponse struct {
	// Spam keyword data
	// in:body
	Body config.SpamKeyword
}

// swagger:route DELETE /config/admin/spam_keywords/{id} config deleteSpamKeyword
// Delete spam keyword
//
// Deletes a spam keyword by ID (Support/Admin only)
//
// Parameters:
//   + name: id
//     in: path
//     description: Spam keyword ID
//     required: true
//     type: integer
//     format: int64
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse
//	403: errorResponse
//	404: errorResponse

// swagger:route GET /config/admin/worry_words config listWorryWords
// List worry words
//
// Returns all worry words (Support/Admin only)
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: worryWordsResponse
//	401: errorResponse
//	403: errorResponse
//
// worryWordsResponse is the response for worry words
// swagger:response worryWordsResponse
type worryWordsResponse struct {
	// List of worry words
	// in:body
	Body []config.WorryWord
}

// swagger:route POST /config/admin/worry_words config createWorryWord
// Create worry word
//
// Creates a new worry word (Support/Admin only)
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: worryWordResponse
//	400: errorResponse
//	401: errorResponse
//	403: errorResponse

// swagger:parameters createWorryWord
type createWorryWordParams struct {
	// Worry word object
	// in: body
	// required: true
	Body CreateWorryWordRequest `json:"body"`
}
//
// worryWordResponse is the response for a single worry word
// swagger:response worryWordResponse
type worryWordResponse struct {
	// Worry word data
	// in:body
	Body config.WorryWord
}

// swagger:route DELETE /config/admin/worry_words/{id} config deleteWorryWord
// Delete worry word
//
// Deletes a worry word by ID (Support/Admin only)
//
// Parameters:
//   + name: id
//     in: path
//     description: Worry word ID
//     required: true
//     type: integer
//     format: int64
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse
//	403: errorResponse
//	404: errorResponse

// swagger:route PATCH /config/admin config patchAdminConfig
// Update admin config keys
//
// Updates admin configuration values (Support/Admin only)
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse
//	403: errorResponse

// ============================================================================
// Dashboard
// ============================================================================

// swagger:route GET /dashboard dashboard getDashboard
// Get dashboard data
//
// Returns dashboard components for moderator/user dashboards
//
// Parameters:
//   + name: components
//     in: query
//     description: Comma-separated component names
//     required: false
//     type: string
//   + name: group
//     in: query
//     description: Group ID
//     required: false
//     type: integer
//
// Responses:
//
//	200: genericResponse

// ============================================================================
// Domains
// ============================================================================

// swagger:route GET /domains domain getDomains
// Get domains
//
// Returns domain information
//
// Responses:
//
//	200: genericResponse

// ============================================================================
// Donations
// ============================================================================

// swagger:route GET /donations donations getDonations
// Get donations
//
// Returns donation target and raised amounts for the current month
//
// Parameters:
//   + name: groupid
//     in: query
//     description: Group ID to filter donations
//     required: false
//     type: integer
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse

// swagger:route PUT /donations donations addDonation
// Record external donation
//
// Records an external bank transfer donation
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route POST /stripecreateintent donations createStripeIntent
// Create Stripe PaymentIntent
//
// Creates a Stripe PaymentIntent for a one-time donation
//
// Responses:
//
//	200: genericResponse
//	400: errorResponse

// swagger:route POST /stripecreatesubscription donations createStripeSubscription
// Create Stripe subscription
//
// Creates a Stripe subscription for recurring monthly donation
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse
//	400: errorResponse

// swagger:route GET /giftaid donations getGiftAid
// Get Gift Aid declaration
//
// Returns user's Gift Aid declaration. With all=true returns admin review list.
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: giftAidResponse
//	401: errorResponse
//
// giftAidResponse is the response for Gift Aid
// swagger:response giftAidResponse
type giftAidResponse struct {
	// Gift Aid data
	// in:body
	Body donations.GiftAid
}

// swagger:route POST /giftaid donations setGiftAid
// Set Gift Aid declaration
//
// Creates or updates the user's Gift Aid declaration
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PATCH /giftaid donations editGiftAid
// Edit Gift Aid declaration (admin)
//
// Admin edits a Gift Aid record
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse
//	403: errorResponse

// swagger:route DELETE /giftaid donations deleteGiftAid
// Delete Gift Aid declaration
//
// Soft-deletes the user's Gift Aid declaration
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// ============================================================================
// Email Tracking
// ============================================================================

// swagger:route GET /modtools/email/stats modtools getEmailStats
// Get email tracking statistics
//
// Returns aggregate email statistics for Support/Admin users
//
// Parameters:
//   + name: type
//     in: query
//     description: Email type filter
//     required: false
//     type: string
//   + name: start
//     in: query
//     description: Start date (YYYY-MM-DD)
//     required: false
//     type: string
//   + name: end
//     in: query
//     description: End date (YYYY-MM-DD)
//     required: false
//     type: string
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse
//	401: errorResponse
//	403: errorResponse

// swagger:route GET /modtools/email/stats/timeseries modtools getEmailTimeSeries
// Get daily email statistics for charting
//
// Returns daily sent/opened/clicked/bounced counts for date range
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse
//	401: errorResponse
//	403: errorResponse

// swagger:route GET /modtools/email/stats/bytype modtools getEmailStatsByType
// Get email statistics by email type
//
// Returns statistics for each email type for comparison charts
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse
//	401: errorResponse
//	403: errorResponse

// swagger:route GET /modtools/email/stats/clicks modtools getTopClickedLinks
// Get top clicked links from emails
//
// Returns the most clicked links, normalized to remove user-specific data
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse
//	401: errorResponse
//	403: errorResponse

// swagger:route GET /modtools/email/user/{id} modtools getUserEmails
// Get email tracking for a user
//
// Returns email tracking records for a specific user (Support/Admin only)
//
// Parameters:
//   + name: id
//     in: path
//     description: User ID
//     required: true
//     type: integer
//     format: int64
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse
//	401: errorResponse
//	403: errorResponse

// swagger:route GET /e/d/p/{id} delivery deliveryPixel
// Delivery pixel
//
// Returns 1x1 transparent GIF for email open tracking
//
// Parameters:
//   + name: id
//     in: path
//     description: Tracking ID
//     required: true
//     type: string
//
// Responses:
//
//	200: fileResponse

// swagger:route GET /e/d/r/{id} delivery deliveryRedirect
// Delivery redirect
//
// Handles link clicks from emails and redirects to destination URL
//
// Parameters:
//   + name: id
//     in: path
//     description: Tracking ID
//     required: true
//     type: string
//   + name: url
//     in: query
//     description: Base64 encoded destination URL
//     required: true
//     type: string
//
// Responses:
//
//	302: description:Redirect

// swagger:route GET /e/d/i/{id} delivery deliveryImage
// Delivery image
//
// Handles image loads for scroll depth tracking and redirects to original image
//
// Parameters:
//   + name: id
//     in: path
//     description: Tracking ID
//     required: true
//     type: string
//   + name: url
//     in: query
//     description: Base64 encoded image URL
//     required: true
//     type: string
//   + name: p
//     in: query
//     description: Position
//     required: true
//     type: string
//
// Responses:
//
//	302: description:Redirect

// ============================================================================
// Export
// ============================================================================

// swagger:route POST /export export postExport
// Request GDPR data export
//
// Initiates a GDPR data export for the authenticated user
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// swagger:route GET /export export getExport
// Get GDPR data export
//
// Returns the GDPR data export for the authenticated user
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse
//	401: errorResponse

// ============================================================================
// Group
// ============================================================================

// swagger:route GET /group group listGroups
// List groups
//
// Returns all groups
//
// Responses:
//
//	200: groupsResponse
//
// groupsResponse is the response for group list
// swagger:response groupsResponse
type groupsResponse struct {
	// List of groups
	// in:body
	Body []group.GroupEntry
}

// swagger:route GET /group/work group getGroupWork
// Get group work counts
//
// Returns per-group work counts for moderators
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: groupWorkResponse
//	401: errorResponse
//
// groupWorkResponse is the response for group work counts
// swagger:response groupWorkResponse
type groupWorkResponse struct {
	// Group work counts
	// in:body
	Body []group.GroupWork
}

// swagger:route GET /group/{id} group getGroup
// Get group by ID
//
// Returns a single group by ID
//
// Parameters:
//   + name: id
//     in: path
//     description: Group ID
//     required: true
//     type: integer
//     format: int64
//
// Responses:
//
//	200: groupResponse
//	404: errorResponse
//
// groupResponse is the response for a single group
// swagger:response groupResponse
type groupResponse struct {
	// Group data
	// in:body
	Body group.Group
}

// swagger:route POST /group group createGroup
// Create a new group
//
// Creates a new freegle group
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route GET /group/{id}/message group getGroupMessages
// Get messages for group
//
// Returns messages for a specific group
//
// Parameters:
//   + name: id
//     in: path
//     description: Group ID
//     required: true
//     type: integer
//     format: int64
//
// Responses:
//
//	200: messagesResponse

// swagger:route PATCH /group group patchGroup
// Update group settings
//
// Updates group fields. Requires mod/owner role or admin/support.
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse
//	403: errorResponse

// ============================================================================
// Image
// ============================================================================

// swagger:route POST /image image postImage
// Create or update image attachment
//
// Registers an externally-uploaded image (via Tus) or rotates an existing image
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse
//	400: errorResponse
//	401: errorResponse

// swagger:parameters postImage
type postImageParams struct {
	// Image request
	// in: body
	// required: true
	Body image.PostRequest `json:"body"`
}

// ============================================================================
// Isochrone
// ============================================================================

// swagger:route GET /isochrone isochrone listIsochrones
// List isochrones
//
// Returns all isochrones for the authenticated user
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: isochronesResponse
//	401: errorResponse
//
// isochronesResponse is the response for isochrone list
// swagger:response isochronesResponse
type isochronesResponse struct {
	// Isochrone data
	// in:body
	Body isochrone.Isochrones
}

// swagger:route PUT /isochrone isochrone createIsochrone
// Create an isochrone
//
// Creates a new isochrone for the authenticated user
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PATCH /isochrone isochrone editIsochrone
// Edit an isochrone
//
// Updates an existing isochrone
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route DELETE /isochrone isochrone deleteIsochrone
// Delete an isochrone
//
// Deletes an isochrone for the authenticated user
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// swagger:route GET /isochrone/message isochrone getIsochroneMessages
// Get messages for isochrone
//
// Returns messages within the user's isochrone areas
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: messagesResponse

// ============================================================================
// Job
// ============================================================================

// swagger:route GET /job job listJobs
// List jobs
//
// Returns all jobs
//
// Responses:
//
//	200: jobsResponse
//
// jobsResponse is the response for job list
// swagger:response jobsResponse
type jobsResponse struct {
	// List of jobs
	// in:body
	Body []job.Job
}

// swagger:route GET /job/{id} job getJob
// Get job by ID
//
// Returns a single job by ID
//
// Parameters:
//   + name: id
//     in: path
//     description: Job ID
//     required: true
//     type: integer
//     format: int64
//
// Responses:
//
//	200: jobResponse
//	404: errorResponse
//
// jobResponse is the response for a single job
// swagger:response jobResponse
type jobResponse struct {
	// Job data
	// in:body
	Body job.Job
}

// swagger:route POST /job job recordJobClick
// Record a job click
//
// Records when a user clicks on a job listing for analytics
//
// Parameters:
//   + name: id
//     in: query
//     description: Job ID
//     required: true
//     type: integer
//
// Responses:
//
//	200: successResponse
//	400: errorResponse

// ============================================================================
// Location
// ============================================================================

// swagger:route GET /location/latlng location getLocationByLatLng
// Get location by latitude/longitude
//
// Returns location info for given coordinates
//
// Parameters:
//   + name: lat
//     in: query
//     description: Latitude
//     required: true
//     type: number
//   + name: lng
//     in: query
//     description: Longitude
//     required: true
//     type: number
//
// Responses:
//
//	200: locationResponse
//
// locationResponse is the response for a single location
// swagger:response locationResponse
type locationResponse struct {
	// Location data
	// in:body
	Body location.Location
}

// swagger:route GET /location/typeahead location locationTypeahead
// Location typeahead search
//
// Returns location suggestions for typeahead
//
// Parameters:
//   + name: term
//     in: query
//     description: Search term
//     required: true
//     type: string
//
// Responses:
//
//	200: locationsResponse
//
// locationsResponse is the response for location list
// swagger:response locationsResponse
type locationsResponse struct {
	// Locations
	// in:body
	Body []location.Location
}

// swagger:route GET /location/{id}/addresses location getLocationAddresses
// Get addresses for location
//
// Returns addresses for a specific location
//
// Parameters:
//   + name: id
//     in: path
//     description: Location ID
//     required: true
//     type: integer
//     format: int64
//
// Responses:
//
//	200: addressesResponse

// swagger:route GET /location/{id} location getLocation
// Get location by ID
//
// Returns a single location by ID
//
// Parameters:
//   + name: id
//     in: path
//     description: Location ID
//     required: true
//     type: integer
//     format: int64
//
// Responses:
//
//	200: locationResponse
//	404: errorResponse

// swagger:route GET /locations location searchLocations
// Search locations
//
// Searches locations by lat/lng, typeahead, or bounding box
//
// Responses:
//
//	200: locationsResponse

// swagger:route PUT /locations location createLocation
// Create a location
//
// Creates a new location
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PATCH /locations location updateLocation
// Update a location
//
// Updates an existing location
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route POST /locations/kml location convertKML
// Convert KML to location
//
// Converts KML data to a location polygon
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse
//	400: errorResponse

// swagger:route POST /locations location excludeLocation
// Exclude a location
//
// Marks a location as excluded
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// ============================================================================
// Logs
// ============================================================================

// swagger:route GET /modtools/logs modtools getLogs
// Get logs
//
// Returns log entries for a user or group
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse
//	401: errorResponse

// ============================================================================
// Logo
// ============================================================================

// swagger:route GET /logo misc getLogo
// Get logo
//
// Returns logo information
//
// Responses:
//
//	200: genericResponse

// ============================================================================
// Membership
// ============================================================================

// swagger:route GET /memberships membership getMemberships
// Get memberships
//
// Returns group memberships for the authenticated user or members of a group
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: membershipsResponse
//	401: errorResponse
//
// membershipsResponse is the response for memberships
// swagger:response membershipsResponse
type membershipsResponse struct {
	// Memberships data
	// in:body
	Body []membership.GetMembershipsMember
}

// swagger:route PUT /memberships membership joinGroup
// Join a group
//
// Adds the authenticated user to a group
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route DELETE /memberships membership leaveGroup
// Leave a group
//
// Removes the authenticated user from a group
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// swagger:route PATCH /memberships membership updateMembership
// Update membership settings
//
// Updates email frequency, events allowed, volunteering allowed
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route POST /memberships membership postMemberships
// Membership moderation actions
//
// Handles member moderation: approve, reject, ban, hold, spam, delete
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse
//	403: errorResponse

// ============================================================================
// Merge
// ============================================================================

// swagger:route GET /merge merge getMerge
// Get merge requests
//
// Returns pending merge requests for moderator review
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse
//	401: errorResponse

// swagger:route PUT /merge merge createMerge
// Create merge request
//
// Creates a new merge request to combine two users
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route POST /merge merge postMerge
// Action merge request
//
// Approves or rejects a merge request
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route DELETE /merge merge deleteMerge
// Delete merge request
//
// Deletes a merge request
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// ============================================================================
// Message
// ============================================================================

// swagger:route GET /modtools/messages modtools listMessages
// List messages
//
// Returns messages with moderation queue support
//
// Responses:
//
//	200: messagesResponse

// swagger:route GET /message/count message getMessageCount
// Get message count
//
// Returns count of messages by type
//
// Responses:
//
//	200: genericResponse

// swagger:route GET /message/inbounds message getMessagesInBounds
// Get messages in bounds
//
// Returns messages within geographic bounds
//
// Parameters:
//   + name: swlat
//     in: query
//     description: Southwest latitude
//     required: true
//     type: number
//   + name: swlng
//     in: query
//     description: Southwest longitude
//     required: true
//     type: number
//   + name: nelat
//     in: query
//     description: Northeast latitude
//     required: true
//     type: number
//   + name: nelng
//     in: query
//     description: Northeast longitude
//     required: true
//     type: number
//
// Responses:
//
//	200: messagesResponse

// swagger:route GET /message/mygroups/{id} message getMessagesByGroup
// Get messages by group
//
// Returns messages for user's groups, optionally filtered by group ID
//
// Parameters:
//   + name: id
//     in: path
//     description: Group ID
//     required: true
//     type: integer
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: messagesResponse

// swagger:route GET /message/search/{term} message searchMessages
// Search messages
//
// Searches messages by term
//
// Parameters:
//   + name: term
//     in: path
//     description: Search term
//     required: true
//     type: string
//   + name: messagetype
//     in: query
//     description: Message type filter
//     required: false
//     type: string
//   + name: groupids
//     in: query
//     description: Group IDs to filter by (comma separated)
//     required: false
//     type: string
//
// Responses:
//
//	200: messagesResponse

// swagger:route GET /message/{ids} message getMessages
// Get messages by ID
//
// Returns messages by ID (comma separated)
//
// Parameters:
//   + name: ids
//     in: path
//     description: Message IDs (comma separated)
//     required: true
//     type: string
//
// Responses:
//
//	200: messagesResponse
//	404: errorResponse
//
// messagesResponse is the response for messages
// swagger:response messagesResponse
type messagesResponse struct {
	// List of messages
	// in:body
	Body []message.Message
}

// (Removed: /modtools/messages/markseen — endpoint does not exist in routes.go)
//	401: errorResponse

// swagger:route POST /message message postMessage
// Message actions
//
// Handles message actions: Promise, Renege, OutcomeIntended, Outcome, AddBy, RemoveBy, View
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PATCH /message message patchMessage
// Update message
//
// Updates message fields
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PUT /message message putMessage
// Create message
//
// Creates a new message (offer or wanted)
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route DELETE /message/{id} message deleteMessage
// Delete message
//
// Deletes a message by ID
//
// Parameters:
//   + name: id
//     in: path
//     description: Message ID
//     required: true
//     type: integer
//     format: int64
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse
//	404: errorResponse

// ============================================================================
// Microvolunteering
// ============================================================================

// swagger:route GET /microvolunteering microvolunteering getMicrovolunteeringChallenge
// Get microvolunteering challenge
//
// Returns a microvolunteering challenge for the authenticated user
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: microvolunteeringResponse
//	401: errorResponse
//
// microvolunteeringResponse is the response for a microvolunteering challenge
// swagger:response microvolunteeringResponse
type microvolunteeringResponse struct {
	// Challenge data
	// in:body
	Body microvolunteering.Challenge
}

// swagger:route POST /microvolunteering microvolunteering postMicrovolunteeringResponse
// Submit micro-volunteering response
//
// Records the user's response to a micro-volunteering challenge
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PATCH /microvolunteering microvolunteering patchMicrovolunteeringFeedback
// Provide moderator feedback on microaction
//
// Allows a moderator to set feedback, score_positive, and score_negative on a microaction
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse
//	403: errorResponse
//
// modFeedbackRequest is the request body for moderator feedback on a microaction
// swagger:parameters patchMicrovolunteeringFeedback
type modFeedbackRequest struct {
	// in:body
	Body microvolunteering.ModFeedbackRequest
}

// ============================================================================
// Misc
// ============================================================================

// swagger:route GET /online misc getOnline
// Check online status
//
// Returns online status information
//
// Responses:
//
//	200: onlineResponse
//
// onlineResponse is the response for online status
// swagger:response onlineResponse
type onlineResponse struct {
	// Online status
	// in:body
	Body misc.OnlineResult
}

// swagger:route GET /latestmessage misc getLatestMessage
// Get latest message timestamp
//
// Returns the timestamp of the latest message
//
// Responses:
//
//	200: latestMessageResponse
//
// latestMessageResponse is the response for latest message
// swagger:response latestMessageResponse
type latestMessageResponse struct {
	// Latest message data
	// in:body
	Body misc.LatestMessageResult
}

// swagger:route GET /illustration misc getIllustration
// Get AI illustration for item
//
// Returns a cached AI-generated illustration for an item name. Returns ret=3 if not cached.
//
// Parameters:
//   + name: item
//     in: query
//     description: Item name
//     required: true
//     type: string
//
// Responses:
//
//	200: illustrationResponse
//
// illustrationResponse is the response for illustration
// swagger:response illustrationResponse
type illustrationResponse struct {
	// Illustration data
	// in:body
	Body misc.IllustrationResult
}

// swagger:route POST /src misc recordSource
// Record source tracking
//
// Records source tracking data for analytics
//
// Responses:
//
//	200: successResponse

// swagger:parameters recordSource
type recordSourceParams struct {
	// Source tracking data
	// in: body
	// required: true
	Body src.SourceRequest `json:"body"`
}

// ============================================================================
// Mod Config
// ============================================================================

// swagger:route GET /modtools/modconfig modtools getModConfig
// Get moderation config
//
// Returns moderation configuration
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: modConfigResponse
//	401: errorResponse
//
// modConfigResponse is the response for mod config
// swagger:response modConfigResponse
type modConfigResponse struct {
	// Mod config data
	// in:body
	Body modconfig.ModConfig
}

// swagger:route POST /modtools/modconfig modtools createModConfig
// Create moderation config
//
// Creates a new moderation configuration
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PATCH /modtools/modconfig modtools patchModConfig
// Update moderation config
//
// Updates an existing moderation configuration
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route DELETE /modtools/modconfig modtools deleteModConfig
// Delete moderation config
//
// Deletes a moderation configuration
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// ============================================================================
// Newsfeed
// ============================================================================

// swagger:route GET /newsfeed newsfeed getNewsfeed
// Get newsfeed
//
// Returns newsfeed items
//
// Responses:
//
//	200: newsfeedResponse
//
// newsfeedResponse is the response for newsfeed
// swagger:response newsfeedResponse
type newsfeedResponse struct {
	// Newsfeed items
	// in:body
	Body []newsfeed.Newsfeed
}

// swagger:route GET /newsfeed/{id} newsfeed getNewsfeedItem
// Get newsfeed item by ID
//
// Returns a single newsfeed item by ID
//
// Parameters:
//   + name: id
//     in: path
//     description: Newsfeed ID
//     required: true
//     type: integer
//     format: int64
//
// Responses:
//
//	200: newsfeedItemResponse
//	404: errorResponse
//
// newsfeedItemResponse is the response for a single newsfeed item
// swagger:response newsfeedItemResponse
type newsfeedItemResponse struct {
	// Newsfeed item
	// in:body
	Body newsfeed.Newsfeed
}

// swagger:route GET /newsfeedcount newsfeed getNewsfeedCount
// Get newsfeed count
//
// Returns count of newsfeed items
//
// Responses:
//
//	200: genericResponse

// swagger:route POST /newsfeed newsfeed postNewsfeed
// Create or action newsfeed item
//
// Creates a new newsfeed post or performs actions (love, report, refer, etc.)
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PATCH /newsfeed newsfeed editNewsfeed
// Edit newsfeed item
//
// Edits an existing newsfeed post
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route DELETE /newsfeed/{id} newsfeed deleteNewsfeed
// Delete newsfeed item
//
// Deletes a newsfeed item by ID
//
// Parameters:
//   + name: id
//     in: path
//     description: Newsfeed ID
//     required: true
//     type: integer
//     format: int64
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// ============================================================================
// Noticeboard
// ============================================================================

// swagger:route GET /noticeboard noticeboard getNoticeboard
// List or get noticeboards
//
// Returns active noticeboards, optionally filtered by authority, or a single noticeboard by ID
//
// Parameters:
//   + name: id
//     in: query
//     description: Noticeboard ID for single fetch
//     required: false
//     type: integer
//   + name: authorityid
//     in: query
//     description: Filter by authority ID
//     required: false
//     type: integer
//
// Responses:
//
//	200: noticeboardsResponse
//
// noticeboardsResponse is the response for noticeboards
// swagger:response noticeboardsResponse
type noticeboardsResponse struct {
	// Noticeboards
	// in:body
	Body []noticeboard.NoticeboardListItem
}

// swagger:route POST /noticeboard noticeboard postNoticeboard
// Create noticeboard or perform action
//
// Creates a new noticeboard (requires lat/lng) or performs an action on existing one
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PATCH /noticeboard noticeboard patchNoticeboard
// Update noticeboard
//
// Updates noticeboard fields and optionally links photo
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route DELETE /noticeboard/{id} noticeboard deleteNoticeboard
// Delete noticeboard
//
// Deletes a noticeboard by ID. Requires mod/admin role.
//
// Parameters:
//   + name: id
//     in: path
//     description: Noticeboard ID
//     required: true
//     type: integer
//     format: int64
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse
//	403: errorResponse

// ============================================================================
// Notification
// ============================================================================

// swagger:route GET /notification notification listNotifications
// List notifications
//
// Returns all notifications for the authenticated user
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: notificationsResponse
//	401: errorResponse
//
// notificationsResponse is the response for notifications
// swagger:response notificationsResponse
type notificationsResponse struct {
	// List of notifications
	// in:body
	Body []notification.Notification
}

// swagger:route GET /notification/count notification getNotificationCount
// Get notification count
//
// Returns count of unseen notifications
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse

// swagger:route POST /notification/seen notification markNotificationSeen
// Mark notification as seen
//
// Marks a specific notification as seen
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// swagger:route POST /notification/allseen notification markAllNotificationsSeen
// Mark all notifications as seen
//
// Marks all notifications as seen for the user
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// ============================================================================
// Session
// ============================================================================

// swagger:route GET /session session getSession
// Get current session
//
// Returns the current user session including user data, memberships, and settings
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse
//	401: errorResponse

// swagger:route POST /session session postSession
// Session actions
//
// Handles session actions: LostPassword, Unsubscribe
//
// Responses:
//
//	200: successResponse
//	400: errorResponse

// swagger:route PATCH /session session patchSession
// Update session
//
// Updates user profile settings (name, about me, email, notifications, etc.)
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route DELETE /session session deleteSession
// Delete session (logout)
//
// Logs out the current user by deleting their session
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse

// ============================================================================
// Shortlink
// ============================================================================

// swagger:route GET /shortlink shortlink getShortlink
// Get shortlinks
//
// Returns a single shortlink by ID or lists all shortlinks
//
// Parameters:
//   + name: id
//     in: query
//     description: Shortlink ID
//     required: false
//     type: integer
//   + name: groupid
//     in: query
//     description: Filter by group ID
//     required: false
//     type: integer
//
// Responses:
//
//	200: shortlinksResponse
//
// shortlinksResponse is the response for shortlinks
// swagger:response shortlinksResponse
type shortlinksResponse struct {
	// Shortlinks
	// in:body
	Body []shortlink.Shortlink
}

// swagger:route POST /shortlink shortlink createShortlink
// Create a shortlink
//
// Creates a new shortlink
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// ============================================================================
// Simulation
// ============================================================================

// swagger:route GET /simulation simulation getSimulation
// Get simulation data
//
// Returns simulation run data for analysis
//
// Responses:
//
//	200: genericResponse

// ============================================================================
// Spammers
// ============================================================================

// swagger:route GET /modtools/spammers modtools getSpammers
// Get spammers
//
// Returns list of reported spammers for moderator review
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse
//	401: errorResponse

// swagger:route GET /modtools/spammers/export modtools exportSpammers
// Export spammers
//
// Exports spammer data for partner integration (requires partner key)
//
// Responses:
//
//	200: genericResponse
//	403: errorResponse

// swagger:route POST /modtools/spammers modtools postSpammer
// Report spammer
//
// Reports a user as a spammer
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PATCH /modtools/spammers modtools patchSpammer
// Update spammer report
//
// Updates a spammer report
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route DELETE /modtools/spammers modtools deleteSpammer
// Delete spammer report
//
// Removes a spammer report
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// ============================================================================
// Status
// ============================================================================

// swagger:route GET /status status getStatus
// Get system status
//
// Returns the system status from /tmp/iznik.status
//
// Responses:
//
//	200: genericResponse

// ============================================================================
// Std Msg (Standard Messages)
// ============================================================================

// swagger:route GET /modtools/stdmsg modtools getStdMsg
// Get standard messages
//
// Returns standard messages for moderation
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: stdMsgResponse
//	401: errorResponse
//
// stdMsgResponse is the response for standard messages
// swagger:response stdMsgResponse
type stdMsgResponse struct {
	// Standard message data
	// in:body
	Body stdmsg.StdMsg
}

// swagger:route POST /modtools/stdmsg modtools createStdMsg
// Create standard message
//
// Creates a new standard message
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PATCH /modtools/stdmsg modtools patchStdMsg
// Update standard message
//
// Updates an existing standard message
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route DELETE /modtools/stdmsg modtools deleteStdMsg
// Delete standard message
//
// Deletes a standard message
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// ============================================================================
// Story
// ============================================================================

// swagger:route GET /story story listStories
// List stories
//
// Returns all stories
//
// Responses:
//
//	200: storiesResponse
//
// storiesResponse is the response for story list
// swagger:response storiesResponse
type storiesResponse struct {
	// Stories
	// in:body
	Body []story.Story
}

// swagger:route GET /story/{id} story getStory
// Get story by ID
//
// Returns a single story by ID
//
// Parameters:
//   + name: id
//     in: path
//     description: Story ID
//     required: true
//     type: integer
//     format: int64
//
// Responses:
//
//	200: storyResponse
//	404: errorResponse
//
// storyResponse is the response for a single story
// swagger:response storyResponse
type storyResponse struct {
	// Story data
	// in:body
	Body story.Story
}

// swagger:route GET /story/group/{id} story getStoriesForGroup
// Get stories for group
//
// Returns stories for a specific group
//
// Parameters:
//   + name: id
//     in: path
//     description: Group ID
//     required: true
//     type: integer
//     format: int64
//
// Responses:
//
//	200: storiesResponse

// swagger:route PUT /story story createStory
// Create a story
//
// Creates a new story
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PATCH /story story updateStory
// Update a story (mod review)
//
// Updates a story, typically for moderation review
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route POST /story story postStory
// Story actions (Like/Unlike)
//
// Handles story actions
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse

// swagger:route POST /story/like story likeStory
// Like a story
//
// Likes a story
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// swagger:route POST /story/unlike story unlikeStory
// Unlike a story
//
// Removes a like from a story
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// swagger:route DELETE /story/{id} story deleteStory
// Delete a story
//
// Deletes a story by ID
//
// Parameters:
//   + name: id
//     in: path
//     description: Story ID
//     required: true
//     type: integer
//     format: int64
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// ============================================================================
// System Logs
// ============================================================================

// swagger:route GET /systemlogs systemlogs getSystemLogs
// Get system logs
//
// Returns system logs (moderator only)
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: systemLogsResponse
//	401: errorResponse
//	403: errorResponse
//
// systemLogsResponse is the response for system logs
// swagger:response systemLogsResponse
type systemLogsResponse struct {
	// System logs
	// in:body
	Body systemlogs.LogsResponse
}

// swagger:route GET /systemlogs/counts systemlogs getSystemLogCounts
// Get log counts by subtype
//
// Returns counts of logs grouped by subtype using Loki metric queries (moderator only)
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: systemLogCountsResponse
//	401: errorResponse
//	403: errorResponse
//
// systemLogCountsResponse is the response for system log counts
// swagger:response systemLogCountsResponse
type systemLogCountsResponse struct {
	// Log counts
	// in:body
	Body systemlogs.CountsResponse
}

// ============================================================================
// Team
// ============================================================================

// swagger:route GET /team team getTeam
// Get team
//
// Returns team information
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: teamResponse
//	401: errorResponse
//
// teamResponse is the response for team
// swagger:response teamResponse
type teamResponse struct {
	// Team data
	// in:body
	Body team.Team
}

// swagger:route POST /team team createTeam
// Create team member
//
// Adds a member to a team
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PATCH /team team patchTeam
// Update team member
//
// Updates a team member
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route DELETE /team team deleteTeam
// Delete team member
//
// Removes a member from a team
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// ============================================================================
// Tryst (Handover Arrangements)
// ============================================================================

// swagger:route GET /tryst tryst getTryst
// Get trysts
//
// Returns handover arrangements for the authenticated user
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: trystResponse
//	401: errorResponse
//
// trystResponse is the response for trysts
// swagger:response trystResponse
type trystResponse struct {
	// Tryst data
	// in:body
	Body tryst.Tryst
}

// swagger:route PUT /tryst tryst createTryst
// Create tryst
//
// Creates a new handover arrangement
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route POST /tryst tryst postTryst
// Tryst actions
//
// Performs actions on a tryst (confirm, decline, etc.)
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PATCH /tryst tryst patchTryst
// Update tryst
//
// Updates a handover arrangement
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route DELETE /tryst tryst deleteTryst
// Delete tryst
//
// Deletes a handover arrangement
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// ============================================================================
// User
// ============================================================================

// swagger:route GET /user/{id} user getUser
// Get user by ID
//
// Returns a single user by ID, or the current user if no ID
//
// Parameters:
//   + name: id
//     in: path
//     description: User ID
//     required: true
//     type: integer
//     format: int64
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: userResponse
//	404: errorResponse
//
// userResponse is the response for a single user
// swagger:response userResponse
type userResponse struct {
	// User data
	// in:body
	Body user.User
}

// swagger:route GET /user/search user searchUsers
// Search users
//
// Searches users by name or email (moderator only)
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse
//	401: errorResponse

// swagger:route GET /user/byemail/{email} user getUserByEmail
// Get user by email
//
// Returns a user by email address
//
// Parameters:
//   + name: email
//     in: path
//     description: Email address
//     required: true
//     type: string
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: userResponse
//	404: errorResponse

// swagger:route POST /user user postUser
// User actions
//
// Handles user actions: Rate, RatingReviewed, AddEmail, RemoveEmail, Engaged
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PUT /user user putUser
// Create user
//
// Creates a new user or signs up
//
// Responses:
//
//	200: genericResponse
//	400: errorResponse

// swagger:route PATCH /user user patchUser
// Update user
//
// Updates user fields
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route DELETE /user user deleteUser
// Delete user
//
// Deletes the authenticated user account
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// swagger:route GET /user/{id}/publiclocation user getUserPublicLocation
// Get user's public location
//
// Returns the public location for a specific user
//
// Parameters:
//   + name: id
//     in: path
//     description: User ID
//     required: true
//     type: integer
//     format: int64
//
// Responses:
//
//	200: locationResponse

// swagger:route GET /user/{id}/message user getUserMessages
// Get messages for user
//
// Returns messages created by a specific user
//
// Parameters:
//   + name: id
//     in: path
//     description: User ID
//     required: true
//     type: integer
//     format: int64
//   + name: active
//     in: query
//     description: Only show active messages
//     required: false
//     type: boolean
//
// Responses:
//
//	200: messagesResponse

// swagger:route GET /user/{id}/search user getUserSearches
// Get searches for user
//
// Returns saved searches for a specific user
//
// Parameters:
//   + name: id
//     in: path
//     description: User ID
//     required: true
//     type: integer
//     format: int64
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: genericResponse

// swagger:route DELETE /usersearch usersearch deleteUserSearch
// Delete a user search
//
// Soft-deletes a user search (sets deleted=1)
//
// Parameters:
//   + name: id
//     in: query
//     description: Search ID
//     required: true
//     type: integer
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// ============================================================================
// Visualise
// ============================================================================

// swagger:route GET /visualise visualise getVisualise
// Get visualisation data
//
// Returns items given/taken with locations and user icons for homepage map
//
// Parameters:
//   + name: swlat
//     in: query
//     description: Southwest latitude
//     required: true
//     type: number
//   + name: swlng
//     in: query
//     description: Southwest longitude
//     required: true
//     type: number
//   + name: nelat
//     in: query
//     description: Northeast latitude
//     required: true
//     type: number
//   + name: nelng
//     in: query
//     description: Northeast longitude
//     required: true
//     type: number
//   + name: limit
//     in: query
//     description: Max results (default 5)
//     required: false
//     type: integer
//
// Responses:
//
//	200: visualiseResponse
//
// visualiseResponse is the response for visualisation data
// swagger:response visualiseResponse
type visualiseResponse struct {
	// Visualise result
	// in:body
	Body visualise.VisualiseResult
}

// ============================================================================
// Volunteering
// ============================================================================

// swagger:route GET /volunteering volunteering listVolunteering
// List volunteering opportunities
//
// Returns all volunteering opportunities
//
// Responses:
//
//	200: volunteeringListResponse
//
// volunteeringListResponse is the response for volunteering list
// swagger:response volunteeringListResponse
type volunteeringListResponse struct {
	// Volunteering opportunities
	// in:body
	Body []volunteering.Volunteering
}

// swagger:route GET /volunteering/group/{id} volunteering listVolunteeringForGroup
// List volunteering opportunities for group
//
// Returns volunteering opportunities for a specific group
//
// Parameters:
//   + name: id
//     in: path
//     description: Group ID
//     required: true
//     type: integer
//     format: int64
//
// Responses:
//
//	200: volunteeringListResponse

// swagger:route GET /volunteering/{id} volunteering getVolunteering
// Get volunteering opportunity by ID
//
// Returns a single volunteering opportunity by ID
//
// Parameters:
//   + name: id
//     in: path
//     description: Volunteering ID
//     required: true
//     type: integer
//     format: int64
//
// Responses:
//
//	200: volunteeringResponse
//	404: errorResponse
//
// volunteeringResponse is the response for a single volunteering opportunity
// swagger:response volunteeringResponse
type volunteeringResponse struct {
	// Volunteering data
	// in:body
	Body volunteering.Volunteering
}

// swagger:route POST /volunteering volunteering createVolunteering
// Create volunteering opportunity
//
// Creates a new volunteering opportunity
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route PATCH /volunteering volunteering updateVolunteering
// Update volunteering opportunity
//
// Updates an existing volunteering opportunity
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	400: errorResponse
//	401: errorResponse

// swagger:route DELETE /volunteering/{id} volunteering deleteVolunteering
// Delete volunteering opportunity
//
// Deletes a volunteering opportunity by ID
//
// Parameters:
//   + name: id
//     in: path
//     description: Volunteering ID
//     required: true
//     type: integer
//     format: int64
//
// security:
// - BearerAuth: []
//
// Responses:
//
//	200: successResponse
//	401: errorResponse

// ============================================================================
// Shared Response Types
// ============================================================================

// successResponse is the response for successful operations
// swagger:response successResponse
type successResponse struct {
	// Success indicator
	// in:body
	Body struct {
		Success bool `json:"success"`
	}
}

// errorResponse is the error response
// swagger:response errorResponse
type errorResponse struct {
	// Error information
	// in:body
	Body fiber.Map
}

// genericResponse is a generic JSON response for endpoints returning fiber.Map
// swagger:response genericResponse
type genericResponse struct {
	// Response data
	// in:body
	Body fiber.Map
}

// noContentResponse is the response for 204 No Content
// swagger:response noContentResponse
type noContentResponse struct{}

// fileResponse is the response for binary file data
// swagger:response fileResponse
type fileResponse struct{}

// ============================================================================
// Shared Request Models
// ============================================================================

// CreateSpamKeywordRequest model for creating spam keywords
// swagger:model configCreateSpamKeywordRequest
type CreateSpamKeywordRequest struct {
	// Word to match
	// Required: true
	Word string `json:"word"`
	// Exclude pattern (optional)
	Exclude *string `json:"exclude"`
	// Action to take: Spam, Review, or Whitelist
	// Required: true
	// Enum: Spam,Review,Whitelist
	Action string `json:"action"`
	// Type of matching: Literal or Regex
	// Required: true
	// Enum: Literal,Regex
	Type string `json:"type"`
}

// CreateWorryWordRequest model for creating worry words
// swagger:model configCreateWorryWordRequest
type CreateWorryWordRequest struct {
	// Keyword to match
	// Required: true
	Keyword string `json:"keyword"`
	// Substance description
	Substance string `json:"substance"`
	// Type of worry word
	Type string `json:"type"`
}
