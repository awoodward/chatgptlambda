# Single Chat API

This document describes the `/chat` endpoint for the ChatGPT Lambda chat application. The chat API provides a stateless, single-request interface for processing individual prompts with full tool and function support.

## Overview

The `/chat` endpoint is designed for programmatic access to the AI chat functionality without requiring a web interface or session management. It processes a single prompt and returns the response immediately, making it ideal for API integrations, automation, and standalone applications.

### Key Features

- **Stateless Operation**: No session storage or persistence required
- **Full Tool Support**: Complete access to your function registry and OpenAI tools
- **JSON API**: Clean request/response format for easy integration
- **Error Handling**: Comprehensive error responses with details
- **Performance Metrics**: Processing time and request tracking
- **Flexible System Messages**: Custom system prompts per request

## API Endpoint

### POST /chat

Processes a single prompt and returns the AI response with full tool support.

#### Request Format

```json
{
  "message": "What is the current price of AAPL stock?",
  "system_message": "You are a helpful financial assistant with access to real-time market data"
}
```

#### Request Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `message` | string | Yes | The user prompt/question to process |
| `system_message` | string | No | Custom system message for this request |

#### Response Format

**Successful Response:**
```json
{
  "message": "What is the current price of AAPL stock?",
  "response": "The current price of AAPL (Apple Inc.) is $195.32, up 2.1% from yesterday's close. The stock is trading near its 52-week high with strong volume.",
  "processing_time_ms": 2340,
  "request_id": "chat_abc123def456",
  "processed_at": "2025-01-15T10:30:15Z"
}
```

**Error Response:**
```json
{
  "message": "What is the current price of AAPL stock?",
  "error": "Rate limit exceeded. Please try again in 60 seconds.",
  "processing_time_ms": 150,
  "request_id": "chat_xyz789ghi012",
  "processed_at": "2025-01-15T10:30:15Z"
}
```

#### Response Fields

- `message`: Echo of the original request message
- `response`: AI-generated response (if successful)
- `error`: Error description (if failed)
- `processing_time_ms`: Time taken to process the request
- `request_id`: Unique identifier for this request
- `processed_at`: Timestamp when processing completed

## Usage Examples

### Basic Chat Request

```bash
curl -X POST https://your-lambda-url/chat \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Explain quantum computing in simple terms"
  }'
```

### With Custom System Message

```bash
curl -X POST https://your-lambda-url/chat \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Analyze the quarterly earnings report",
    "system_message": "You are a senior financial analyst with expertise in earnings analysis. Provide detailed insights with specific metrics."
  }'
```

### Tool-Enabled Request

```bash
curl -X POST https://your-lambda-url/chat \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Get the current weather in New York and London, then compare them",
    "system_message": "You are a weather assistant with access to real-time weather data"
  }'
```

### Data Analysis Request

```bash
curl -X POST https://your-lambda-url/chat \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Look up the latest stock prices for MSFT, GOOGL, and AMZN, then calculate their market cap changes from last week",
    "system_message": "You are a financial data analyst with access to real-time market data and calculation tools"
  }'
```

## Error Handling

### HTTP Status Codes

- `200`: Success (response may still contain an error from AI processing)
- `400`: Bad request (invalid JSON, missing message, etc.)
- `500`: Server error (JSON encoding issues, internal failures)

### Common Error Scenarios

1. **Rate Limiting**: OpenAI API rate limits exceeded
2. **Tool Failures**: External data source unavailable
3. **Invalid Prompts**: Malformed or problematic input
4. **Timeout Errors**: Request processing exceeded time limits
5. **Authentication Issues**: API key problems

### Error Response Examples

**Rate Limit Error:**
```json
{
  "message": "Get stock data for 100 companies",
  "error": "Rate limit exceeded. Please try again in 60 seconds.",
  "processing_time_ms": 100,
  "request_id": "chat_rate_limit_001",
  "processed_at": "2025-01-15T10:30:15Z"
}
```

**Tool Error:**
```json
{
  "message": "Get weather for Mars",
  "error": "Weather data source error: Location not supported",
  "processing_time_ms": 1500,
  "request_id": "chat_tool_error_001",
  "processed_at": "2025-01-15T10:30:15Z"
}
```

## Integration Examples

### Python Client

```python
import requests
import json
from typing import Optional, Dict, Any

class ChatClient:
    def __init__(self, base_url: str):
        self.base_url = base_url.rstrip('/')
        
    def chat(self, message: str, system_message: Optional[str] = None) -> Dict[str, Any]:
        """Send a single chat message and get response."""
        url = f"{self.base_url}/chat"
        
        payload = {"message": message}
        if system_message:
            payload["system_message"] = system_message
            
        response = requests.post(url, json=payload)
        
        if response.status_code != 200:
            raise Exception(f"HTTP {response.status_code}: {response.text}")
            
        return response.json()
    
    def safe_chat(self, message: str, system_message: Optional[str] = None) -> tuple[Optional[str], Optional[str]]:
        """Send chat message with error handling. Returns (response, error)."""
        try:
            result = self.chat(message, system_message)
            if 'error' in result:
                return None, result['error']
            return result.get('response'), None
        except Exception as e:
            return None, str(e)

# Usage
client = ChatClient("https://your-lambda-url")

# Simple chat
result = client.chat("What's the capital of France?")
print(f"Response: {result['response']}")

# Chat with tools
result = client.chat(
    "Get current weather in Tokyo", 
    "You are a weather assistant"
)
print(f"Weather: {result['response']}")

# Safe chat with error handling
response, error = client.safe_chat("Analyze TSLA stock performance")
if error:
    print(f"Error: {error}")
else:
    print(f"Analysis: {response}")
```

