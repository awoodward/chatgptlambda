package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamoTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Message represents a chat message
type Message struct {
	Content   string    `json:"content" dynamodbav:"content"`
	IsUser    bool      `json:"is_user" dynamodbav:"is_user"`
	Timestamp time.Time `json:"timestamp" dynamodbav:"timestamp"`
}

// ChatSession represents a chat session stored in DynamoDB
type ChatSession struct {
	SessionID    string    `json:"session_id" dynamodbav:"session_id"`
	Messages     []Message `json:"messages" dynamodbav:"messages"`
	LastActivity time.Time `json:"last_activity" dynamodbav:"last_activity"`
	TTL          int64     `json:"ttl" dynamodbav:"ttl"` // DynamoDB TTL
}

// PageData holds the data for template rendering
type PageData struct {
	Messages []Message
}

// OpenAI API structures
type OpenAIMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
	ToolChoice  string          `json:"tool_choice,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
}

type OpenAIChoice struct {
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
	Delta        *OpenAIDelta  `json:"delta,omitempty"`
}

type OpenAIDelta struct {
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Role      string     `json:"role,omitempty"`
}

type OpenAIResponse struct {
	Choices []OpenAIChoice `json:"choices"`
	Error   *OpenAIError   `json:"error,omitempty"`
	Usage   *OpenAIUsage   `json:"usage,omitempty"`
}

type OpenAIStreamResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
}

type OpenAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Streaming response data structures
type StreamData struct {
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`
	Done    bool   `json:"done,omitempty"`
}

var (
	dynamoClient     *dynamodb.Client
	tableName        string
	openaiAPIKey     string
	httpClient       *http.Client
	functionRegistry *FunctionRegistry
)

func init() {
	// Get table name from environment variable
	tableName = os.Getenv("DYNAMODB_TABLE_NAME")
	if tableName == "" {
		tableName = "bedrock-chat-sessions" // Default table name
	}

	// Get OpenAI API key from environment variable
	openaiAPIKey = os.Getenv("OPENAI_API_KEY")
	if openaiAPIKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable not set")
	}

	// Initialize AWS clients
	cfg, err := awsconfig.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatal("Error loading AWS config:", err)
	}

	dynamoClient = dynamodb.NewFromConfig(cfg)
	httpClient = &http.Client{
		Timeout: 60 * time.Second,
	}

	// Initialize function registry
	functionRegistry = NewFunctionRegistry()
	registerFunctions()

	log.Printf("Initialized AWS clients")
	log.Printf("DynamoDB table: %s", tableName)
	log.Printf("Registered %d functions", len(functionRegistry.schemas))
}

// generateSessionID creates a random session ID
func generateSessionID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// getSessionFromDynamoDB retrieves a session from DynamoDB
func getSessionFromDynamoDB(ctx context.Context, sessionID string) (*ChatSession, error) {
	result, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]dynamoTypes.AttributeValue{
			"session_id": &dynamoTypes.AttributeValueMemberS{Value: sessionID},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("error getting session from DynamoDB: %w", err)
	}

	if result.Item == nil {
		return nil, nil // Session not found
	}

	var session ChatSession
	err = attributevalue.UnmarshalMap(result.Item, &session)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling session: %w", err)
	}

	return &session, nil
}

// saveSessionToDynamoDB saves a session to DynamoDB
func saveSessionToDynamoDB(ctx context.Context, session *ChatSession) error {
	// Set TTL to 7 days from now
	session.TTL = time.Now().Add(7 * 24 * time.Hour).Unix()
	session.LastActivity = time.Now()

	item, err := attributevalue.MarshalMap(session)
	if err != nil {
		return fmt.Errorf("error marshaling session: %w", err)
	}

	_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})

	if err != nil {
		return fmt.Errorf("error saving session to DynamoDB: %w", err)
	}

	return nil
}

