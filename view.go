package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// handleViewSession displays a specific chat session by session ID
func handleViewSession(ctx context.Context, event events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	// Extract session ID from path parameter
	// Expected format: /session/{sessionID}
	pathParts := strings.Split(strings.Trim(event.RawPath, "/"), "/")
	if len(pathParts) < 2 || pathParts[0] != "session" {
		return events.LambdaFunctionURLResponse{
			StatusCode: 400,
			Body:       "Invalid session URL format. Use /session/{sessionID}",
		}, nil
	}

	requestedSessionID := pathParts[1]
	if requestedSessionID == "" {
		return events.LambdaFunctionURLResponse{
			StatusCode: 400,
			Body:       "Session ID is required",
		}, nil
	}

	// Get the session from DynamoDB
	session, err := getSessionFromDynamoDB(ctx, requestedSessionID)
	if err != nil {
		log.Printf("Error retrieving session %s: %v", requestedSessionID, err)
		return events.LambdaFunctionURLResponse{
			StatusCode: 500,
			Body:       "Internal server error",
		}, nil
	}

	if session == nil {
		return events.LambdaFunctionURLResponse{
			StatusCode: 404,
			Body:       "Session not found",
		}, nil
	}

	// Parse and execute template
	tmpl, err := template.ParseFiles("templates/view.html")
	if err != nil {
		log.Printf("Template parsing error: %v", err)
		return events.LambdaFunctionURLResponse{
			StatusCode: 500,
			Body:       "Template error",
		}, nil
	}

	var buf strings.Builder
	err = tmpl.Execute(&buf, session)
	if err != nil {
		log.Printf("Template execution error: %v", err)
		return events.LambdaFunctionURLResponse{
			StatusCode: 500,
			Body:       "Template execution error",
		}, nil
	}

	headers := map[string]string{
		"Content-Type": "text/html; charset=utf-8",
	}

	// Set the session cookie to the viewed session
	headers["Set-Cookie"] = fmt.Sprintf("session_id=%s; Path=/; Max-Age=604800; HttpOnly; SameSite=Strict", session.SessionID)

	return events.LambdaFunctionURLResponse{
		StatusCode: 200,
		Headers:    headers,
		Body:       buf.String(),
	}, nil
}

// handleListSessions returns a JSON list of all available sessions (optional utility function)
func handleListSessions(ctx context.Context, event events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	// Note: This is a basic implementation. For production, you'd want to implement pagination
	// and possibly filter by user or time range

	// Scan DynamoDB table to get all sessions
	result, err := dynamoClient.Scan(ctx, &dynamodb.ScanInput{
		TableName:            aws.String(tableName),
		ProjectionExpression: aws.String("session_id, last_activity"),
	})

	if err != nil {
		log.Printf("Error scanning sessions: %v", err)
		return events.LambdaFunctionURLResponse{
			StatusCode: 500,
			Body:       "Internal server error",
		}, nil
	}

	type SessionSummary struct {
		SessionID    string    `json:"session_id"`
		LastActivity time.Time `json:"last_activity"`
	}

	var sessions []SessionSummary
	for _, item := range result.Items {
		var summary SessionSummary
		err = attributevalue.UnmarshalMap(item, &summary)
		if err != nil {
			log.Printf("Error unmarshaling session summary: %v", err)
			continue
		}
		sessions = append(sessions, summary)
	}

	// Sort by last activity (most recent first)
	for i := 0; i < len(sessions); i++ {
		for j := i + 1; j < len(sessions); j++ {
			if sessions[i].LastActivity.Before(sessions[j].LastActivity) {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}

	body, err := json.Marshal(map[string]interface{}{
		"sessions": sessions,
		"count":    len(sessions),
	})
	if err != nil {
		return events.LambdaFunctionURLResponse{
			StatusCode: 500,
			Body:       "Error encoding response",
		}, nil
	}

	return events.LambdaFunctionURLResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(body),
	}, nil
}
