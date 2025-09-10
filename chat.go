package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
)

// Add this new handler
func handleSingleChat(ctx context.Context, event events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	type ChatRequest struct {
		Message       string `json:"message"`
		SystemMessage string `json:"system_message,omitempty"`
	}

	type ChatResponse struct {
		Message        string    `json:"message"`
		Response       string    `json:"response"`
		ProcessingTime int64     `json:"processing_time_ms"`
		RequestID      string    `json:"request_id"`
		ProcessedAt    time.Time `json:"processed_at"`
		Error          string    `json:"error,omitempty"`
	}

	startTime := time.Now()
	requestID := generateSessionID()

	// Parse request
	var chatReq ChatRequest
	body := event.Body
	if event.IsBase64Encoded {
		decodedBody, err := base64.StdEncoding.DecodeString(body)
		if err != nil {
			return events.LambdaFunctionURLResponse{
				StatusCode: 400,
				Headers:    map[string]string{"Content-Type": "application/json"},
				Body:       `{"error": "Invalid base64 encoding"}`,
			}, nil
		}
		body = string(decodedBody)
	}

	err := json.Unmarshal([]byte(body), &chatReq)
	if err != nil {
		return events.LambdaFunctionURLResponse{
			StatusCode: 400,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error": "Invalid JSON"}`,
		}, nil
	}

	if strings.TrimSpace(chatReq.Message) == "" {
		return events.LambdaFunctionURLResponse{
			StatusCode: 400,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error": "Message is required"}`,
		}, nil
	}

	// Create messages array
	messages := []Message{}

	if chatReq.SystemMessage != "" {
		messages = append(messages, Message{
			Content:   chatReq.SystemMessage,
			IsUser:    false,
			Timestamp: time.Now(),
		})
	}

	messages = append(messages, Message{
		Content:   chatReq.Message,
		IsUser:    true,
		Timestamp: time.Now(),
	})

	// Call OpenAI with full tool support
	response, err := callOpenAI(ctx, messages)

	processingTime := time.Since(startTime).Milliseconds()

	chatResponse := ChatResponse{
		Message:        chatReq.Message,
		RequestID:      requestID,
		ProcessedAt:    time.Now(),
		ProcessingTime: processingTime,
	}

	if err != nil {
		chatResponse.Error = err.Error()
	} else {
		chatResponse.Response = response
	}

	responseBody, err := json.Marshal(chatResponse)
	if err != nil {
		return events.LambdaFunctionURLResponse{
			StatusCode: 500,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error": "Failed to encode response"}`,
		}, nil
	}

	return events.LambdaFunctionURLResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(responseBody),
	}, nil
}