// deleteSessionFromDynamoDB removes a session from DynamoDB
func deleteSessionFromDynamoDB(ctx context.Context, sessionID string) error {
	_, err := dynamoClient.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(tableName),
		Key: map[string]dynamoTypes.AttributeValue{
			"session_id": &dynamoTypes.AttributeValueMemberS{Value: sessionID},
		},
	})

	if err != nil {
		return fmt.Errorf("error deleting session from DynamoDB: %w", err)
	}

	return nil
}

// getOrCreateSession gets an existing session or creates a new one
func getOrCreateSession(ctx context.Context, sessionID string) (*ChatSession, error) {
	if sessionID != "" {
		session, err := getSessionFromDynamoDB(ctx, sessionID)
		if err != nil {
			log.Printf("Error getting session from DynamoDB: %v", err)
		} else if session != nil {
			return session, nil
		}
	}

	// Create new session
	newSessionID := generateSessionID()

	session := &ChatSession{
		SessionID:    newSessionID,
		Messages:     make([]Message, 0),
		LastActivity: time.Now(),
	}

	return session, nil
}

// Fixed streamOpenAIResponse function with proper tool call handling
func streamOpenAIResponse(ctx context.Context, messages []Message, writer io.Writer, session *ChatSession) error {
	log.Println("Starting streaming OpenAI API call...")

	// Convert our messages to OpenAI format
	openaiMessages := make([]OpenAIMessage, 0)

	// Add system message
	systemMessage := os.Getenv("SYSTEM_MESSAGE")
	if systemMessage == "" {
		systemMessage = "You are a helpful assistant with access to various tools and functions."
	}
	openaiMessages = append(openaiMessages, OpenAIMessage{
		Role:    "system",
		Content: systemMessage,
	})

	// Add conversation history with validation
	for _, msg := range messages {
		role := "assistant"
		if msg.IsUser {
			role = "user"
		}

		content := strings.TrimSpace(msg.Content)
		if content == "" {
			log.Printf("Warning: Empty content found in message, skipping")
			continue
		}

		openaiMessages = append(openaiMessages, OpenAIMessage{
			Role:    role,
			Content: content,
		})
	}

	// Get model from environment variable
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4o-mini" // Default model
	}

	// Prepare tools
	tools := make([]OpenAITool, 0)
	schemas := functionRegistry.GetSchemas()
	for _, schema := range schemas {
		tools = append(tools, OpenAITool{
			Type:     "function",
			Function: schema,
		})
	}

	// For streaming with function calls, we need to handle this differently
	// OpenAI streaming with function calls is complex, so let's fall back to non-streaming for function calls
	if len(tools) > 0 {
		log.Println("Tools detected, using non-streaming mode for function calls")
		return streamNonStreamingResponse(ctx, openaiMessages, tools, model, writer, session)
	}

	// Continue with regular streaming for simple responses
	return streamRegularResponse(ctx, openaiMessages, model, writer, session)
}

