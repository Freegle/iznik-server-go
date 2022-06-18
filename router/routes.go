package router

import (
	"github.com/freegle/iznik-server-go/group"
	"github.com/freegle/iznik-server-go/isochrone"
	"github.com/freegle/iznik-server-go/message"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

func SetupRoutes(app *fiber.App) {
	// TODO Can we avoid duplicating routes?
	api := app.Group("/api")
	api.Get("/group", group.ListGroups)
	api.Get("/group/:id", group.GetGroup)
	api.Get("/group/:id/message", group.GetGroupMessages)
	api.Get("/message/inbounds", message.Bounds)
	api.Get("/message/mygroups", message.Groups)
	api.Get("/message/:id", message.GetMessage)
	api.Get("/user/:id?", user.GetUser)
	api.Get("/isochrone", isochrone.ListIsochrones)
	api.Get("/isochrone/message", isochrone.Messages)

	apiv2 := app.Group("/apiv2")
	apiv2.Get("/group", group.ListGroups)
	apiv2.Get("/group/:id", group.GetGroup)
	apiv2.Get("/group/:id/message", group.GetGroupMessages)
	apiv2.Get("/message/inbounds", message.Bounds)
	apiv2.Get("/message/mygroups", message.Groups)
	apiv2.Get("/message/:id", message.GetMessage)
	apiv2.Get("/user/:id?", user.GetUser)
	apiv2.Get("/isochrone", isochrone.ListIsochrones)
	apiv2.Get("/isochrone/message", isochrone.Messages)
}
