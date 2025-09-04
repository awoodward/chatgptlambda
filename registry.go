package main

import (
	"context"
	"encoding/json"
	"fmt"
)

// Function calling structures
type FunctionParameter struct {
	Type        string                       `json:"type"`
	Description string                       `json:"description,omitempty"`
	Properties  map[string]FunctionParameter `json:"properties,omitempty"`
	Required    []string                     `json:"required,omitempty"`
	Items       *FunctionParameter           `json:"items,omitempty"`
	Enum        []string                     `json:"enum,omitempty"`
}

type OpenAIFunction struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Parameters  FunctionParameter `json:"parameters"`
}

type OpenAITool struct {
	Type     string         `json:"type"`
	Function OpenAIFunction `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// Function handler type
type FunctionHandler func(ctx context.Context, args map[string]interface{}) (interface{}, error)

// Function registry
type FunctionRegistry struct {
	functions map[string]FunctionHandler
	schemas   map[string]OpenAIFunction
}

func NewFunctionRegistry() *FunctionRegistry {
	return &FunctionRegistry{
		functions: make(map[string]FunctionHandler),
		schemas:   make(map[string]OpenAIFunction),
	}
}

func (fr *FunctionRegistry) Register(name string, handler FunctionHandler, schema OpenAIFunction) {
	fr.functions[name] = handler
	fr.schemas[name] = schema
}

func (fr *FunctionRegistry) GetHandler(name string) (FunctionHandler, bool) {
	handler, exists := fr.functions[name]
	return handler, exists
}

func (fr *FunctionRegistry) GetSchemas() []OpenAIFunction {
	schemas := make([]OpenAIFunction, 0, len(fr.schemas))
	for _, schema := range fr.schemas {
		schemas = append(schemas, schema)
	}
	return schemas
}

// executeFunctionCall executes a function call and returns the result
func executeFunctionCall(ctx context.Context, toolCall ToolCall) (string, error) {
	handler, exists := functionRegistry.GetHandler(toolCall.Function.Name)
	if !exists {
		return "", fmt.Errorf("unknown function: %s", toolCall.Function.Name)
	}

	// Parse function arguments
	var args map[string]interface{}
	if toolCall.Function.Arguments != "" {
		err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args)
		if err != nil {
			return "", fmt.Errorf("error parsing function arguments: %w", err)
		}
	} else {
		args = make(map[string]interface{})
	}

	// Execute function
	result, err := handler(ctx, args)
	if err != nil {
		return "", fmt.Errorf("error executing function %s: %w", toolCall.Function.Name, err)
	}

	// Convert result to JSON string
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("error marshaling function result: %w", err)
	}

	return string(resultJSON), nil
}
