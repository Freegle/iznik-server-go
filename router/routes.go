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
	"github.com/freegle/iznik-server-go/address"
	"github.com/freegle/iznik-server-go/authority"
	"github.com/freegle/iznik-server-go/chat"
	"github.com/freegle/iznik-server-go/clientlog"
	"github.com/freegle/iznik-server-go/communityevent"
	"github.com/freegle/iznik-server-go/config"
	"github.com/freegle/iznik-server-go/donations"
	"github.com/freegle/iznik-server-go/group"
	"github.com/freegle/iznik-server-go/isochrone"
	"github.com/freegle/iznik-server-go/job"
	"github.com/freegle/iznik-server-go/location"
	"github.com/freegle/iznik-server-go/logo"
	"github.com/freegle/iznik-server-go/message"
	"github.com/freegle/iznik-server-go/microvolunteering"
	"github.com/freegle/iznik-server-go/misc"
	"github.com/freegle/iznik-server-go/newsfeed"
	"github.com/freegle/iznik-server-go/notification"
	"github.com/freegle/iznik-server-go/src"
	"github.com/freegle/iznik-server-go/story"
	"github.com/freegle/iznik-server-go/systemlogs"
	"github.com/freegle/iznik-server-go/user"
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
		// Message Activity
		// @Router /activity [get]
		// @Summary List recent posts and outcomes near you
		// @Description Returns the most recent message activity (new posts and outcomes) in groups near your location
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
		// @Summary List conversations for logged-in user
		// @Description Returns all chat conversations (about posts or direct messages) for the authenticated user
		// @Tags chat
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {array} chat.ChatRoom
		rg.Get("/chat", chat.ListForUser)

		// Chat Messages
		// @Router /chat/{id}/message [get]
		// @Summary Get messages in a conversation
		// @Description Returns all messages within a specific chat conversation
		// @Tags chat
		// @Produce json
		// @Param id path integer true "Chat ID"
		// @Security BearerAuth
		// @Success 200 {array} chat.ChatMessage
		rg.Get("/chat/:id/message", chat.GetChatMessages)

		// Create Chat Message
		// @Router /chat/{id}/message [post]
		// @Summary Send a message in a conversation
		// @Description Creates and sends a new message within an existing chat conversation
		// @Tags chat
		// @Accept json
		// @Produce json
		// @Param id path integer true "Chat ID"
		// @Param message body chat.ChatMessage true "Chat message object"
		// @Security BearerAuth
		// @Success 200 {object} chat.ChatMessage
		rg.Post("/chat/:id/message", chat.CreateChatMessage)

		// LoveJunk Chat
		// @Router /chat/lovejunk [post]
		// @Summary Create chat from LoveJunk integration
		// @Description Creates a new chat message initiated from LoveJunk partner integration
		// @Tags chat
		// @Accept json
		// @Produce json
		// @Param message body chat.ChatMessage true "Chat message object"
		// @Security BearerAuth
		// @Success 200 {object} chat.ChatMessage
		rg.Post("/chat/lovejunk", chat.CreateChatMessageLoveJunk)

		// Single Chat
		// @Router /chat/{id} [get]
		// @Summary Get conversation details
		// @Description Returns details of a specific chat conversation including participants and related post
		// @Tags chat
		// @Produce json
		// @Param id path integer true "Chat ID"
		// @Security BearerAuth
		// @Success 200 {object} chat.ChatRoom
		// @Failure 404 {object} fiber.Error "Chat not found"
		rg.Get("/chat/:id", chat.GetChat)

		// Community Events
		// @Router /communityevent [get]
		// @Summary List community events near you
		// @Description Returns upcoming community events (repair cafes, swap shops, etc) in groups near your location
		// @Tags communityevent
		// @Produce json
		// @Success 200 {array} communityevent.CommunityEvent
		rg.Get("/communityevent", communityevent.List)

		// Group Community Events
		// @Router /communityevent/group/{id} [get]
		// @Summary List community events for a Freegle group
		// @Description Returns upcoming community events posted to a specific Freegle group
		// @Tags communityevent
		// @Produce json
		// @Param id path integer true "Group ID"
		// @Success 200 {array} communityevent.CommunityEvent
		rg.Get("/communityevent/group/:id", communityevent.ListGroup)

		// Single Community Event
		// @Router /communityevent/{id} [get]
		// @Summary Get community event details
		// @Description Returns details of a specific community event including dates and location
		// @Tags communityevent
		// @Produce json
		// @Param id path integer true "Community Event ID"
		// @Success 200 {object} communityevent.CommunityEvent
		// @Failure 404 {object} fiber.Error "Community event not found"
		rg.Get("/communityevent/:id", communityevent.Single)

		// Config
		// @Router /config/{key} [get]
		// @Summary Get site configuration value
		// @Description Returns a specific configuration value by key (e.g. email domains, settings)
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
		// @Summary List all Freegle groups
		// @Description Returns all active Freegle groups with their locations and settings
		// @Tags group
		// @Produce json
		// @Success 200 {array} group.Group
		rg.Get("/group", group.ListGroups)

		// Single Group
		// @Router /group/{id} [get]
		// @Summary Get Freegle group details
		// @Description Returns details of a specific Freegle group including settings and statistics
		// @Tags group
		// @Produce json
		// @Param id path integer true "Group ID"
		// @Success 200 {object} group.Group
		// @Failure 404 {object} fiber.Error "Group not found"
		rg.Get("/group/:id", group.GetGroup)

		// Group Messages
		// @Router /group/{id}/message [get]
		// @Summary List active posts in a Freegle group
		// @Description Returns current OFFERs and WANTEDs posted to a specific Freegle group
		// @Tags group,message
		// @Produce json
		// @Param id path integer true "Group ID"
		// @Success 200 {array} message.Message
		rg.Get("/group/:id/message", group.GetGroupMessages)

		// Isochrones
		// @Router /isochrone [get]
		// @Summary List saved travel-time search areas
		// @Description Returns isochrone areas (regions reachable within set travel times) saved by the logged-in user
		// @Tags isochrone
		// @Produce json
		// @Success 200 {array} isochrone.Isochrone
		rg.Get("/isochrone", isochrone.ListIsochrones)

		// Isochrone Messages
		// @Router /isochrone/message [get]
		// @Summary List posts within travel-time search areas
		// @Description Returns OFFERs and WANTEDs within the user's saved isochrone travel-time areas
		// @Tags isochrone,message
		// @Produce json
		// @Success 200 {array} isochrone.Message
		rg.Get("/isochrone/message", isochrone.Messages)

		// Jobs
		// @Router /job [get]
		// @Summary List green job opportunities near you
		// @Description Returns environmental and sustainability job listings near the user's location
		// @Tags job
		// @Produce json
		// @Success 200 {array} job.Job
		rg.Get("/job", job.GetJobs)

		// Single Job
		// @Router /job/{id} [get]
		// @Summary Get green job details
		// @Description Returns details of a specific green/environmental job listing
		// @Tags job
		// @Produce json
		// @Param id path integer true "Job ID"
		// @Success 200 {object} job.Job
		// @Failure 404 {object} fiber.Error "Job not found"
		rg.Get("/job/:id", job.GetJob)

		// Record Job Click
		// @Router /job [post]
		// @Summary Record user clicked on a job listing
		// @Description Tracks when a user clicks through to view a green job listing (for analytics)
		// @Tags job
		// @Accept json
		// @Produce json
		// @Param id query string true "Job ID"
		// @Param link query string false "Job link URL"
		// @Success 200 {object} fiber.Map
		// @Failure 400 {object} fiber.Error "Bad Request"
		rg.Post("/job", job.RecordJobClick)

		// Donations
		// @Router /donations [get]
		// @Summary Get fundraising progress for current month
		// @Description Returns the monthly donation target and amount raised so far, optionally filtered by group
		// @Tags donations
		// @Accept json
		// @Produce json
		// @Param groupid query int false "Group ID to filter donations"
		// @Success 200 {object} map[string]interface{} "Donation summary with target and raised amounts"
		rg.Get("/donations", donations.GetDonations)

		// Gift Aid
		// @Router /giftaid [get]
		// @Summary Get your Gift Aid declaration status
		// @Description Returns whether the logged-in user has made a Gift Aid declaration for tax-efficient donations
		// @Tags donations
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} donations.GiftAid "User's Gift Aid declaration"
		// @Failure 401 {object} fiber.Map "Not logged in"
		// @Failure 404 {object} fiber.Map "No Gift Aid declaration found"
		rg.Get("/giftaid", donations.GetGiftAid)

		// Location by Lat/Lng
		// @Router /location/latlng [get]
		// @Summary Look up location from coordinates
		// @Description Returns location details (postcode, area name) for given latitude/longitude coordinates
		// @Tags location
		// @Produce json
		// @Param lat query number true "Latitude"
		// @Param lng query number true "Longitude"
		// @Success 200 {object} location.Location
		rg.Get("/location/latlng", location.LatLng)

		// Location Typeahead
		// @Router /location/typeahead [get]
		// @Summary Search for location by postcode or place name
		// @Description Returns location suggestions as user types a postcode or place name
		// @Tags location
		// @Produce json
		// @Param term query string true "Search term"
		// @Success 200 {array} location.Location
		rg.Get("/location/typeahead", location.Typeahead)

		// Location Addresses
		// @Router /location/{id}/addresses [get]
		// @Summary List addresses at a location
		// @Description Returns street addresses within a specific postcode/location area
		// @Tags location
		// @Produce json
		// @Param id path integer true "Location ID"
		// @Success 200 {array} address.Address
		rg.Get("/location/:id/addresses", location.GetLocationAddresses)

		// Single Location
		// @Router /location/{id} [get]
		// @Summary Get location details
		// @Description Returns details of a specific location including postcode, coordinates, and area name
		// @Tags location
		// @Produce json
		// @Param id path integer true "Location ID"
		// @Success 200 {object} location.Location
		// @Failure 404 {object} fiber.Error "Location not found"
		rg.Get("/location/:id", location.GetLocation)

		// Logo
		// @Router /logo [get]
		// @Summary Get special logo for today
		// @Description Returns a seasonal or event-specific logo if one is active for today's date
		// @Tags logo
		// @Produce json
		// @Success 200 {object} fiber.Map
		rg.Get("/logo", logo.Get)

		// Micro-volunteering Challenge
		// @Router /microvolunteering [get]
		// @Summary Get a quick volunteering task
		// @Description Returns a small task (like photo categorization) the user can complete to help Freegle
		// @Tags microvolunteering
		// @Accept json
		// @Produce json
		// @Param groupid query int false "Group ID to get challenges for"
		// @Param types query string false "Challenge types to include (comma-separated)"
		// @Security BearerAuth
		// @Success 200 {object} microvolunteering.Challenge
		// @Failure 401 {object} fiber.Map "Not logged in"
		rg.Get("/microvolunteering", microvolunteering.GetChallenge)

		// Message Count
		// @Router /message/count [get]
		// @Summary Count posts by type (OFFER/WANTED)
		// @Description Returns counts of active OFFERs and WANTEDs, optionally filtered by area
		// @Tags message
		// @Produce json
		// @Success 200 {object} isochrone.CountResult
		rg.Get("/message/count", isochrone.Count)

		// Message Bounds
		// @Router /message/inbounds [get]
		// @Summary List posts within map area
		// @Description Returns OFFERs and WANTEDs within the specified geographic bounding box (for map view)
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
		// @Summary List posts from your Freegle groups
		// @Description Returns OFFERs and WANTEDs from groups the logged-in user is a member of
		// @Tags message,group
		// @Produce json
		// @Param id path integer false "Group ID (optional)"
		// @Security BearerAuth
		// @Success 200 {array} message.Message
		rg.Get("/message/mygroups/:id?", message.Groups)

		// Message Search
		// @Router /message/search/{term} [get]
		// @Summary Search for items being offered or wanted
		// @Description Searches post titles and descriptions for matching items (e.g. "sofa", "books")
		// @Tags message
		// @Produce json
		// @Param term path string true "Search term"
		// @Param messagetype query string false "Message type filter"
		// @Param groupids query string false "Group IDs to filter by (comma separated)"
		// @Success 200 {array} message.SearchResult
		rg.Get("/message/search/:term", message.Search)

		// Messages by ID
		// @Router /message/{ids} [get]
		// @Summary Get post details
		// @Description Returns full details of one or more posts (OFFERs/WANTEDs) by their IDs
		// @Tags message
		// @Produce json
		// @Param ids path string true "Message IDs (comma separated)"
		// @Success 200 {array} message.Message
		// @Failure 404 {object} fiber.Error "Message not found"
		rg.Get("/message/:ids", message.GetMessages)

		// User
		// @Router /user/{id} [get]
		// @Summary Get user profile
		// @Description Returns user profile information - your own full profile if logged in, or another user's public profile
		// @Tags user
		// @Produce json
		// @Param id path integer false "User ID (optional)"
		// @Security BearerAuth
		// @Success 200 {object} user.User
		// @Failure 404 {object} fiber.Error "User not found"
		rg.Get("/user/:id?", user.GetUser)

		// User by Email
		// @Router /user/byemail/{email} [get]
		// @Summary Check if email is already registered
		// @Description Returns whether the email address is already registered (used during signup)
		// @Tags user
		// @Produce json
		// @Param email path string true "User email"
		// @Success 200 {object} fiber.Map "Returns {exists: boolean}"
		// @Failure 400 {object} fiber.Error "Email parameter required"
		rg.Get("/user/byemail/:email", user.GetUserByEmail)

		// User Public Location
		// @Router /user/{id}/publiclocation [get]
		// @Summary Get user's approximate location
		// @Description Returns the user's public location (area-level, not exact address) for display on posts
		// @Tags user
		// @Produce json
		// @Param id path integer true "User ID"
		// @Success 200 {object} location.Location
		rg.Get("/user/:id/publiclocation", user.GetPublicLocation)

		// User Messages
		// @Router /user/{id}/message [get]
		// @Summary List posts by a user
		// @Description Returns OFFERs and WANTEDs posted by a specific user, optionally filtered to active only
		// @Tags user,message
		// @Produce json
		// @Param id path integer true "User ID"
		// @Param active query boolean false "Only show active messages"
		// @Success 200 {array} message.MessageSummary
		rg.Get("/user/:id/message", message.GetMessagesForUser)

		// User Searches
		// @Router /user/{id}/search [get]
		// @Summary List saved search alerts
		// @Description Returns the user's saved search alerts that notify them of matching items
		// @Tags user
		// @Produce json
		// @Param id path integer true "User ID"
		// @Security BearerAuth
		// @Success 200 {array} user.Search
		rg.Get("/user/:id/search", user.GetSearchesForUser)

		// Newsfeed Item
		// @Router /newsfeed/{id} [get]
		// @Summary Get ChitChat post details
		// @Description Returns a single ChitChat (community discussion) post with its replies
		// @Tags newsfeed
		// @Produce json
		// @Param id path integer true "Newsfeed ID"
		// @Success 200 {object} newsfeed.Item
		// @Failure 404 {object} fiber.Error "Newsfeed item not found"
		rg.Get("/newsfeed/:id", newsfeed.Single)

		// Newsfeed Count
		// @Router /newsfeedcount [get]
		// @Summary Count unseen ChitChat items
		// @Description Returns count of unseen ChitChat (newsfeed) items for logged-in user
		// @Tags newsfeed
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} newsfeed.CountResult
		rg.Get("/newsfeedcount", newsfeed.Count)

		// Newsfeed
		// @Router /newsfeed [get]
		// @Summary List ChitChat posts in your area
		// @Description Returns community discussion posts (ChitChat) from groups near your location
		// @Tags newsfeed
		// @Produce json
		// @Success 200 {array} newsfeed.Item
		rg.Get("/newsfeed", newsfeed.Feed)

		// Notification Count
		// @Router /notification/count [get]
		// @Summary Count unread notifications
		// @Description Returns count of unseen notifications for the logged-in user
		// @Tags notification
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} notification.CountResult
		rg.Get("/notification/count", notification.Count)

		// Notifications
		// @Router /notification [get]
		// @Summary List your notifications
		// @Description Returns notifications (replies, mentions, etc) for the logged-in user
		// @Tags notification
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {array} notification.Notification
		rg.Get("/notification", notification.List)

		// Mark notification as seen
		// @Router /notification/seen [post]
		// @Summary Mark a notification as read
		// @Description Marks a specific notification as seen so it no longer appears as unread
		// @Tags notification
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Param body body notification.SeenRequest true "Notification ID"
		// @Success 200 {object} map[string]interface{}
		rg.Post("/notification/seen", notification.Seen)

		// Mark all notifications as seen
		// @Router /notification/allseen [post]
		// @Summary Mark all notifications as read
		// @Description Marks all notifications as seen for the logged-in user (clears unread count)
		// @Tags notification
		// @Produce json
		// @Security BearerAuth
		// @Success 200 {object} map[string]interface{}
		rg.Post("/notification/allseen", notification.AllSeen)

		// Online Status
		// @Router /online [get]
		// @Summary Check if API server is running
		// @Description Returns OK if the API server is online and responding (used for health checks)
		// @Tags misc
		// @Produce json
		// @Success 200 {object} misc.OnlineResult
		rg.Get("/online", misc.Online)

		// Latest Message
		// @Router /latestmessage [get]
		// @Summary Get time of most recent post (monitoring)
		// @Description Returns the timestamp of the most recent message arrival - used for monitoring that posts are being received
		// @Tags misc
		// @Produce json
		// @Success 200 {object} misc.LatestMessageResult
		rg.Get("/latestmessage", misc.LatestMessage)

		// Stories
		// @Router /story [get]
		// @Summary List freegling success stories
		// @Description Returns approved stories from freeglers about their positive experiences
		// @Tags story
		// @Produce json
		// @Success 200 {array} story.Story
		rg.Get("/story", story.List)

		// Single Story
		// @Router /story/{id} [get]
		// @Summary Get success story details
		// @Description Returns a specific freegling success story with full text
		// @Tags story
		// @Produce json
		// @Param id path integer true "Story ID"
		// @Success 200 {object} story.Story
		// @Failure 404 {object} fiber.Error "Story not found"
		rg.Get("/story/:id", story.Single)

		// Group Stories
		// @Router /story/group/{id} [get]
		// @Summary List success stories from a Freegle group
		// @Description Returns freegling success stories from members of a specific group
		// @Tags story,group
		// @Produce json
		// @Param id path integer true "Group ID"
		// @Success 200 {array} story.Story
		rg.Get("/story/group/:id", story.Group)

		// Volunteering Opportunities
		// @Router /volunteering [get]
		// @Summary List volunteering opportunities near you
		// @Description Returns environmental and community volunteering opportunities in groups near your location
		// @Tags volunteering
		// @Produce json
		// @Success 200 {array} volunteering.Volunteering
		rg.Get("/volunteering", volunteering.List)

		// Group Volunteering Opportunities
		// @Router /volunteering/group/{id} [get]
		// @Summary List volunteering opportunities in a Freegle group
		// @Description Returns volunteering opportunities posted to a specific Freegle group
		// @Tags volunteering,group
		// @Produce json
		// @Param id path integer true "Group ID"
		// @Success 200 {array} volunteering.Volunteering
		rg.Get("/volunteering/group/:id", volunteering.ListGroup)

		// Single Volunteering Opportunity
		// @Router /volunteering/{id} [get]
		// @Summary Get volunteering opportunity details
		// @Description Returns details of a specific volunteering opportunity including contact info
		// @Tags volunteering
		// @Produce json
		// @Param id path integer true "Volunteering ID"
		// @Success 200 {object} volunteering.Volunteering
		// @Failure 404 {object} fiber.Error "Volunteering opportunity not found"
		rg.Get("/volunteering/:id", volunteering.Single)

		// Source Tracking
		// @Router /src [post]
		// @Summary Track how user found the site
		// @Description Records the referral source (marketing campaign, partner link, etc) for analytics
		// @Tags tracking
		// @Accept json
		// @Produce json
		// @Param source body src.SourceRequest true "Source tracking data"
		// @Success 204 "No Content"
		// @Failure 400 {object} fiber.Map "Bad Request"
		rg.Post("/src", src.RecordSource)

		// Client Logs
		// @Router /clientlog [post]
		// @Summary Store browser-side logs for debugging
		// @Description Receives client-side log entries (errors, events) for distributed tracing and debugging
		// @Tags logging
		// @Accept json
		// @Produce json
		// @Param logs body clientlog.ClientLogRequest true "Client log entries"
		// @Success 204 "No Content"
		rg.Post("/clientlog", clientlog.ReceiveClientLogs)

		// System Logs (protected route group for moderators)
		systemLogsGroup := rg.Group("/systemlogs")
		systemLogsGroup.Use(systemlogs.RequireModeratorMiddleware())

		// @Router /systemlogs [get]
		// @Summary Search system logs for user activity
		// @Description Query logs from Loki to investigate user actions, API calls, and system events. Requires Moderator role.
		// @Tags logging
		// @Produce json
		// @Param sources query string false "Comma-separated sources: api,logs_table,client,email,batch"
		// @Param types query string false "Comma-separated log types: User,Message,Group,etc"
		// @Param subtypes query string false "Comma-separated subtypes: Login,Logout,etc"
		// @Param levels query string false "Comma-separated levels: info,warn,error,debug"
		// @Param search query string false "Text search in log messages"
		// @Param start query string false "Start time: relative (1m,1h,1d) or ISO8601"
		// @Param end query string false "End time: 'now' or ISO8601"
		// @Param limit query int false "Max results (default 100, max 1000)"
		// @Param direction query string false "Sort direction: backward or forward"
		// @Param userid query int false "Filter by user ID"
		// @Param groupid query int false "Filter by group ID"
		// @Param trace_id query string false "Filter by trace ID"
		// @Param session_id query string false "Filter by session ID"
		// @Security BearerAuth
		// @Success 200 {object} systemlogs.LogsResponse
		// @Failure 401 {object} fiber.Error "Authentication required"
		// @Failure 403 {object} fiber.Error "Moderator role required"
		systemLogsGroup.Get("", systemlogs.GetLogs)
	}
}
