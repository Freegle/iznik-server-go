package message

import (
	"context"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	fiberadapter "github.com/awslabs/aws-lambda-go-api-proxy/fiber"
	"github.com/gofiber/fiber/v2"
	"log"
)

var fiberLambda *fiberadapter.FiberLambda

func init() {
	log.Printf("Fiber cold start")
	var app *fiber.App
	app = fiber.New()

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Hello, World!")
	})

	fiberLambda = fiberadapter.New(app)
}

func Handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	return fiberLambda.ProxyWithContext(ctx, req)
}

func main() {
	lambda.Start(Handler)
}
