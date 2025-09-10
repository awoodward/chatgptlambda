package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/aws/aws-lambda-go/events"
)

// handleSaveSession creates a copy of the current session with a new session ID
func handleSaveSession(ctx context.Context, event events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	sessionID := extractCookieValue(event.Headers, "session_id")

	if sessionID == "" {
		return events.LambdaFunctionURLResponse{
			StatusCode: 400,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error": "No active session to save"}`,
		}, nil
	}

	// Get the current session from DynamoDB
	session, err := getSessionFromDynamoDB(ctx, sessionID)
	if err != nil {
		log.Printf("Error retrieving session %s for saving: %v", sessionID, err)
		return events.LambdaFunctionURLResponse{
			StatusCode: 500,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error": "Failed to retrieve session"}`,
		}, nil
	}

	if session == nil {
		return events.LambdaFunctionURLResponse{
			StatusCode: 404,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error": "Session not found"}`,
		}, nil
	}

	// Create a new session with a new ID but same messages
	newSessionID := generateSessionID()
	newSession := &ChatSession{
		SessionID:    newSessionID,
		Messages:     make([]Message, len(session.Messages)), // Create a copy of messages
		LastActivity: time.Now(),
	}

	// Deep copy the messages to avoid reference issues
	copy(newSession.Messages, session.Messages)

	// Save the new session to DynamoDB
	err = saveSessionToDynamoDB(ctx, newSession)
	if err != nil {
		log.Printf("Error saving copied session: %v", err)
		return events.LambdaFunctionURLResponse{
			StatusCode: 500,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error": "Failed to save session copy"}`,
		}, nil
	}

	log.Printf("Session %s copied to new session %s", sessionID, newSessionID)

	// Return the new session ID
	response := map[string]interface{}{
		"success":             true,
		"new_session_id":      newSessionID,
		"message_count":       len(newSession.Messages),
		"original_session_id": sessionID,
	}

	body, err := json.Marshal(response)
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
		Body:       string(body),
	}, nil
}