// Handle non-streaming responses but send them as a stream
func streamNonStreamingResponse(ctx context.Context, openaiMessages []OpenAIMessage, tools []OpenAITool, model string, writer io.Writer, session *ChatSession) error {
	// Prepare request for non-streaming
	reqBody := OpenAIRequest{
		Model:       model,
		Messages:    openaiMessages,
		MaxTokens:   2000,
		Temperature: 0.7,
		Stream:      false, // Important: disable streaming for function calls
	}

	// Add tools
	reqBody.Tools = tools
	reqBody.ToolChoice = "auto"

	// Handle multiple rounds of tool calls
	maxIterations := 5
	totalResponse := ""

	for iteration := 0; iteration < maxIterations; iteration++ {
		response, err := makeOpenAIRequest(ctx, reqBody)
		if err != nil {
			errorData := StreamData{Error: err.Error()}
			jsonData, _ := json.Marshal(errorData)
			fmt.Fprintf(writer, "data: %s\n\n", string(jsonData))
			return err
		}

		if len(response.Choices) == 0 {
			errorData := StreamData{Error: "No response from API"}
			jsonData, _ := json.Marshal(errorData)
			fmt.Fprintf(writer, "data: %s\n\n", string(jsonData))
			return fmt.Errorf("no response from API")
		}

		choice := response.Choices[0]

		// If no tool calls, we're done
		if choice.FinishReason != "tool_calls" || len(choice.Message.ToolCalls) == 0 {
			// Stream the final response
			if choice.Message.Content != "" {
				totalResponse += choice.Message.Content
				// Send it as chunks to simulate streaming
				words := strings.Fields(choice.Message.Content)
				for _, word := range words {
					streamData := StreamData{Content: word + " "}
					jsonData, _ := json.Marshal(streamData)
					fmt.Fprintf(writer, "data: %s\n\n", string(jsonData))
					time.Sleep(50 * time.Millisecond) // Small delay to simulate streaming
				}
			}

			// Save to session
			if totalResponse != "" {
				session.Messages = append(session.Messages, Message{
					Content:   totalResponse,
					IsUser:    false,
					Timestamp: time.Now(),
				})
				saveSessionToDynamoDB(ctx, session)
			}

			return nil
		}

		// Handle function calls
		log.Printf("Function calls detected: %d (iteration %d)", len(choice.Message.ToolCalls), iteration+1)

		// Add assistant message with tool calls
		assistantMsg := choice.Message
		if assistantMsg.Content == "" {
			assistantMsg.Content = ""
		}
		openaiMessages = append(openaiMessages, assistantMsg)

		// Execute each tool call and stream the results
		for _, toolCall := range choice.Message.ToolCalls {
			log.Printf("Executing function: %s with args: %s", toolCall.Function.Name, toolCall.Function.Arguments)

			result, err := executeFunctionCall(ctx, toolCall)
			if err != nil {
				log.Printf("Function execution error: %v", err)
				result = fmt.Sprintf("Error executing function: %v", err)
			}

			// Stream the function result
			//functionResult := fmt.Sprintf("\n[Function %s]: %s\n", toolCall.Function.Name, result)
			functionResult := fmt.Sprintf("\n[Calling Function: %s]\n", toolCall.Function.Name)
			totalResponse += functionResult

			streamData := StreamData{Content: functionResult}
			jsonData, _ := json.Marshal(streamData)
			fmt.Fprintf(writer, "data: %s\n\n", string(jsonData))

			// Add function result message
			openaiMessages = append(openaiMessages, OpenAIMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: toolCall.ID,
			})
		}

		// Update request for next iteration
		reqBody.Messages = openaiMessages
	}

	// Save final response to session
	if totalResponse != "" {
		session.Messages = append(session.Messages, Message{
			Content:   totalResponse,
			IsUser:    false,
			Timestamp: time.Now(),
		})
		saveSessionToDynamoDB(ctx, session)
	}

	return nil
}