### Node.js Client

```javascript
const axios = require('axios');

class ChatClient {
    constructor(baseUrl) {
        this.baseUrl = baseUrl.replace(/\/$/, '');
    }
    
    async chat(message, systemMessage = null) {
        const url = `${this.baseUrl}/chat`;
        const payload = { message };
        
        if (systemMessage) {
            payload.system_message = systemMessage;
        }
        
        try {
            const response = await axios.post(url, payload);
            return response.data;
        } catch (error) {
            if (error.response) {
                throw new Error(`HTTP ${error.response.status}: ${error.response.data}`);
            }
            throw error;
        }
    }
    
    async safeChat(message, systemMessage = null) {
        try {
            const result = await this.chat(message, systemMessage);
            if (result.error) {
                return { response: null, error: result.error };
            }
            return { response: result.response, error: null };
        } catch (error) {
            return { response: null, error: error.message };
        }
    }
}

// Usage
const client = new ChatClient('https://your-lambda-url');

// Simple chat
client.chat('What is machine learning?')
    .then(result => console.log('Response:', result.response))
    .catch(error => console.error('Error:', error.message));

// Chat with tools
client.chat(
    'Get the latest price of Bitcoin', 
    'You are a cryptocurrency analyst'
)
    .then(result => {
        if (result.error) {
            console.error('AI Error:', result.error);
        } else {
            console.log('Bitcoin Price:', result.response);
        }
    });

// Safe chat with error handling
(async () => {
    const { response, error } = await client.safeChat('Analyze market trends');
    if (error) {
        console.error('Failed:', error);
    } else {
        console.log('Analysis:', response);
    }
})();
```

### Bash/cURL Examples

```bash
#!/bin/bash

CHAT_URL="https://your-lambda-url/chat"

# Simple function to send chat message
send_chat() {
    local message="$1"
    local system_message="$2"
    
    local payload="{\"message\": \"$message\""
    if [ -n "$system_message" ]; then
        payload="$payload, \"system_message\": \"$system_message\""
    fi
    payload="$payload}"
    
    curl -X POST "$CHAT_URL" \
        -H "Content-Type: application/json" \
        -d "$payload" \
        -w "\nHTTP Status: %{http_code}\n"
}

# Usage examples
send_chat "What's the weather in Paris?"

send_chat "Analyze AAPL stock" "You are a financial analyst"

send_chat "Get latest crypto prices for BTC, ETH, ADA" "You are a crypto expert"
```

## Best Practices

### Request Design

1. **Clear Prompts**: Write specific, actionable prompts
2. **System Messages**: Use system messages to set context and behavior
3. **Tool Usage**: Leverage tools for data retrieval and analysis
4. **Error Handling**: Always check for errors in responses

### Performance Optimization

1. **Prompt Length**: Keep prompts focused to reduce token usage
2. **Parallel Requests**: Use batch processing for multiple related prompts
3. **Caching**: Cache responses for repeated identical requests
4. **Timeouts**: Set appropriate timeout values for your client

### Error Handling Strategies

1. **Retry Logic**: Implement exponential backoff for rate limits
2. **Validation**: Validate inputs before sending requests
3. **Graceful Degradation**: Handle partial failures appropriately
4. **Logging**: Log request IDs for debugging

### Security Considerations

1. **Input Validation**: Sanitize user inputs before sending
2. **Rate Limiting**: Implement client-side rate limiting
3. **Authentication**: Secure your Lambda endpoint appropriately
4. **Data Privacy**: Don't send sensitive data unless necessary

## Comparison with Other Endpoints

| Feature | `/chat` | `/batch` | `/send` |
|---------|---------|----------|---------|
| **Use Case** | Single API request | Multiple concurrent requests | Web UI interaction |
| **Input Format** | JSON | JSON | Form data |
| **Output Format** | JSON | JSON | HTML redirect |
| **Session Management** | Stateless | Stateless | Session-based |
| **Tool Support** | Full | Full | Full |
| **Concurrency** | Single request | Configurable workers | Single request |
| **Integration** | API clients | Batch processing | Web browsers |

## Troubleshooting

### Common Issues

1. **Empty Responses**: Check if message field is properly set
2. **Tool Failures**: Verify external data sources are accessible
3. **Rate Limits**: Implement proper retry logic with backoff
4. **Timeouts**: Consider breaking complex requests into smaller parts

### Debugging Tips

1. **Request IDs**: Use request IDs from responses for tracking
2. **Processing Times**: Monitor processing times for performance issues
3. **Error Messages**: Check specific error messages for root causes
4. **Lambda Logs**: Review Lambda logs for detailed error information

### Performance Monitoring

Track these metrics for optimal performance:

- **Response Times**: Average processing time per request
- **Error Rates**: Percentage of failed requests
- **Tool Usage**: Which tools are called most frequently
- **Token Consumption**: Monitor OpenAI token usage

This endpoint provides a clean, stateless way to integrate AI chat functionality into any application while maintaining full access to your tools and functions.