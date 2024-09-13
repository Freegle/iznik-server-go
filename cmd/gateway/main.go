package main

import (
	"context"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/freegle/iznik-server-go/adapter"
	"github.com/freegle/iznik-server-go/handler"
	"github.com/gofiber/fiber/v2"
	"time"
)

var fiberLambda *adapter.FiberLambda

func init() {
	app := fiber.New()
	app.Static("/", "./public")
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendFile("index")
	})
	app.Get("/api/:ip", handler.CacheRequest(10*time.Minute), handler.GeoLocation)

	fiberLambda = adapter.New(app)
}

// Handler proxies our app requests to aws lambda
func Handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// If no name is provided in the HTTP request body, throw an error
	return fiberLambda.ProxyWithContext(ctx, req)
}

func main() {
	//
	lambda.Start(Handler)
}
