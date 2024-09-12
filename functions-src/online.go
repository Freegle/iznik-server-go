package main

import (
	"encoding/json"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

type OnlineResult struct {
	Online bool `json:"online"`
}

func handler(request events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {
	var result OnlineResult
	result.Online = true
	rsp, _ := json.Marshal(result)

	return &events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       string(rsp),
	}, nil
}

func main() {
	lambda.Start(handler)
}
