package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"idempotency/idempotency"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(handler)
}

type Body struct {
	Name string `json:"name"`
}

func handler(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var body Body
	err := json.Unmarshal([]byte(event.Body), &body)
	if err != nil {
		return respond(err.Error(), http.StatusInternalServerError)
	}

	tableName := os.Getenv("TABLE_NAME")
	if tableName == "" {
		return respond("TABLE_NAME environment variable not found", http.StatusInternalServerError)
	}
	_, err = idempotency.Idempotent(ctx, body.Name, tableName, func() (string, error) {
		payload := bytes.NewReader([]byte(event.Body))
		fmt.Println("[handler] making the request")
		_, err := http.Post("https://webhook.site/04514f20-f564-49a2-8659-c3bbf5db8e16", "application/json", payload)
		if err != nil {
			fmt.Println("[handler] error", err.Error())
			return "", err
		}

		fmt.Println("[handler] returning the body.name")
		return body.Name, nil
	})
	if err != nil {
		fmt.Println("[handler] idempotent wrapper error")
		return respond(err.Error(), http.StatusInternalServerError)
	}

	fmt.Println("[handler] responding, everything is okay")
	return respond(http.StatusText(http.StatusOK), http.StatusOK)
}

func respond(body string, statusCode int) (events.APIGatewayProxyResponse, error) {
	return events.APIGatewayProxyResponse{
		Body:       body,
		StatusCode: statusCode,
	}, nil
}
