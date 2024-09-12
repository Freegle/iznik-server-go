package main

import (
	"github.com/aws/aws-lambda-go/lambda"
)

type OnlineResult struct {
	Online bool `json:"online"`
}

func handler() (*OnlineResult, error) {
	var result OnlineResult
	result.Online = true
	return &result, nil
}

func main() {
	lambda.Start(handler)
}
