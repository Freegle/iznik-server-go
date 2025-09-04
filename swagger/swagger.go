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
	"github.com/freegle/iznik-server-go/address"
	"github.com/freegle/iznik-server-go/chat"
	"github.com/freegle/iznik-server-go/config"
	"github.com/freegle/iznik-server-go/message"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// NOTE: The following structs and methods are not meant to be implemented,
// they exist solely for Swagger to generate documentation.

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
