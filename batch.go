package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/events"
)

// Add these new types and variables to your existing code

var (
	maxConcurrentWorkers int
)

func init() {
	// Add this to your existing init() function
	maxConcurrentWorkersStr := os.Getenv("MAX_CONCURRENT_WORKERS")
	if maxConcurrentWorkersStr == "" {
		maxConcurrentWorkersStr = "3" // Default to 3 concurrent workers
	}
	var err error
	maxConcurrentWorkers, err = strconv.Atoi(maxConcurrentWorkersStr)
	if err != nil {
		maxConcurrentWorkers = 3
	}
	log.Printf("Max concurrent workers set to: %d", maxConcurrentWorkers)
}

// Simplified batch processing types (no session dependencies)
type BatchRequest struct {
	Prompts        []string `json:"prompts"`
	SystemMessage  string   `json:"system_message,omitempty"`
	MaxWorkers     int      `json:"max_workers,omitempty"`
	DelayBetweenMS int      `json:"delay_between_ms,omitempty"`
}

type BatchResult struct {
	Index          int       `json:"index"`
	Prompt         string    `json:"prompt"`
	Response       string    `json:"response"`
	Error          string    `json:"error,omitempty"`
	ProcessingTime int64     `json:"processing_time_ms"`
	StartTime      time.Time `json:"start_time"`
	EndTime        time.Time `json:"end_time"`
}

type BatchResponse struct {
	Results     []BatchResult `json:"results"`
	Summary     BatchSummary  `json:"summary"`
	RequestID   string        `json:"request_id"`
	ProcessedAt time.Time     `json:"processed_at"`
}

type BatchSummary struct {
	Total       int           `json:"total"`
	Succeeded   int           `json:"succeeded"`
	Failed      int           `json:"failed"`
	TotalTime   time.Duration `json:"total_time_ms"`
	AverageTime time.Duration `json:"average_time_ms"`
	MaxWorkers  int           `json:"max_workers_used"`
}

// Worker pool for batch processing
type WorkerPool struct {
	maxWorkers   int
	jobs         chan BatchJob
	results      chan BatchResult
	ctx          context.Context
	delayBetween time.Duration
}

type BatchJob struct {
	Index         int
	Prompt        string
	SystemMessage string
}

// Create a new worker pool
func NewWorkerPool(ctx context.Context, maxWorkers int, delayBetween time.Duration) *WorkerPool {
	return &WorkerPool{
		maxWorkers:   maxWorkers,
		jobs:         make(chan BatchJob, maxWorkers*2),
		results:      make(chan BatchResult, maxWorkers*2),
		ctx:          ctx,
		delayBetween: delayBetween,
	}
}

// Start worker pool
func (wp *WorkerPool) Start() {
	for i := 0; i < wp.maxWorkers; i++ {
		go wp.worker(i)
	}
}

// Simplified worker function without session handling
func (wp *WorkerPool) worker(workerID int) {
	log.Printf("Worker %d started", workerID)

	for job := range wp.jobs {
		startTime := time.Now()

		log.Printf("Worker %d processing job %d", workerID, job.Index)

		// Add delay between requests for throttling
		if wp.delayBetween > 0 && job.Index > 0 {
			time.Sleep(wp.delayBetween)
		}

		result := BatchResult{
			Index:     job.Index,
			Prompt:    job.Prompt,
			StartTime: startTime,
		}

		// Create a simple messages array for OpenAI call
		messages := []Message{}

		// Add system message if provided
		if job.SystemMessage != "" {
			messages = append(messages, Message{
				Content:   job.SystemMessage,
				IsUser:    false,
				Timestamp: time.Now(),
			})
		}

		// Add user prompt
		messages = append(messages, Message{
			Content:   job.Prompt,
			IsUser:    true,
			Timestamp: time.Now(),
		})

		// Call OpenAI with full tool support
		response, err := callOpenAI(wp.ctx, messages)

		endTime := time.Now()
		result.EndTime = endTime
		result.ProcessingTime = endTime.Sub(startTime).Milliseconds()

		if err != nil {
			result.Error = err.Error()
			log.Printf("Worker %d job %d failed: %v", workerID, job.Index, err)
		} else {
			result.Response = response
			log.Printf("Worker %d job %d completed successfully", workerID, job.Index)
		}

		wp.results <- result
	}

	log.Printf("Worker %d finished", workerID)
}

// Add job to worker pool
func (wp *WorkerPool) AddJob(job BatchJob) {
	wp.jobs <- job
}

// Close worker pool
func (wp *WorkerPool) Close() {
	close(wp.jobs)
}

// Get result from worker pool
func (wp *WorkerPool) GetResult() BatchResult {
	return <-wp.results
}

// Simplified batch processing handler without session dependencies
func handleBatchProcess(ctx context.Context, event events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	startTime := time.Now()
	requestID := generateSessionID() // Reuse for request tracking

	log.Printf("Starting batch process request %s", requestID)

	// Parse request body
	var batchReq BatchRequest
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

	err := json.Unmarshal([]byte(body), &batchReq)
	if err != nil {
		return events.LambdaFunctionURLResponse{
			StatusCode: 400,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       fmt.Sprintf(`{"error": "Invalid JSON: %v"}`, err),
		}, nil
	}

	// Validate request
	if len(batchReq.Prompts) == 0 {
		return events.LambdaFunctionURLResponse{
			StatusCode: 400,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error": "No prompts provided"}`,
		}, nil
	}

	if len(batchReq.Prompts) > 100 {
		return events.LambdaFunctionURLResponse{
			StatusCode: 400,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error": "Too many prompts (max 100)"}`,
		}, nil
	}

	// Determine worker count
	workers := batchReq.MaxWorkers
	if workers <= 0 || workers > maxConcurrentWorkers {
		workers = maxConcurrentWorkers
	}

	// Determine delay between requests
	delay := time.Duration(batchReq.DelayBetweenMS) * time.Millisecond
	if delay < 0 {
		delay = 0
	}

	log.Printf("Processing %d prompts with %d workers, %v delay", len(batchReq.Prompts), workers, delay)

	// Create worker pool
	wp := NewWorkerPool(ctx, workers, delay)
	wp.Start()

	// Submit jobs
	for i, prompt := range batchReq.Prompts {
		job := BatchJob{
			Index:         i,
			Prompt:        prompt,
			SystemMessage: batchReq.SystemMessage,
		}
		wp.AddJob(job)
	}
	wp.Close()

	// Collect results
	results := make([]BatchResult, len(batchReq.Prompts))
	summary := BatchSummary{
		Total:      len(batchReq.Prompts),
		MaxWorkers: workers,
	}

	for i := 0; i < len(batchReq.Prompts); i++ {
		result := wp.GetResult()
		results[result.Index] = result

		if result.Error == "" {
			summary.Succeeded++
		} else {
			summary.Failed++
		}
	}

	// Calculate timing statistics
	totalTime := time.Since(startTime)
	summary.TotalTime = totalTime
	if summary.Succeeded > 0 {
		var totalProcessingTime int64
		for _, result := range results {
			if result.Error == "" {
				totalProcessingTime += result.ProcessingTime
			}
		}
		summary.AverageTime = time.Duration(totalProcessingTime/int64(summary.Succeeded)) * time.Millisecond
	}

	batchResponse := BatchResponse{
		Results:     results,
		Summary:     summary,
		RequestID:   requestID,
		ProcessedAt: time.Now(),
	}

	log.Printf("Batch request %s completed: %d succeeded, %d failed, took %v",
		requestID, summary.Succeeded, summary.Failed, totalTime)

	responseBody, err := json.Marshal(batchResponse)
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
