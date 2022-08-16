package router

import (
	"github.com/freegle/iznik-server-go/address"
	"github.com/freegle/iznik-server-go/chat"
	"github.com/freegle/iznik-server-go/group"
	"github.com/freegle/iznik-server-go/isochrone"
	"github.com/freegle/iznik-server-go/message"
	"github.com/freegle/iznik-server-go/newsfeed"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

func SetupRoutes(app *fiber.App) {
	// We have two groups because of how the API is used in the old and new clients.
	api := app.Group("/api")
	apiv2 := app.Group("/apiv2")

	for _, rg := range []fiber.Router{api, apiv2} {
		rg.Get("/address", address.ListForUser)
		rg.Get("/chat", chat.ListForUser)
		rg.Get("/chat/:id", chat.GetChat)
		rg.Get("/chat/:id/message", chat.GetChatMessages)
		rg.Get("/group", group.ListGroups)
		rg.Get("/group/:id", group.GetGroup)
		rg.Get("/group/:id/message", group.GetGroupMessages)
		rg.Get("/isochrone", isochrone.ListIsochrones)
		rg.Get("/isochrone/message", isochrone.Messages)
		rg.Get("/message/inbounds", message.Bounds)
		rg.Get("/message/mygroups", message.Groups)
		rg.Get("/message/:id", message.GetMessage)
		rg.Get("/user/:id?", user.GetUser)
		rg.Get("/user/:id/message", message.GetMessagesForUser)
		rg.Get("/newsfeed", newsfeed.Feed)
	}
}
