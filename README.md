# ChatGPT Lambda Chat App

A serverless chat application built with Go, AWS Lambda, DynamoDB, and OpenAI's ChatGPT API. Features a modular function calling system that allows the AI to execute custom functions and tools.

## 🚀 Features

- **Serverless Architecture**: Built on AWS Lambda for automatic scaling and cost efficiency
- **Persistent Sessions**: Chat history stored in DynamoDB with automatic TTL cleanup
- **Function Calling**: Modular system for extending AI capabilities with custom functions
- **Session Management**: Cookie-based session tracking with secure HttpOnly cookies
- **Web Interface**: Clean HTML interface for chat interactions
- **Error Handling**: Robust error handling for API failures and function execution

## 🏗️ Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Web Browser   │────│  Lambda Function │────│   OpenAI API    │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                                │
                                │
                       ┌──────────────────┐
                       │    DynamoDB      │
                       │ (Session Store)  │
                       └──────────────────┘
```

## 📋 Prerequisites

- Go 1.21+
- AWS Account with appropriate permissions
- OpenAI API key
- AWS CLI configured (for deployment)

## 🛠️ Installation & Setup

### 1. Clone the Repository

```bash
git clone https://github.com/awoodward/chatgptlambda.git
cd chatgptlambda
```

### 2. Install Dependencies

```bash
go mod init chatgpt-lambda-chat
go get github.com/aws/aws-lambda-go/lambda
go get github.com/aws/aws-lambda-go/events
go get github.com/aws/aws-sdk-go-v2/config
go get github.com/aws/aws-sdk-go-v2/service/dynamodb
go get github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue
```

### 3. Create DynamoDB Table

```bash
aws dynamodb create-table \
    --table-name bedrock-chat-sessions \
    --attribute-definitions \
        AttributeName=session_id,AttributeType=S \
    --key-schema \
        AttributeName=session_id,KeyType=HASH \
    --billing-mode PAY_PER_REQUEST \
    --time-to-live-specification \
        AttributeName=ttl,Enabled=true
```

### 4. Build the Lambda Function

```bash
GOOS=linux GOARCH=amd64 go build -o bootstrap main.go
zip lambda.zip main templates/chat.html
```

### 5. Deploy to AWS Lambda

```bash
# Create the Lambda function
aws lambda create-function \
    --function-name chatgpt-chat-app \
    --runtime go1.x \
    --role arn:aws:iam::YOUR_ACCOUNT_ID:role/lambda-execution-role \
    --handler main \
    --zip-file fileb://lambda-function.zip

# Create Function URL
aws lambda create-function-url-config \
    --function-name chatgpt-chat-app \
    --auth-type NONE \
    --cors-config AllowCredentials=true,AllowMethods=GET,POST,AllowOrigins=*
```

## ⚙️ Configuration

Set the following environment variables in your Lambda function:

### Required
- `OPENAI_API_KEY`: Your OpenAI API key

### Optional
- `DYNAMODB_TABLE_NAME`: DynamoDB table name (default: "bedrock-chat-sessions")
- `OPENAI_MODEL`: OpenAI model to use (default: "gpt-3.5-turbo")
- `SYSTEM_MESSAGE`: System message for the AI (default: "You are a helpful assistant with access to various tools and functions.")

## 🔧 Function Calling System

### Built-in Functions

The app comes with three example functions:

1. **`get_current_time`**: Get current date and time with optional timezone
2. **`calculate`**: Perform basic math operations (add, subtract, multiply, divide)
3. **`get_weather`**: Get weather information (mock implementation)

### Adding Custom Functions

To add a new function, modify the `registerFunctions()` function in `main.go`:

```go
// Register your function
functionRegistry.Register("your_function_name", yourFunctionHandler, OpenAIFunction{
    Name:        "your_function_name",
    Description: "What your function does",
    Parameters: FunctionParameter{
        Type: "object",
        Properties: map[string]FunctionParameter{
            "param1": {
                Type:        "string",
                Description: "Description of parameter",
            },
        },
        Required: []string{"param1"},
    },
})

