package router

import (
	"github.com/freegle/iznik-server-go/address"
	"github.com/freegle/iznik-server-go/authority"
	"github.com/freegle/iznik-server-go/chat"
	"github.com/freegle/iznik-server-go/communityevent"
	"github.com/freegle/iznik-server-go/config"
	"github.com/freegle/iznik-server-go/group"
	"github.com/freegle/iznik-server-go/isochrone"
	"github.com/freegle/iznik-server-go/job"
	"github.com/freegle/iznik-server-go/location"
	"github.com/freegle/iznik-server-go/message"
	"github.com/freegle/iznik-server-go/misc"
	"github.com/freegle/iznik-server-go/newsfeed"
	"github.com/freegle/iznik-server-go/notification"
	"github.com/freegle/iznik-server-go/story"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/volunteering"
	"github.com/gofiber/fiber/v2"
)

func SetupRoutes(app *fiber.App) {
	// We have two groups because of how the API is used in the old and new clients.
	api := app.Group("/api")
	apiv2 := app.Group("/apiv2")

	for _, rg := range []fiber.Router{api, apiv2} {
		rg.Get("/address", address.ListForUser)
		rg.Get("/address/:id", address.GetAddress)
		rg.Get("/authority/:id/message", authority.Messages)
		rg.Get("/chat", chat.ListForUser)
		rg.Get("/chat/:id/message", chat.GetChatMessages)
		rg.Post("/chat/:id/message", chat.CreateChatMessage)
		rg.Post("/chat/lovejunk", chat.CreateChatMessageLoveJunk)
		rg.Get("/chat/:id", chat.GetChat)
		rg.Get("/communityevent", communityevent.List)
		rg.Get("/communityevent/group/:id", communityevent.ListGroup)
		rg.Get("/communityevent/:id", communityevent.Single)
		rg.Get("/config/:key", config.Get)
		rg.Get("/group", group.ListGroups)
		rg.Get("/group/:id", group.GetGroup)
		rg.Get("/group/:id/message", group.GetGroupMessages)
		rg.Get("/isochrone", isochrone.ListIsochrones)
		rg.Get("/isochrone/message", isochrone.Messages)
		rg.Get("/job", job.GetJobs)
		rg.Get("/job/:id", job.GetJob)
		rg.Get("/location/:id", location.GetLocation)
		rg.Get("/message/count", isochrone.Count)
		rg.Get("/message/inbounds", message.Bounds)
		rg.Get("/message/mygroups/:id?", message.Groups)
		rg.Get("/message/search/:term", message.Search)
		rg.Get("/message/:ids", message.GetMessages)
		rg.Get("/user/:id?", user.GetUser)
		rg.Get("/user/:id/publiclocation", user.GetPublicLocation)
		rg.Get("/user/:id/message", message.GetMessagesForUser)
		rg.Get("/user/:id/search", user.GetSearchesForUser)
		rg.Get("/newsfeed/:id", newsfeed.Single)
		rg.Get("/newsfeedcount", newsfeed.Count)
		rg.Get("/newsfeed", newsfeed.Feed)
		rg.Get("/notification/count", notification.Count)
		rg.Get("/notification", notification.List)
		rg.Get("/online", misc.Online)
		rg.Get("/story", story.List)
		rg.Get("/story/:id", story.Single)
		rg.Get("/story/group/:id", story.Group)
		rg.Get("/volunteering", volunteering.List)
		rg.Get("/volunteering/group/:id", volunteering.ListGroup)
		rg.Get("/volunteering/:id", volunteering.Single)
	}
}