// Handle regular streaming responses (no function calls)
func streamRegularResponse(ctx context.Context, openaiMessages []OpenAIMessage, model string, writer io.Writer, session *ChatSession) error {
	// Prepare request for streaming
	reqBody := OpenAIRequest{
		Model:       model,
		Messages:    openaiMessages,
		MaxTokens:   2000,
		Temperature: 0.7,
		Stream:      true,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("error marshaling request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+openaiAPIKey)

	// Make request
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error making request to OpenAI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OpenAI API returned status %d: %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	var fullResponse strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := line[6:] // Remove "data: " prefix

			if data == "[DONE]" {
				break
			}

			var streamResponse OpenAIStreamResponse
			if err := json.Unmarshal([]byte(data), &streamResponse); err != nil {
				log.Printf("Error parsing stream response: %v", err)
				continue
			}

			if len(streamResponse.Choices) > 0 {
				choice := streamResponse.Choices[0]

				if choice.Delta != nil && choice.Delta.Content != "" {
					fullResponse.WriteString(choice.Delta.Content)

					// Send chunk to client
					streamData := StreamData{Content: choice.Delta.Content}
					jsonData, _ := json.Marshal(streamData)
					fmt.Fprintf(writer, "data: %s\n\n", string(jsonData))
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stream: %w", err)
	}

	// Save complete response to session
	if fullResponse.Len() > 0 {
		session.Messages = append(session.Messages, Message{
			Content:   fullResponse.String(),
			IsUser:    false,
			Timestamp: time.Now(),
		})

		if err := saveSessionToDynamoDB(ctx, session); err != nil {
			log.Printf("Error saving session after streaming: %v", err)
		}
	}

	return nil
}

// callOpenAI sends a message to OpenAI ChatGPT API (non-streaming fallback)
func callOpenAI(ctx context.Context, messages []Message) (string, error) {
	log.Println("Calling OpenAI API (non-streaming)...")
	// Convert our messages to OpenAI format
	openaiMessages := make([]OpenAIMessage, 0)

	// Add system message
	systemMessage := os.Getenv("SYSTEM_MESSAGE")
	if systemMessage == "" {
		systemMessage = "You are a helpful assistant with access to various tools and functions."
	}
	openaiMessages = append(openaiMessages, OpenAIMessage{
		Role:    "system",
		Content: systemMessage,
	})

	// Add conversation history with validation
	for _, msg := range messages {
		role := "assistant"
		if msg.IsUser {
			role = "user"
		}

		content := strings.TrimSpace(msg.Content)
		if content == "" {
			log.Printf("Warning: Empty content found in message, skipping")
			continue
		}

		openaiMessages = append(openaiMessages, OpenAIMessage{
			Role:    role,
			Content: content,
		})
	}

	// Get model from environment variable
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4o-mini" // Default model
	}

	// Prepare tools
	tools := make([]OpenAITool, 0)
	for _, schema := range functionRegistry.GetSchemas() {
		tools = append(tools, OpenAITool{
			Type:     "function",
			Function: schema,
		})
	}

	// Prepare request
	reqBody := OpenAIRequest{
		Model:       model,
		Messages:    openaiMessages,
		MaxTokens:   2000,
		Temperature: 0.7,
		Stream:      false,
	}

	// Add tools if any are available
	if len(tools) > 0 {
		reqBody.Tools = tools
		reqBody.ToolChoice = "auto"
	}

	// Handle multiple rounds of tool calls (existing logic)
	maxIterations := 5
	for iteration := 0; iteration < maxIterations; iteration++ {
		response, err := makeOpenAIRequest(ctx, reqBody)
		if err != nil {
			return "", err
		}

		if len(response.Choices) == 0 {
			return "I'm sorry, I didn't receive a response from the API.", nil
		}

		choice := response.Choices[0]

		if choice.FinishReason != "tool_calls" || len(choice.Message.ToolCalls) == 0 {
			return choice.Message.Content, nil
		}

		// Handle function calls
		assistantMsg := choice.Message
		if assistantMsg.Content == "" {
			assistantMsg.Content = ""
		}
		openaiMessages = append(openaiMessages, assistantMsg)

		for _, toolCall := range choice.Message.ToolCalls {
			result, err := executeFunctionCall(ctx, toolCall)
			if err != nil {
				result = fmt.Sprintf("Error executing function: %v", err)
			}

			openaiMessages = append(openaiMessages, OpenAIMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: toolCall.ID,
			})
		}

		reqBody.Messages = openaiMessages
	}

	return "I apologize, but I encountered an issue processing your request after multiple attempts.", nil
}

// makeOpenAIRequest makes an HTTP request to OpenAI API
func makeOpenAIRequest(ctx context.Context, reqBody OpenAIRequest) (*OpenAIResponse, error) {
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+openaiAPIKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request to OpenAI: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API returned status %d: %s", resp.StatusCode, string(body))
	}

	var openaiResp OpenAIResponse
	err = json.Unmarshal(body, &openaiResp)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	if openaiResp.Error != nil {
		return nil, fmt.Errorf("OpenAI API error: %s", openaiResp.Error.Message)
	}

	return &openaiResp, nil
}

