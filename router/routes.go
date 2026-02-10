// Package router provides routing for the API
//
// @title Iznik API
// @version 1.0
// @description The Iznik API provides access to functionality for freegling (free reuse) groups.  See README.md for more info.
// @termsOfService https://www.ilovefreegle.org/terms
//
// @contact.name Freegle Geeks
// @contact.url https://www.ilovefreegle.org/help
// @contact.email geeks@ilovefreegle.org
//
// @license.name GPL v2
// @license.url https://www.gnu.org/licenses/old-licenses/gpl-2.0.en.html
//
// @host api.ilovefreegle.org
// @BasePath /api
// @query.collection.format multi
//
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
package router

import (
	"github.com/freegle/iznik-server-go/abtest"
	"github.com/freegle/iznik-server-go/address"
	"github.com/freegle/iznik-server-go/amp"
	"github.com/freegle/iznik-server-go/authority"
	"github.com/freegle/iznik-server-go/chat"
	"github.com/freegle/iznik-server-go/clientlog"
	"github.com/freegle/iznik-server-go/comment"
	"github.com/freegle/iznik-server-go/communityevent"
	"github.com/freegle/iznik-server-go/config"
	"github.com/freegle/iznik-server-go/dashboard"
	"github.com/freegle/iznik-server-go/donations"
	"github.com/freegle/iznik-server-go/invitation"
	"github.com/freegle/iznik-server-go/emailtracking"
	"github.com/freegle/iznik-server-go/group"
	"github.com/freegle/iznik-server-go/image"
	"github.com/freegle/iznik-server-go/isochrone"
	"github.com/freegle/iznik-server-go/job"
	"github.com/freegle/iznik-server-go/location"
	"github.com/freegle/iznik-server-go/logo"
	"github.com/freegle/iznik-server-go/logs"
	"github.com/freegle/iznik-server-go/membership"
	"github.com/freegle/iznik-server-go/message"
	"github.com/freegle/iznik-server-go/microvolunteering"
	"github.com/freegle/iznik-server-go/misc"
	"github.com/freegle/iznik-server-go/newsfeed"
	"github.com/freegle/iznik-server-go/noticeboard"
	"github.com/freegle/iznik-server-go/notification"
	"github.com/freegle/iznik-server-go/session"
	"github.com/freegle/iznik-server-go/shortlink"
	"github.com/freegle/iznik-server-go/spammers"
	"github.com/freegle/iznik-server-go/src"
	"github.com/freegle/iznik-server-go/status"
	"github.com/freegle/iznik-server-go/story"
	"github.com/freegle/iznik-server-go/systemlogs"
	"github.com/freegle/iznik-server-go/team"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/visualise"
	"github.com/freegle/iznik-server-go/volunteering"
	"github.com/gofiber/fiber/v2"
)

