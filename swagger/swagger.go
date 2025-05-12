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
// Security:
// - BearerAuth:
//
// SecurityDefinitions:
// BearerAuth:
//
//	type: apiKey
//	name: Authorization
//	in: header
//
// swagger:meta
package swagger

import (
	"github.com/freegle/iznik-server-go/address"
	"github.com/freegle/iznik-server-go/chat"
	"github.com/freegle/iznik-server-go/message"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// NOTE: The following structs and methods are not meant to be implemented,
// they exist solely for Swagger to generate documentation.

// swagger:route GET /activity message getActivity
// Get recent activity
//
// # Returns the most recent activity in groups
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
// # Returns all addresses for the authenticated user
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
// # Returns a single address by ID
//
// Parameters:
//   - name: id
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

// swagger:route GET /chat chat listChats
// List chats for user
//
// # Returns all chats for the authenticated user
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
// # Returns a single chat by ID
//
// Parameters:
//   - name: id
//     in: path
//     description: Chat ID
//     required: true
//     type: integer
//     format: int64
//
// Security:
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
//   - name: ids
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
// # Returns a single user by ID, or the current user if no ID
//
// Parameters:
//   - name: id
//     in: path
//     description: User ID
//     required: true
//     type: integer
//     format: int64
//
// Security:
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

// errorResponse is the error response
// swagger:response errorResponse
type errorResponse struct {
	// Error information
	// in:body
	Body fiber.Map
}