// extractCookieValue extracts a cookie value from the headers
func extractCookieValue(headers map[string]string, cookieName string) string {
	cookieHeader, exists := headers["cookie"]
	if !exists {
		cookieHeader = headers["Cookie"]
	}

	if cookieHeader == "" {
		return ""
	}

	cookies := strings.Split(cookieHeader, ";")
	for _, cookie := range cookies {
		parts := strings.SplitN(strings.TrimSpace(cookie), "=", 2)
		if len(parts) == 2 && parts[0] == cookieName {
			return parts[1]
		}
	}
	return ""
}

// handleChatPage renders the chat interface
func handleChatPage(ctx context.Context, event events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	sessionID := extractCookieValue(event.Headers, "session_id")

	session, err := getOrCreateSession(ctx, sessionID)
	if err != nil {
		return events.LambdaFunctionURLResponse{
			StatusCode: 500,
			Body:       "Internal server error",
		}, nil
	}

	// Parse and execute template
	tmpl, err := template.ParseFiles("templates/chat.html")
	if err != nil {
		return events.LambdaFunctionURLResponse{
			StatusCode: 500,
			Body:       "Template error",
		}, nil
	}

	data := PageData{Messages: session.Messages}

	var buf strings.Builder
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return events.LambdaFunctionURLResponse{
			StatusCode: 500,
			Body:       "Template execution error",
		}, nil
	}

	headers := map[string]string{
		"Content-Type": "text/html; charset=utf-8",
	}

	// Set cookie if it's a new session
	if sessionID != session.SessionID {
		headers["Set-Cookie"] = fmt.Sprintf("session_id=%s; Path=/; Max-Age=604800; HttpOnly; SameSite=Strict", session.SessionID)
	}

	return events.LambdaFunctionURLResponse{
		StatusCode: 200,
		Headers:    headers,
		Body:       buf.String(),
	}, nil
}

// handleSendMessageStreaming processes a chat message with streaming response
func handleSendMessageStreaming(ctx context.Context, event events.LambdaFunctionURLRequest) (*events.LambdaFunctionURLStreamingResponse, error) {
	sessionID := extractCookieValue(event.Headers, "session_id")

	session, err := getOrCreateSession(ctx, sessionID)
	if err != nil {
		return &events.LambdaFunctionURLStreamingResponse{
			StatusCode: 500,
			Headers:    map[string]string{"Content-Type": "application/json"},
		}, nil
	}

	// Get message from query parameters
	message := strings.TrimSpace(event.QueryStringParameters["message"])
	if message == "" {
		return &events.LambdaFunctionURLStreamingResponse{
			StatusCode: 400,
			Headers:    map[string]string{"Content-Type": "application/json"},
		}, nil
	}

	// Add user message to session
	userMsg := Message{
		Content:   message,
		IsUser:    true,
		Timestamp: time.Now(),
	}
	session.Messages = append(session.Messages, userMsg)

	// Create response with SSE headers
	headers := map[string]string{
		"Content-Type":                "text/event-stream",
		"Cache-Control":               "no-cache",
		"Connection":                  "keep-alive",
		"Access-Control-Allow-Origin": "*",
	}

	// Set cookie if it's a new session
	if sessionID != session.SessionID {
		headers["Set-Cookie"] = fmt.Sprintf("session_id=%s; Path=/; Max-Age=604800; HttpOnly; SameSite=Strict", session.SessionID)
	}

	// Create pipe for streaming
	reader, writer := io.Pipe()

	// Start streaming in a goroutine
	go func() {
		defer writer.Close()

		// Stream the OpenAI response
		err := streamOpenAIResponse(ctx, session.Messages, writer, session)
		if err != nil {
			log.Printf("Error streaming OpenAI response: %v", err)
			errorData := StreamData{Error: err.Error()}
			jsonData, _ := json.Marshal(errorData)
			fmt.Fprintf(writer, "data: %s\n\n", string(jsonData))
		}

		// Send completion signal
		doneData := StreamData{Done: true}
		jsonData, _ := json.Marshal(doneData)
		fmt.Fprintf(writer, "data: %s\n\n", string(jsonData))
	}()

	return &events.LambdaFunctionURLStreamingResponse{
		StatusCode: 200,
		Headers:    headers,
		Body:       reader,
	}, nil
}