// SetupRoutes registers all API routes
// @Summary Setup all API routes
// @Description Configures both /api and /apiv2 route groups
func SetupRoutes(app *fiber.App) {
	// We have two groups because of how the API is used in the old and new clients.
	api := app.Group("/api")
	apiv2 := app.Group("/apiv2")

	for _, rg := range []fiber.Router{api, apiv2} {
		// A/B Test GET
		// @Router /abtest [get]
		// @Summary Get A/B test variant
		// @Description Returns the best-performing variant for a test UID using epsilon-greedy bandit
		// @Tags abtest
		// @Produce json
		rg.Get("/abtest", abtest.GetABTest)

		// A/B Test POST
		// @Router /abtest [post]
		// @Summary Track A/B test event
		// @Description Record a shown or action event for a variant
		// @Tags abtest
		// @Accept json
		// @Produce json
		rg.Post("/abtest", abtest.PostABTest)

		// Message Activity
		// @Router /activity [get]
		// @Summary Get recent activity
		// @Description Returns the most recent activity in groups
		// @Tags message
		// @Produce json
		// @Success 200 {array} message.Activity
		rg.Get("/activity", message.GetRecentActivity)

		// User Addresses
		// @Router /address [get]
		// @Summary List addresses for user
		// @Description Returns all addresses for the authenticated user
		// @Tags address
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {array} address.Address
		rg.Get("/address", address.ListForUser)

		// Single Address
		// @Router /address/{id} [get]
		// @Summary Get address by ID
		// @Description Returns a single address by ID
		// @Tags address
		// @Produce json
		// @Param id path integer true "Address ID"
		// @Success 200 {object} address.Address
		// @Failure 404 {object} fiber.Error "Address not found"
		rg.Get("/address/:id", address.GetAddress)

		// Create Address
		// @Router /address [post]
		// @Summary Create a new address
		// @Tags address
		// @Accept json
		// @Produce json
		rg.Post("/address", address.Create)

		// Update Address
		// @Router /address [patch]
		// @Summary Update an existing address
		// @Tags address
		// @Accept json
		// @Produce json
		rg.Patch("/address", address.Update)

		// Delete Address
		// @Router /address/{id} [delete]
		// @Summary Delete an address
		// @Tags address
		// @Param id path integer true "Address ID"
		rg.Delete("/address/:id", address.Delete)

		// Authority Search
		// @Router /authority [get]
		// @Summary Search authorities
		// @Description Searches authorities by name
		// @Tags authority
		// @Produce json
		// @Param search query string true "Search term"
		// @Param limit query integer false "Maximum results (default 10)"
		// @Success 200 {array} authority.SearchResult
		rg.Get("/authority", authority.Search)

		// Single Authority
		// @Router /authority/{id} [get]
		// @Summary Get authority by ID
		// @Description Returns a single authority by ID with polygon, centre, and overlapping groups
		// @Tags authority
		// @Produce json
		// @Param id path integer true "Authority ID"
		// @Success 200 {object} authority.Authority
		// @Failure 404 {object} fiber.Error "Authority not found"
		rg.Get("/authority/:id", authority.Single)

		// Authority Messages
		// @Router /authority/{id}/message [get]
		// @Summary Get messages for authority
		// @Description Returns messages for a specific authority
		// @Tags authority
		// @Produce json
		// @Param id path integer true "Authority ID"
		// @Success 200 {array} authority.Message
		rg.Get("/authority/:id/message", authority.Messages)

		// Chats
		// @Router /chat [get]
		// @Summary List chats for user
		// @Description Returns all chats for the authenticated user
		// @Tags chat
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {array} chat.ChatRoom
		rg.Get("/chat", chat.ListForUser)

		// Chat Messages
		// @Router /chat/{id}/message [get]
		// @Summary Get chat messages
		// @Description Returns messages for a specific chat
		// @Tags chat
		// @Produce json
		// @Param id path integer true "Chat ID"
		// @Security BearerAuth
		// @Success 200 {array} chat.ChatMessage
		rg.Get("/chat/:id/message", chat.GetChatMessages)

		// Create Chat Message
		// @Router /chat/{id}/message [post]
		// @Summary Create chat message
		// @Description Creates a new message in a chat
		// @Tags chat
		// @Accept json
		// @Produce json
		// @Param id path integer true "Chat ID"
		// @Param message body chat.ChatMessage true "Chat message object"
		// @Security BearerAuth
		// @Success 200 {object} chat.ChatMessage
		rg.Post("/chat/:id/message", chat.CreateChatMessage)

		// Patch Chat Message
		// @Router /chatmessages [patch]
		// @Summary Update chat message
		// @Description Updates a chat message (e.g. replyexpected flag)
		// @Tags chat
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} fiber.Map
		rg.Patch("/chatmessages", chat.PatchChatMessage)

		// Delete Chat Message
		// @Router /chatmessages [delete]
		// @Summary Delete chat message
		// @Description Soft-deletes a chat message owned by the logged-in user
		// @Tags chat
		// @Produce json
		// @Param id query integer true "Chat Message ID"
		// @Security BearerAuth
		// @Success 200 {object} fiber.Map
		rg.Delete("/chatmessages", chat.DeleteChatMessage)

		// LoveJunk Chat
		// @Router /chat/lovejunk [post]
		// @Summary Create LoveJunk chat message
		// @Description Creates a new LoveJunk chat message
		// @Tags chat
		// @Accept json
		// @Produce json
		// @Param message body chat.ChatMessage true "Chat message object"
		// @Security BearerAuth
		// @Success 200 {object} chat.ChatMessage
		rg.Post("/chat/lovejunk", chat.CreateChatMessageLoveJunk)

		// Single Chat
		// @Router /chat/{id} [get]
		// @Summary Get chat by ID
		// @Description Returns a single chat by ID
		// @Tags chat
		// @Produce json
		// @Param id path integer true "Chat ID"
		// @Security BearerAuth
		// @Success 200 {object} chat.ChatRoom
		// @Failure 404 {object} fiber.Error "Chat not found"
		rg.Get("/chat/:id", chat.GetChat)

		// Chatroom Actions
		// @Router /chatrooms [post]
		// @Summary Chatroom actions (roster update, nudge, typing)
		// @Description Handles roster updates, nudge messages, and typing indicators for chat rooms
		// @Tags chat
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} map[string]interface{}
		rg.Post("/chatrooms", chat.PostChatRoom)

		// Client Logging
		// @Router /clientlog [post]
		// @Summary Receive client logs
		// @Description Accepts client-side log entries for distributed tracing
		// @Tags logging
		// @Accept json
		// @Produce json
		// @Param logs body clientlog.ClientLogRequest true "Client log entries"
		// @Success 204 "No Content"
		rg.Post("/clientlog", clientlog.ReceiveClientLogs)

		// Dashboard
		// @Router /dashboard [get]
		// @Summary Get dashboard data
		// @Description Returns dashboard components for moderator/user dashboards
		// @Tags dashboard
		// @Produce json
		// @Param components query string false "Comma-separated component names"
		// @Param group query integer false "Group ID"
		// @Success 200 {object} map[string]interface{}
		rg.Get("/dashboard", dashboard.GetDashboard)

		// Community Events
		// @Router /communityevent [get]
		// @Summary List community events
		// @Description Returns all community events
		// @Tags communityevent
		// @Produce json
		// @Success 200 {array} communityevent.CommunityEvent
		rg.Get("/communityevent", communityevent.List)

		// Group Community Events
		// @Router /communityevent/group/{id} [get]
		// @Summary List community events for group
		// @Description Returns all community events for a specific group
		// @Tags communityevent
		// @Produce json
		// @Param id path integer true "Group ID"
		// @Success 200 {array} communityevent.CommunityEvent
		rg.Get("/communityevent/group/:id", communityevent.ListGroup)

		// Single Community Event
		// @Router /communityevent/{id} [get]
		// @Summary Get community event by ID
		// @Description Returns a single community event by ID
		// @Tags communityevent
		// @Produce json
		// @Param id path integer true "Community Event ID"
		// @Success 200 {object} communityevent.CommunityEvent
		// @Failure 404 {object} fiber.Error "Community event not found"
		rg.Get("/communityevent/:id", communityevent.Single)
		rg.Post("/communityevent", communityevent.Create)
		rg.Patch("/communityevent", communityevent.Update)
		rg.Delete("/communityevent/:id", communityevent.Delete)

		// Comment Write Operations
		// @Router /comment [post]
		// @Summary Create a comment on a user
		// @Description Moderators can add comments to users in their groups
		// @Tags comment
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} map[string]interface{}
		rg.Post("/comment", comment.Create)

		// @Router /comment [patch]
		// @Summary Edit a comment
		// @Description Moderators can edit comments on users in their groups
		// @Tags comment
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} map[string]interface{}
		rg.Patch("/comment", comment.Edit)

		// @Router /comment/{id} [delete]
		// @Summary Delete a comment
		// @Description Moderators can delete comments on users in their groups
		// @Tags comment
		// @Produce json
		// @Param id path integer true "Comment ID"
		// @Security BearerAuth
		// @Success 200 {object} map[string]interface{}
		rg.Delete("/comment/:id", comment.Delete)

		// Config
		// @Router /config/{key} [get]
		// @Summary Get configuration
		// @Description Returns configuration by key
		// @Tags config
		// @Produce json
		// @Param key path string true "Configuration key"
		// @Success 200 {object} config.ConfigItem
		rg.Get("/config/:key", config.Get)

		// Create a protected route group for admin endpoints
		adminConfig := rg.Group("/config/admin")
		adminConfig.Use(config.RequireSupportOrAdminMiddleware())

		// Spam Keywords (Admin protected)
		// @Router /config/admin/spam_keywords [get]
		// @Summary List spam keywords
		// @Description Returns all spam keywords (Support/Admin only)
		// @Tags config
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {array} config.SpamKeyword
		// @Failure 401 {object} fiber.Error "Authentication required"
		// @Failure 403 {object} fiber.Error "Support or Admin role required"
		adminConfig.Get("/spam_keywords", config.ListSpamKeywords)

		// @Router /config/admin/spam_keywords [post]
		// @Summary Create spam keyword
		// @Description Creates a new spam keyword (Support/Admin only)
		// @Tags config
		// @Accept json
		// @Produce json
		// @Param spam_keyword body config.CreateSpamKeywordRequest true "Spam keyword object"
		// @Security BearerAuth
		// @Success 200 {object} config.SpamKeyword
		// @Failure 400 {object} fiber.Error "Invalid request"
		// @Failure 401 {object} fiber.Error "Authentication required"
		// @Failure 403 {object} fiber.Error "Support or Admin role required"
		adminConfig.Post("/spam_keywords", config.CreateSpamKeyword)

		// @Router /config/admin/spam_keywords/{id} [delete]
		// @Summary Delete spam keyword
		// @Description Deletes a spam keyword by ID (Support/Admin only)
		// @Tags config
		// @Param id path integer true "Spam keyword ID"
		// @Security BearerAuth
		// @Success 200 {object} fiber.Map "Success"
		// @Failure 400 {object} fiber.Error "Invalid ID"
		// @Failure 401 {object} fiber.Error "Authentication required"
		// @Failure 403 {object} fiber.Error "Support or Admin role required"
		// @Failure 404 {object} fiber.Error "Spam keyword not found"
		adminConfig.Delete("/spam_keywords/:id", config.DeleteSpamKeyword)

		// Worry Words (Admin protected)
		// @Router /config/admin/worry_words [get]
		// @Summary List worry words
		// @Description Returns all worry words (Support/Admin only)
		// @Tags config
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {array} config.WorryWord
		// @Failure 401 {object} fiber.Error "Authentication required"
		// @Failure 403 {object} fiber.Error "Support or Admin role required"
		adminConfig.Get("/worry_words", config.ListWorryWords)

		// @Router /config/admin/worry_words [post]
		// @Summary Create worry word
		// @Description Creates a new worry word (Support/Admin only)
		// @Tags config
		// @Accept json
		// @Produce json
		// @Param worry_word body config.CreateWorryWordRequest true "Worry word object"
		// @Security BearerAuth
		// @Success 200 {object} config.WorryWord
		// @Failure 400 {object} fiber.Error "Invalid request"
		// @Failure 401 {object} fiber.Error "Authentication required"
		// @Failure 403 {object} fiber.Error "Support or Admin role required"
		adminConfig.Post("/worry_words", config.CreateWorryWord)

		// @Router /config/admin/worry_words/{id} [delete]
		// @Summary Delete worry word
		// @Description Deletes a worry word by ID (Support/Admin only)
		// @Tags config
		// @Param id path integer true "Worry word ID"
		// @Security BearerAuth
		// @Success 200 {object} fiber.Map "Success"
		// @Failure 400 {object} fiber.Error "Invalid ID"
		// @Failure 401 {object} fiber.Error "Authentication required"
		// @Failure 403 {object} fiber.Error "Support or Admin role required"
		// @Failure 404 {object} fiber.Error "Worry word not found"
		adminConfig.Delete("/worry_words/:id", config.DeleteWorryWord)

		// Groups
		// @Router /group [get]
		// @Summary List groups
		// @Description Returns all groups
		// @Tags group
		// @Produce json
		// @Success 200 {array} group.Group
		rg.Get("/group", group.ListGroups)

		// Single Group
		// @Router /group/{id} [get]
		// @Summary Get group by ID
		// @Description Returns a single group by ID
		// @Tags group
		// @Produce json
		// @Param id path integer true "Group ID"
		// @Success 200 {object} group.Group
		// @Failure 404 {object} fiber.Error "Group not found"
		rg.Get("/group/:id", group.GetGroup)

		// Group Messages
		// @Router /group/{id}/message [get]
		// @Summary Get messages for group
		// @Description Returns messages for a specific group
		// @Tags group,message
		// @Produce json
		// @Param id path integer true "Group ID"
		// @Success 200 {array} message.Message
		rg.Get("/group/:id/message", group.GetGroupMessages)

		// Group PATCH
		// @Router /group [patch]
		// @Summary Update group settings
		// @Description Update group fields. Requires mod/owner role or admin/support.
		// @Tags group
		// @Accept json
		// @Produce json
		rg.Patch("/group", group.PatchGroup)

		// Noticeboard POST (create + action)
		// @Router /noticeboard [post]
		// @Summary Create noticeboard or perform action
		// @Description Create a new noticeboard (requires lat/lng) or perform an action on existing one
		// @Tags noticeboard
		// @Accept json
		// @Produce json
		rg.Post("/noticeboard", noticeboard.PostNoticeboard)

		// Noticeboard PATCH
		// @Router /noticeboard [patch]
		// @Summary Update noticeboard
		// @Description Update noticeboard fields and optionally link photo
		// @Tags noticeboard
		// @Accept json
		// @Produce json
		rg.Patch("/noticeboard", noticeboard.PatchNoticeboard)

		// Isochrones
		// @Router /isochrone [get]
		// @Summary List isochrones
		// @Description Returns all isochrones
		// @Tags isochrone
		// @Produce json
		// @Success 200 {array} isochrone.Isochrone
		rg.Get("/isochrone", isochrone.ListIsochrones)
		rg.Put("/isochrone", isochrone.CreateIsochrone)
		rg.Patch("/isochrone", isochrone.EditIsochrone)
		rg.Delete("/isochrone", isochrone.DeleteIsochrone)

		// Isochrone Messages
		// @Router /isochrone/message [get]
		// @Summary Get messages for isochrone
		// @Description Returns messages for isochrones
		// @Tags isochrone,message
		// @Produce json
		// @Success 200 {array} isochrone.Message
		rg.Get("/isochrone/message", isochrone.Messages)

		// Volunteering Write Operations
		rg.Post("/volunteering", volunteering.Create)
		rg.Patch("/volunteering", volunteering.Update)
		rg.Delete("/volunteering/:id", volunteering.Delete)

		// Image Attachments
		// @Router /image [post]
		// @Summary Create or update image attachment
		// @Description Registers an externally-uploaded image (via Tus) or rotates an existing image
		// @Tags image
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} map[string]interface{}
		rg.Post("/image", image.Post)

		// Jobs
		// @Router /job [get]
		// @Summary List jobs
		// @Description Returns all jobs
		// @Tags job
		// @Produce json
		// @Success 200 {array} job.Job
		rg.Get("/job", job.GetJobs)

		// Single Job
		// @Router /job/{id} [get]
		// @Summary Get job by ID
		// @Description Returns a single job by ID
		// @Tags job
		// @Produce json
		// @Param id path integer true "Job ID"
		// @Success 200 {object} job.Job
		// @Failure 404 {object} fiber.Error "Job not found"
		rg.Get("/job/:id", job.GetJob)

		// Record Job Click
		// @Router /job [post]
		// @Summary Record a job click
		// @Description Records when a user clicks on a job listing for analytics
		// @Tags job
		// @Produce json
		// @Param id query integer true "Job ID"
		// @Param link query string false "Job URL"
		// @Success 200 {object} map[string]interface{} "Success response"
		// @Failure 400 {object} fiber.Error "Job ID required"
		rg.Post("/job", job.RecordJobClick)

		// Location by Lat/Lng
		// @Router /location/latlng [get]
		// @Summary Get location by latitude/longitude
		// @Description Returns location info for given coordinates
		// @Tags location
		// @Produce json
		// @Param lat query number true "Latitude"
		// @Param lng query number true "Longitude"
		// @Success 200 {object} location.Location
		rg.Get("/location/latlng", location.LatLng)

		// Location Typeahead
		// @Router /location/typeahead [get]
		// @Summary Location typeahead search
		// @Description Returns location suggestions for typeahead
		// @Tags location
		// @Produce json
		// @Param term query string true "Search term"
		// @Success 200 {array} location.Location
		rg.Get("/location/typeahead", location.Typeahead)

		// Location Addresses
		// @Router /location/{id}/addresses [get]
		// @Summary Get addresses for location
		// @Description Returns addresses for a specific location
		// @Tags location
		// @Produce json
		// @Param id path integer true "Location ID"
		// @Success 200 {array} address.Address
		rg.Get("/location/:id/addresses", location.GetLocationAddresses)

		// Single Location
		// @Router /location/{id} [get]
		// @Summary Get location by ID
		// @Description Returns a single location by ID
		// @Tags location
		// @Produce json
		// @Param id path integer true "Location ID"
		// @Success 200 {object} location.Location
		// @Failure 404 {object} fiber.Error "Location not found"
		rg.Get("/location/:id", location.GetLocation)

		// Message Count
		// @Router /message/count [get]
		// @Summary Get message count
		// @Description Returns count of messages by type
		// @Tags message
		// @Produce json
		// @Success 200 {object} isochrone.CountResult
		rg.Get("/message/count", isochrone.Count)

		// Message Bounds
		// @Router /message/inbounds [get]
		// @Summary Get messages in bounds
		// @Description Returns messages within geographic bounds
		// @Tags message
		// @Produce json
		// @Param swlat query number true "Southwest latitude"
		// @Param swlng query number true "Southwest longitude"
		// @Param nelat query number true "Northeast latitude"
		// @Param nelng query number true "Northeast longitude"
		// @Success 200 {array} message.Message
		rg.Get("/message/inbounds", message.Bounds)

		// Messages by Group
		// @Router /message/mygroups/{id} [get]
		// @Summary Get messages by group
		// @Description Returns messages for user's groups, optionally filtered by group ID
		// @Tags message,group
		// @Produce json
		// @Param id path integer false "Group ID (optional)"
		// @Security BearerAuth
		// @Success 200 {array} message.Message
		rg.Get("/message/mygroups/:id?", message.Groups)

		// Message Search
		// @Router /message/search/{term} [get]
		// @Summary Search messages
		// @Description Searches messages by term
		// @Tags message
		// @Produce json
		// @Param term path string true "Search term"
		// @Param messagetype query string false "Message type filter"
		// @Param groupids query string false "Group IDs to filter by (comma separated)"
		// @Success 200 {array} message.SearchResult
		rg.Get("/message/search/:term", message.Search)

		// Messages by ID
		// @Router /message/{ids} [get]
		// @Summary Get messages by ID
		// @Description Returns messages by ID (comma separated)
		// @Tags message
		// @Produce json
		// @Param ids path string true "Message IDs (comma separated)"
		// @Success 200 {array} message.Message
		// @Failure 404 {object} fiber.Error "Message not found"
		rg.Get("/message/:ids", message.GetMessages)

		// Mark Messages Seen
		// @Router /messages/markseen [post]
		// @Summary Mark messages as seen
		// @Description Records that the user has viewed the specified messages
		// @Tags message
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} map[string]interface{}
		// @Failure 400 {object} fiber.Error "Invalid request"
		// @Failure 401 {object} fiber.Error "Not logged in"
		rg.Post("/messages/markseen", message.MarkSeen)

		// Message Actions (POST)
		// @Router /message [post]
		// @Summary Message actions
		// @Description Handles message actions: Promise, Renege, OutcomeIntended, Outcome, AddBy, RemoveBy, View
		// @Tags message
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} map[string]interface{}
		rg.Post("/message", message.PostMessage)

		// User
		// @Router /user/{id} [get]
		// @Summary Get user by ID
		// @Description Returns a single user by ID, or the current user if no ID
		// @Tags user
		// @Produce json
		// @Param id path integer false "User ID (optional)"
		// @Security BearerAuth
		// @Success 200 {object} user.User
		// @Failure 404 {object} fiber.Error "User not found"
		rg.Get("/user/:id?", user.GetUser)

		// User Actions (POST)
		// @Router /user [post]
		// @Summary User actions
		// @Description Handles user actions: Rate, RatingReviewed, AddEmail, RemoveEmail, Engaged
		// @Tags user
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} fiber.Map
		rg.Post("/user", user.PostUser)

		// User Public Location
		// @Router /user/{id}/publiclocation [get]
		// @Summary Get user's public location
		// @Description Returns the public location for a specific user
		// @Tags user
		// @Produce json
		// @Param id path integer true "User ID"
		// @Success 200 {object} location.Location
		rg.Get("/user/:id/publiclocation", user.GetPublicLocation)

		// User Messages
		// @Router /user/{id}/message [get]
		// @Summary Get messages for user
		// @Description Returns messages created by a specific user
		// @Tags user,message
		// @Produce json
		// @Param id path integer true "User ID"
		// @Param active query boolean false "Only show active messages"
		// @Success 200 {array} message.MessageSummary
		rg.Get("/user/:id/message", message.GetMessagesForUser)

		// User Searches
		// @Router /user/{id}/search [get]
		// @Summary Get searches for user
		// @Description Returns saved searches for a specific user
		// @Tags user
		// @Produce json
		// @Param id path integer true "User ID"
		// @Security BearerAuth
		// @Success 200 {array} user.Search
		rg.Get("/user/:id/search", user.GetSearchesForUser)

		// Delete User Search
		// @Router /usersearch [delete]
		// @Summary Delete a user search
		// @Description Soft-deletes a user search (sets deleted=1)
		// @Tags usersearch
		// @Produce json
		// @Param id query integer true "Search ID"
		// @Security BearerAuth
		// @Success 200 {object} map[string]interface{}
		rg.Delete("/usersearch", user.DeleteUserSearch)

		// Newsfeed Item
		// @Router /newsfeed/{id} [get]
		// @Summary Get newsfeed item by ID
		// @Description Returns a single newsfeed item by ID
		// @Tags newsfeed
		// @Produce json
		// @Param id path integer true "Newsfeed ID"
		// @Success 200 {object} newsfeed.Item
		// @Failure 404 {object} fiber.Error "Newsfeed item not found"
		rg.Get("/newsfeed/:id", newsfeed.Single)

		// Newsfeed Count
		// @Router /newsfeedcount [get]
		// @Summary Get newsfeed count
		// @Description Returns count of newsfeed items
		// @Tags newsfeed
		// @Produce json
		// @Success 200 {object} newsfeed.CountResult
		rg.Get("/newsfeedcount", newsfeed.Count)

		// Newsfeed
		// @Router /newsfeed [get]
		// @Summary Get newsfeed
		// @Description Returns newsfeed items
		// @Tags newsfeed
		// @Produce json
		// @Success 200 {array} newsfeed.Item
		rg.Get("/newsfeed", newsfeed.Feed)
		rg.Post("/newsfeed", newsfeed.Post)
		rg.Patch("/newsfeed", newsfeed.Edit)
		rg.Delete("/newsfeed/:id", newsfeed.Delete)

		// Notification Count
		// @Router /notification/count [get]
		// @Summary Get notification count
		// @Description Returns count of notifications
		// @Tags notification
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} notification.CountResult
		rg.Get("/notification/count", notification.Count)

		// Notifications
		// @Router /notification [get]
		// @Summary List notifications
		// @Description Returns all notifications for the authenticated user
		// @Tags notification
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {array} notification.Notification
		rg.Get("/notification", notification.List)

		// Online Status
		// @Router /online [get]
		// @Summary Check online status
		// @Description Returns online status information
		// @Tags misc
		// @Produce json
		// @Success 200 {object} misc.OnlineResult
		rg.Get("/online", misc.Online)

		// Stories
		// @Router /story [get]
		// @Summary List stories
		// @Description Returns all stories
		// @Tags story
		// @Produce json
		// @Success 200 {array} story.Story
		rg.Get("/story", story.List)

		// Single Story
		// @Router /story/{id} [get]
		// @Summary Get story by ID
		// @Description Returns a single story by ID
		// @Tags story
		// @Produce json
		// @Param id path integer true "Story ID"
		// @Success 200 {object} story.Story
		// @Failure 404 {object} fiber.Error "Story not found"
		rg.Get("/story/:id", story.Single)

		// Group Stories
		// @Router /story/group/{id} [get]
		// @Summary Get stories for group
		// @Description Returns stories for a specific group
		// @Tags story,group
		// @Produce json
		// @Param id path integer true "Group ID"
		// @Success 200 {array} story.Story
		rg.Get("/story/group/:id", story.Group)

		// Session Actions
		// @Router /session [post]
		// @Summary Session actions (LostPassword, Unsubscribe)
		// @Description Dispatches session write actions based on "action" parameter
		// @Tags session
		// @Accept json
		// @Produce json
		// @Param body body object true "Action and email"
		// @Success 200 {object} map[string]interface{}
		rg.Post("/session", session.PostSession)

		// Shortlinks
		// @Router /shortlink [get]
		// @Summary Get shortlinks
		// @Description Returns a single shortlink by ID or lists all shortlinks
		// @Tags shortlink
		// @Produce json
		// @Param id query integer false "Shortlink ID"
		// @Param groupid query integer false "Filter by group ID"
		// @Success 200 {object} map[string]interface{}
		rg.Get("/shortlink", shortlink.GetShortlink)

		// Create Shortlink
		// @Router /shortlink [post]
		// @Summary Create a shortlink
		// @Tags shortlink
		// @Accept json
		// @Produce json
		rg.Post("/shortlink", shortlink.PostShortlink)

		// System Status
		// @Router /status [get]
		// @Summary Get system status
		// @Description Returns the system status from /tmp/iznik.status
		// @Tags status
		// @Produce json
		// @Success 200 {object} map[string]interface{}
		rg.Get("/status", status.GetStatus)

		// Logs
		rg.Get("/logs", logs.GetLogs)

		// Spammers
		rg.Get("/spammers", spammers.GetSpammers)
		rg.Post("/spammers", spammers.PostSpammer)
		rg.Patch("/spammers", spammers.PatchSpammer)
		rg.Delete("/spammers", spammers.DeleteSpammer)

		// Teams
		rg.Get("/team", team.GetTeam)
		rg.Post("/team", team.PostTeam)
		rg.Patch("/team", team.PatchTeam)
		rg.Delete("/team", team.DeleteTeam)

		// Volunteering Opportunities
		// @Router /volunteering [get]
		// @Summary List volunteering opportunities
		// @Description Returns all volunteering opportunities
		// @Tags volunteering
		// @Produce json
		// @Success 200 {array} volunteering.Volunteering
		rg.Get("/volunteering", volunteering.List)

		// Group Volunteering Opportunities
		// @Router /volunteering/group/{id} [get]
		// @Summary List volunteering opportunities for group
		// @Description Returns volunteering opportunities for a specific group
		// @Tags volunteering,group
		// @Produce json
		// @Param id path integer true "Group ID"
		// @Success 200 {array} volunteering.Volunteering
		rg.Get("/volunteering/group/:id", volunteering.ListGroup)

		// Single Volunteering Opportunity
		// @Router /volunteering/{id} [get]
		// @Summary Get volunteering opportunity by ID
		// @Description Returns a single volunteering opportunity by ID
		// @Tags volunteering
		// @Produce json
		// @Param id path integer true "Volunteering ID"
		// @Success 200 {object} volunteering.Volunteering
		// @Failure 404 {object} fiber.Error "Volunteering opportunity not found"
		rg.Get("/volunteering/:id", volunteering.Single)

		// Visualise
		// @Router /visualise [get]
		// @Summary Get visualisation data
		// @Description Returns items given/taken with locations and user icons for homepage map
		// @Tags visualise
		// @Produce json
		// @Param swlat query number true "Southwest latitude"
		// @Param swlng query number true "Southwest longitude"
		// @Param nelat query number true "Northeast latitude"
		// @Param nelng query number true "Northeast longitude"
		// @Param limit query integer false "Max results (default 5)"
		// @Param context query integer false "Pagination cursor"
		// @Success 200 {object} map[string]interface{}
		rg.Get("/visualise", visualise.GetVisualise)

		// Email Statistics (authenticated, admin only)
		// @Router /email/stats [get]
		// @Summary Get email tracking statistics
		// @Description Returns aggregate email statistics for Support/Admin users
		// @Tags emailtracking
		// @Produce json
		// @Security BearerAuth
		// @Param type query string false "Email type filter"
		// @Param start query string false "Start date (YYYY-MM-DD)"
		// @Param end query string false "End date (YYYY-MM-DD)"
		// @Success 200 {object} map[string]interface{}
		// @Failure 401 {object} fiber.Error "Unauthorized"
		// @Failure 403 {object} fiber.Error "Forbidden"
		rg.Get("/email/stats", emailtracking.Stats)

		// Email Statistics Time Series (authenticated, admin only)
		// @Router /email/stats/timeseries [get]
		// @Summary Get daily email statistics for charting
		// @Description Returns daily sent/opened/clicked/bounced counts for date range
		// @Tags emailtracking
		// @Produce json
		// @Security BearerAuth
		// @Param type query string false "Email type filter"
		// @Param start query string false "Start date (YYYY-MM-DD)"
		// @Param end query string false "End date (YYYY-MM-DD)"
		// @Success 200 {object} map[string]interface{}
		// @Failure 401 {object} fiber.Error "Unauthorized"
		// @Failure 403 {object} fiber.Error "Forbidden"
		rg.Get("/email/stats/timeseries", emailtracking.TimeSeries)

		// Email Statistics By Type (authenticated, admin only)
		// @Router /email/stats/bytype [get]
		// @Summary Get email statistics by email type
		// @Description Returns statistics for each email type for comparison charts
		// @Tags emailtracking
		// @Produce json
		// @Security BearerAuth
		// @Param start query string false "Start date (YYYY-MM-DD)"
		// @Param end query string false "End date (YYYY-MM-DD)"
		// @Success 200 {object} map[string]interface{}
		// @Failure 401 {object} fiber.Error "Unauthorized"
		// @Failure 403 {object} fiber.Error "Forbidden"
		rg.Get("/email/stats/bytype", emailtracking.StatsByType)

		// Top Clicked Links (authenticated, admin only)
		// @Router /email/stats/clicks [get]
		// @Summary Get top clicked links from emails
		// @Description Returns the most clicked links, normalized to remove user-specific data
		// @Tags emailtracking
		// @Produce json
		// @Security BearerAuth
		// @Param start query string false "Start date (YYYY-MM-DD)"
		// @Param end query string false "End date (YYYY-MM-DD)"
		// @Param limit query int false "Number of links to return (default 5, use 0 for all)"
		// @Success 200 {object} map[string]interface{}
		// @Failure 401 {object} fiber.Error "Unauthorized"
		// @Failure 403 {object} fiber.Error "Forbidden"
		rg.Get("/email/stats/clicks", emailtracking.TopClickedLinks)

		// Email Tracking for specific user (authenticated, admin only)
		// @Router /email/user/{id} [get]
		// @Summary Get email tracking for a user
		// @Description Returns email tracking records for a specific user (Support/Admin only)
		// @Tags emailtracking
		// @Produce json
		// @Security BearerAuth
		// @Param id path int true "User ID"
		// @Param limit query int false "Number of records (default 50)"
		// @Param offset query int false "Offset for pagination"
		// @Success 200 {object} map[string]interface{}
		// @Failure 401 {object} fiber.Error "Unauthorized"
		// @Failure 403 {object} fiber.Error "Forbidden"
		rg.Get("/email/user/:id", emailtracking.UserEmails)

		// Donations
		// @Router /donations [get]
		// @Summary Get donations
		// @Description Returns donation information
		// @Tags donations
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} donations.DonationsResponse
		rg.Get("/donations", donations.GetDonations)

		// @Router /donations [put]
		// @Summary Record external donation
		// @Description Records an external bank transfer donation
		// @Tags donations
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} map[string]interface{}
		rg.Put("/donations", donations.AddDonation)

		// Gift Aid
		// @Router /giftaid [get]
		// @Summary Get Gift Aid declaration
		// @Description Returns user's Gift Aid declaration
		// @Tags donations
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} donations.GiftAid
		rg.Get("/giftaid", donations.GetGiftAid)

		// Invitation
		// @Router /invitation [get]
		// @Summary List user invitations
		// @Tags invitation
		// @Produce json
		// @Security BearerAuth
		rg.Get("/invitation", invitation.ListInvitations)

		// @Router /invitation [put]
		// @Summary Send invitation email
		// @Tags invitation
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		rg.Put("/invitation", invitation.CreateInvitation)

		// @Router /invitation [patch]
		// @Summary Update invitation outcome
		// @Tags invitation
		// @Accept json
		// @Produce json
		rg.Patch("/invitation", invitation.UpdateOutcome)

		// Logo
		// @Router /logo [get]
		// @Summary Get logo
		// @Description Returns logo information
		// @Tags misc
		// @Produce json
		// @Success 200 {object} logo.LogoResponse
		rg.Get("/logo", logo.Get)

		// Microvolunteering
		// @Router /microvolunteering [get]
		// @Summary Get microvolunteering challenge
		// @Description Returns a microvolunteering challenge
		// @Tags microvolunteering
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} microvolunteering.Challenge
		rg.Get("/microvolunteering", microvolunteering.GetChallenge)

		// User by Email
		// @Router /user/byemail/{email} [get]
		// @Summary Get user by email
		// @Description Returns a user by email address
		// @Tags user
		// @Produce json
		// @Param email path string true "Email address"
		// @Security BearerAuth
		// @Success 200 {object} user.User
		rg.Get("/user/byemail/:email", user.GetUserByEmail)

		// Mark Notification Seen
		// @Router /notification/seen [post]
		// @Summary Mark notification as seen
		// @Description Marks a specific notification as seen
		// @Tags notification
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} map[string]interface{}
		rg.Post("/notification/seen", notification.Seen)

		// Mark All Notifications Seen
		// @Router /notification/allseen [post]
		// @Summary Mark all notifications as seen
		// @Description Marks all notifications as seen for the user
		// @Tags notification
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} map[string]interface{}
		rg.Post("/notification/allseen", notification.AllSeen)

		// Latest Message
		// @Router /latestmessage [get]
		// @Summary Get latest message timestamp
		// @Description Returns the timestamp of the latest message
		// @Tags message
		// @Produce json
		// @Success 200 {object} misc.LatestMessageResponse
		rg.Get("/latestmessage", misc.LatestMessage)

		// AI Illustration
		// @Router /illustration [get]
		// @Summary Get AI illustration for item
		// @Description Returns a cached AI-generated illustration for an item name. Returns ret=3 if not cached.
		// @Tags misc
		// @Produce json
		// @Param item query string true "Item name"
		// @Success 200 {object} misc.IllustrationResult
		rg.Get("/illustration", misc.GetIllustration)

		// Source Tracking
		// @Router /src [post]
		// @Summary Record source tracking
		// @Description Records source tracking data for analytics
		// @Tags misc
		// @Accept json
		// @Produce json
		// @Param source body src.SourceRequest true "Source tracking data"
		// @Success 200 {object} map[string]interface{}
		rg.Post("/src", src.RecordSource)

		// Memberships
		// @Router /memberships [put]
		// @Summary Join a group
		// @Description Adds the authenticated user to a group
		// @Tags membership
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} fiber.Map
		rg.Put("/memberships", membership.PutMemberships)

		// @Router /memberships [delete]
		// @Summary Leave a group
		// @Description Removes the authenticated user from a group
		// @Tags membership
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} fiber.Map
		rg.Delete("/memberships", membership.DeleteMemberships)

		// @Router /memberships [patch]
		// @Summary Update membership settings
		// @Description Updates email frequency, events allowed, volunteering allowed
		// @Tags membership
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} fiber.Map
		rg.Patch("/memberships", membership.PatchMemberships)

		// System Logs (moderator only)
		systemLogsGroup := rg.Group("/systemlogs")
		systemLogsGroup.Use(systemlogs.RequireModeratorMiddleware())
		// @Router /systemlogs [get]
		// @Summary Get system logs
		// @Description Returns system logs (moderator only)
		// @Tags systemlogs
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} systemlogs.LogsResponse
		systemLogsGroup.Get("", systemlogs.GetLogs)
		// @Router /systemlogs/counts [get]
		// @Summary Get log counts by subtype
		// @Description Returns counts of logs grouped by subtype using Loki metric queries (moderator only)
		// @Tags systemlogs
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} systemlogs.CountsResponse
		systemLogsGroup.Get("/counts", systemlogs.GetLogCounts)
	}

	// Delivery routes (public - no auth required for email client access)
	// Using bland paths to avoid privacy blocker detection
	delivery := app.Group("/e/d")

	// Pixel - returns 1x1 transparent GIF
	// @Router /e/d/p/{id} [get]
	// @Summary Delivery pixel
	// @Description Returns 1x1 transparent GIF
	// @Tags delivery
	// @Produce image/gif
	// @Param id path string true "ID"
	// @Success 200 {file} file
	delivery.Get("/p/:id", emailtracking.Pixel)

	// Redirect - handles link clicks and button actions
	// @Router /e/d/r/{id} [get]
	// @Summary Delivery redirect
	// @Description Redirects to destination URL
	// @Tags delivery
	// @Param id path string true "ID"
	// @Param url query string true "Base64 encoded destination URL"
	// @Param p query string false "Position"
	// @Param a query string false "Action type"
	// @Success 302 {string} string "Redirect"
	delivery.Get("/r/:id", emailtracking.Click)

	// Image - handles image loads for scroll depth
	// @Router /e/d/i/{id} [get]
	// @Summary Delivery image
	// @Description Redirects to original image
	// @Tags delivery
	// @Param id path string true "ID"
	// @Param url query string true "Base64 encoded image URL"
	// @Param p query string true "Position"
	// @Param s query integer false "Scroll percentage"
	// @Success 302 {string} string "Redirect"
	delivery.Get("/i/:id", emailtracking.Image)

	// Note: MDN read receipts come as emails and are processed by PHP incoming mail handler
	// The emailtracking.RecordMDNOpen() function can be called from PHP via internal API

	// AMP Email endpoints (public - token authenticated)
	// These endpoints support AMP for Email dynamic content and inline actions.
	// See: https://amp.dev/documentation/guides-and-tutorials/learn/cors-in-email
	ampGroup := app.Group("/amp")
	ampGroup.Use(amp.AMPCORSMiddleware())

	// Get chat messages for AMP email
	// @Router /amp/chat/{id} [get]
	// @Summary Get chat messages for AMP email
	// @Description Returns the last 5 chat messages for the "Earlier conversation" section
	// @Tags AMP
	// @Produce json
	// @Param id path int true "Chat ID"
	// @Param rt query string true "Read token (HMAC)"
	// @Param uid query int true "User ID"
	// @Param exp query int true "Token expiry timestamp"
	// @Param exclude query int false "Message ID to exclude (shown statically)"
	// @Param since query int false "Message ID - newer messages marked as NEW"
	// @Success 200 {object} amp.ChatResponse
	ampGroup.Get("/chat/:id", amp.GetChatMessages)

	// Post reply from AMP email
	// @Router /amp/chat/{id}/reply [post]
	// @Summary Post reply from AMP email
	// @Description Submits an inline reply from AMP email (one-time token)
	// @Tags AMP
	// @Accept json
	// @Produce json
	// @Param id path int true "Chat ID"
	// @Param wt query string true "Write token (one-time nonce)"
	// @Param body body object true "Message body with 'message' field"
	// @Success 200 {object} amp.ReplyResponse
	ampGroup.Post("/chat/:id/reply", amp.PostChatReply)
}