// Implement the function handler
func yourFunctionHandler(ctx context.Context, args map[string]interface{}) (interface{}, error) {
    param1 := args["param1"].(string)
    
    // Your logic here
    result := doSomething(param1)
    
    return map[string]interface{}{
        "result": result,
        "status": "success",
    }, nil
}
```

### Function Examples

Users can interact with functions naturally:

- "What time is it in Tokyo?" → Calls `get_current_time` with timezone
- "Calculate 15 * 23" → Calls `calculate` function
- "What's the weather in New York?" → Calls `get_weather` function

## 📁 Project Structure

```
chatgpt-lambda-chat/
├── main.go                # Main application code
├── registry.go            # Function registry code
├── functions.go           # Example functions and registrations 
├── templates/
│   └── chat.html          # HTML template for chat interface
├── go.mod                 # Go module file
├── README.md              # This file
└── lambda-function.zip    # Deployment package
```

## 🚀 API Endpoints

- `GET /` - Chat interface (HTML page)
- `POST /send` - Send a chat message
- `POST /clear` - Clear current chat session
- `GET /status` - Health check and function list

## 🔒 Security Features

- **HttpOnly Cookies**: Session cookies are HttpOnly and Secure
- **SameSite Protection**: CSRF protection with SameSite=Strict
- **TTL Cleanup**: Automatic session cleanup after 7 days
- **Error Isolation**: Function execution errors don't crash the app

## 📊 Monitoring

### CloudWatch Logs
Monitor your application through CloudWatch logs:
- Function execution logs
- Error tracking
- Performance metrics

### Status Endpoint
Check application health and available functions:
```bash
curl https://your-lambda-url/status
```

Response:
```json
{
  "status": "ok",
  "service": "chatgpt-chat-lambda",
  "functions": ["get_current_time", "calculate", "get_weather"]
}
```

## 🔧 Development

### Local Testing
```bash
# Run tests (if you add them)
go test ./...

# Build for local testing
go build -o chatgptlambda main.go
```

### Updating the Function
```bash
# Rebuild and update
GOOS=linux GOARCH=amd64 go build -o main main.go
zip lambda-function.zip main templates/chat.html

aws lambda update-function-code \
    --function-name chatgpt-chat-app \
    --zip-file fileb://lambda-function.zip
```

## 💰 Cost Considerations

- **Lambda**: Pay per request and execution time
- **DynamoDB**: Pay per read/write with automatic scaling
- **OpenAI API**: Pay per token usage
- **Data Transfer**: Minimal costs for web requests

Typical costs for moderate usage:
- Lambda: $0.01-$1.00/month
- DynamoDB: $0.25-$5.00/month
- OpenAI: Variable based on usage and model

## 🐛 Troubleshooting

### Common Issues

1. **OpenAI API Key Issues**
   - Ensure `OPENAI_API_KEY` environment variable is set
   - Verify API key has sufficient credits

2. **DynamoDB Permissions**
   - Ensure Lambda execution role has DynamoDB read/write permissions
   - Check table name matches `DYNAMODB_TABLE_NAME`

3. **Template Loading Issues**
   - Ensure `chat.html` is included in the deployment zip
   - Check file path in the zip archive

4. **Function Calling Errors**
   - Check CloudWatch logs for function execution errors
   - Verify function parameter types match schema

### Debug Mode
Enable verbose logging by checking CloudWatch logs for your Lambda function.

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- OpenAI for the ChatGPT API
- AWS for the serverless infrastructure
- The Go community for excellent AWS SDKs

## 🔗 Related Projects

- [AWS Lambda Go Runtime](https://github.com/aws/aws-lambda-go)
- [OpenAI Go Library](https://github.com/sashabaranov/go-openai) (alternative client)
- [AWS CDK for Go](https://github.com/aws/aws-cdk-go) (Infrastructure as Code)

---

For questions or support, please open an issue in the GitHub repository.