// handleSendMessage processes a chat message (non-streaming fallback)
func handleSendMessage(ctx context.Context, event events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	sessionID := extractCookieValue(event.Headers, "session_id")

	session, err := getOrCreateSession(ctx, sessionID)
	if err != nil {
		return events.LambdaFunctionURLResponse{
			StatusCode: 500,
			Body:       "Internal server error",
		}, nil
	}

	// Parse form data
	body := event.Body
	if event.IsBase64Encoded {
		decodedBody, err := base64.StdEncoding.DecodeString(body)
		if err != nil {
			return events.LambdaFunctionURLResponse{
				StatusCode: 400,
				Body:       "Error decoding request body",
			}, nil
		}
		body = string(decodedBody)
	}

	values, err := url.ParseQuery(body)
	if err != nil {
		return events.LambdaFunctionURLResponse{
			StatusCode: 400,
			Body:       "Invalid form data",
		}, nil
	}

	userMessage := strings.TrimSpace(values.Get("message"))
	if userMessage == "" {
		return events.LambdaFunctionURLResponse{
			StatusCode: 302,
			Headers:    map[string]string{"Location": "/"},
		}, nil
	}

	// Add user message to session
	userMsg := Message{
		Content:   userMessage,
		IsUser:    true,
		Timestamp: time.Now(),
	}
	session.Messages = append(session.Messages, userMsg)

	// Get bot response from OpenAI (non-streaming)
	botResponse, err := callOpenAI(ctx, session.Messages)
	if err != nil {
		log.Printf("Error calling OpenAI: %v", err)
		botResponse = "I'm sorry, I encountered an error processing your message. Please try again."
	}

	// Add bot response to session
	session.Messages = append(session.Messages, Message{
		Content:   botResponse,
		IsUser:    false,
		Timestamp: time.Now(),
	})

	// Save session
	err = saveSessionToDynamoDB(ctx, session)
	if err != nil {
		log.Printf("Error saving session: %v", err)
	}

	headers := map[string]string{"Location": "/"}

	// Set cookie if it's a new session
	if sessionID != session.SessionID {
		headers["Set-Cookie"] = fmt.Sprintf("session_id=%s; Path=/; Max-Age=604800; HttpOnly; SameSite=Strict", session.SessionID)
	}

	return events.LambdaFunctionURLResponse{
		StatusCode: 302,
		Headers:    headers,
	}, nil
}

// handleClearSession clears the current session
func handleClearSession(ctx context.Context, event events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	sessionID := extractCookieValue(event.Headers, "session_id")

	if sessionID != "" {
		err := deleteSessionFromDynamoDB(ctx, sessionID)
		if err != nil {
			log.Printf("Error deleting session: %v", err)
		}
	}

	return events.LambdaFunctionURLResponse{
		StatusCode: 302,
		Headers: map[string]string{
			"Location":   "/",
			"Set-Cookie": "session_id=; Path=/; Max-Age=-1; HttpOnly",
		},
	}, nil
}

// handleStatus provides basic status information
func handleStatus(ctx context.Context, event events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	functionList := make([]string, 0, len(functionRegistry.schemas))
	for name := range functionRegistry.schemas {
		functionList = append(functionList, name)
	}

	status := map[string]interface{}{
		"status":    "ok",
		"service":   "chatgpt-chat-lambda",
		"functions": functionList,
		"streaming": true,
	}

	body, _ := json.Marshal(status)

	return events.LambdaFunctionURLResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(body),
	}, nil
}

