package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Register all available functions
func registerFunctions() {
	// Example: Get current time function
	functionRegistry.Register("get_current_time", getCurrentTime, OpenAIFunction{
		Name:        "get_current_time",
		Description: "Get the current date and time",
		Parameters: FunctionParameter{
			Type: "object",
			Properties: map[string]FunctionParameter{
				"timezone": {
					Type:        "string",
					Description: "Timezone (optional, defaults to UTC)",
				},
			},
		},
	})

	// Example: Calculator function
	functionRegistry.Register("calculate", calculate, OpenAIFunction{
		Name:        "calculate",
		Description: "Perform basic mathematical calculations",
		Parameters: FunctionParameter{
			Type: "object",
			Properties: map[string]FunctionParameter{
				"operation": {
					Type:        "string",
					Description: "The mathematical operation to perform",
					Enum:        []string{"add", "subtract", "multiply", "divide"},
				},
				"a": {
					Type:        "number",
					Description: "First number",
				},
				"b": {
					Type:        "number",
					Description: "Second number",
				},
			},
			Required: []string{"operation", "a", "b"},
		},
	})

	// Example: Weather function (mock)
	functionRegistry.Register("get_weather", getWeather, OpenAIFunction{
		Name:        "get_weather",
		Description: "Get current weather information for a location",
		Parameters: FunctionParameter{
			Type: "object",
			Properties: map[string]FunctionParameter{
				"location": {
					Type:        "string",
					Description: "The city and state, e.g. San Francisco, CA",
				},
				"unit": {
					Type:        "string",
					Description: "Temperature unit",
					Enum:        []string{"celsius", "fahrenheit"},
				},
			},
			Required: []string{"location"},
		},
	})
}

// Function implementations
func getCurrentTime(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	timezone := "UTC"
	if tz, ok := args["timezone"].(string); ok && tz != "" {
		timezone = tz
	}

	var loc *time.Location
	var err error
	if timezone != "UTC" {
		loc, err = time.LoadLocation(timezone)
		if err != nil {
			return nil, fmt.Errorf("invalid timezone: %s", timezone)
		}
	} else {
		loc = time.UTC
	}

	now := time.Now().In(loc)
	return map[string]interface{}{
		"current_time": now.Format("2006-01-02 15:04:05"),
		"timezone":     timezone,
		"day_of_week":  now.Weekday().String(),
	}, nil
}

func calculate(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	operation, ok := args["operation"].(string)
	if !ok {
		return nil, fmt.Errorf("operation is required")
	}

	a, ok := args["a"].(float64)
	if !ok {
		return nil, fmt.Errorf("parameter 'a' must be a number")
	}

	b, ok := args["b"].(float64)
	if !ok {
		return nil, fmt.Errorf("parameter 'b' must be a number")
	}

	var result float64
	switch operation {
	case "add":
		result = a + b
	case "subtract":
		result = a - b
	case "multiply":
		result = a * b
	case "divide":
		if b == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		result = a / b
	default:
		return nil, fmt.Errorf("unsupported operation: %s", operation)
	}

	return map[string]interface{}{
		"result":    result,
		"operation": operation,
		"a":         a,
		"b":         b,
	}, nil
}

func getWeather(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	location, ok := args["location"].(string)
	if !ok {
		return nil, fmt.Errorf("location is required")
	}

	unit := "fahrenheit"
	if u, ok := args["unit"].(string); ok && u != "" {
		unit = u
	}

	// Mock weather data - in a real implementation, you'd call a weather API
	temp := 72.0
	if unit == "celsius" {
		temp = 22.0
	}

	return map[string]interface{}{
		"location":    location,
		"temperature": temp,
		"unit":        unit,
		"condition":   "sunny",
		"humidity":    60,
		"description": fmt.Sprintf("It's currently %s and %g°%s in %s", "sunny", temp, strings.ToUpper(unit[:1]), location),
	}, nil
}