// handleRequest routes requests to appropriate handlers
func handleRequest(ctx context.Context, event events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	log.Printf("Request: %s %s", event.RequestContext.HTTP.Method, event.RawPath)

	switch {
	case event.RequestContext.HTTP.Method == "GET" && (event.RawPath == "/" || event.RawPath == ""):
		return handleChatPage(ctx, event)
	case event.RequestContext.HTTP.Method == "POST" && event.RawPath == "/send":
		return handleSendMessage(ctx, event)
	case event.RequestContext.HTTP.Method == "POST" && event.RawPath == "/clear":
		return handleClearSession(ctx, event)
	case event.RequestContext.HTTP.Method == "GET" && event.RawPath == "/status":
		return handleStatus(ctx, event)
	default:
		return events.LambdaFunctionURLResponse{
			StatusCode: 404,
			Body:       "Not Found",
		}, nil
	}
}

// Update your main handler to properly route streaming vs non-streaming
func handleStreamingRequest(ctx context.Context, event events.LambdaFunctionURLRequest) (*events.LambdaFunctionURLStreamingResponse, error) {
	log.Printf("Streaming Request: %s %s", event.RequestContext.HTTP.Method, event.RawPath)

	switch {
	case event.RequestContext.HTTP.Method == "GET" && event.RawPath == "/send-stream":
		return handleSendMessageStreaming(ctx, event)
	case event.RequestContext.HTTP.Method == "GET" && (event.RawPath == "/" || event.RawPath == ""):
		// Convert regular response to streaming response for the main page
		regularResponse, err := handleChatPage(ctx, event)
		if err != nil {
			return &events.LambdaFunctionURLStreamingResponse{
				StatusCode: 500,
				Headers:    map[string]string{"Content-Type": "text/plain"},
			}, err
		}

		// Convert to streaming response
		reader := strings.NewReader(regularResponse.Body)
		return &events.LambdaFunctionURLStreamingResponse{
			StatusCode: regularResponse.StatusCode,
			Headers:    regularResponse.Headers,
			Body:       reader,
		}, nil
	case event.RequestContext.HTTP.Method == "POST" && event.RawPath == "/send":
		// Convert regular response to streaming response for non-streaming messages
		regularResponse, err := handleSendMessage(ctx, event)
		if err != nil {
			return &events.LambdaFunctionURLStreamingResponse{
				StatusCode: 500,
				Headers:    map[string]string{"Content-Type": "text/plain"},
			}, err
		}

		// Convert to streaming response
		reader := strings.NewReader(regularResponse.Body)
		return &events.LambdaFunctionURLStreamingResponse{
			StatusCode: regularResponse.StatusCode,
			Headers:    regularResponse.Headers,
			Body:       reader,
		}, nil
	case event.RequestContext.HTTP.Method == "POST" && event.RawPath == "/clear":
		// Convert regular response to streaming response for clear session
		regularResponse, err := handleClearSession(ctx, event)
		if err != nil {
			return &events.LambdaFunctionURLStreamingResponse{
				StatusCode: 500,
				Headers:    map[string]string{"Content-Type": "text/plain"},
			}, err
		}

		// Convert to streaming response
		reader := strings.NewReader(regularResponse.Body)
		return &events.LambdaFunctionURLStreamingResponse{
			StatusCode: regularResponse.StatusCode,
			Headers:    regularResponse.Headers,
			Body:       reader,
		}, nil
	case event.RequestContext.HTTP.Method == "GET" && event.RawPath == "/status":
		// Convert regular response to streaming response for status
		regularResponse, err := handleStatus(ctx, event)
		if err != nil {
			return &events.LambdaFunctionURLStreamingResponse{
				StatusCode: 500,
				Headers:    map[string]string{"Content-Type": "text/plain"},
			}, err
		}

		// Convert to streaming response
		reader := strings.NewReader(regularResponse.Body)
		return &events.LambdaFunctionURLStreamingResponse{
			StatusCode: regularResponse.StatusCode,
			Headers:    regularResponse.Headers,
			Body:       reader,
		}, nil
	default:
		return &events.LambdaFunctionURLStreamingResponse{
			StatusCode: 404,
			Headers:    map[string]string{"Content-Type": "text/plain"},
			Body:       strings.NewReader("Not Found"),
		}, nil
	}
}

func main() {

	// Start with regular handler that supports streaming
	lambda.Start(handleStreamingRequest)
}
