# Batch Processing API

This document describes the batch processing functionality for the ChatGPT Lambda chat application. The batch processor allows you to submit multiple prompts for concurrent processing while maintaining full access to tools and functions.

## Overview

The batch processing system processes multiple prompts concurrently using a configurable worker pool. Each prompt is processed independently with full tool/function calling support, making it ideal for tasks that require data retrieval, analysis, or other tool-based operations.

### Key Features

- **Concurrent Processing**: Configurable worker pools (1-N workers)
- **Rate Limiting**: Built-in delays between requests to respect API limits
- **Tool Support**: Full access to your function registry and OpenAI tools
- **Throttling Control**: Configurable delays to respect data provider limits
- **Stateless**: No session storage - results returned directly
- **Error Handling**: Individual prompt failures don't affect the batch
- **Performance Metrics**: Detailed timing and success statistics

## Configuration

### Environment Variables

Set these in your Lambda environment:

```bash
MAX_CONCURRENT_WORKERS=5  # Maximum workers allowed (default: 3)
```

### Limits

- Maximum prompts per batch: 100
- Lambda timeout considerations apply (15 minutes max)
- OpenAI rate limits apply per worker

## API Endpoint

### POST /batch

Processes multiple prompts concurrently and returns all results.

#### Request Format

```json
{
  "prompts": [
    "Analyze the current stock market trends",
    "Get weather data for New York",
    "Search for recent AI research papers"
  ],
  "system_message": "You are a helpful research assistant",
  "max_workers": 3,
  "delay_between_ms": 1000
}
```

#### Request Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompts` | array[string] | Yes | Array of prompts to process (max 100) |
| `system_message` | string | No | System message applied to all prompts |
| `max_workers` | integer | No | Number of concurrent workers (max: env limit) |
| `delay_between_ms` | integer | No | Delay between requests in milliseconds |

#### Response Format

```json
{
  "results": [
    {
      "index": 0,
      "prompt": "Analyze the current stock market trends",
      "response": "Based on current market data...",
      "processing_time_ms": 2340,
      "start_time": "2025-01-15T10:30:00Z",
      "end_time": "2025-01-15T10:30:02Z"
    },
    {
      "index": 1,
      "prompt": "Get weather data for New York",
      "response": "Current weather in New York...",
      "processing_time_ms": 1890,
      "start_time": "2025-01-15T10:30:01Z",
      "end_time": "2025-01-15T10:30:03Z"
    },
    {
      "index": 2,
      "prompt": "Search for recent AI research papers",
      "error": "Rate limit exceeded",
      "processing_time_ms": 100,
      "start_time": "2025-01-15T10:30:02Z",
      "end_time": "2025-01-15T10:30:02Z"
    }
  ],
  "summary": {
    "total": 3,
    "succeeded": 2,
    "failed": 1,
    "total_time_ms": 5200,
    "average_time_ms": 2115,
    "max_workers_used": 3
  },
  "request_id": "batch_abc123def456",
  "processed_at": "2025-01-15T10:30:05Z"
}
```

#### Response Fields

**Result Object:**
- `index`: Original position in the prompts array
- `prompt`: The original prompt text
- `response`: AI response (if successful)
- `error`: Error message (if failed)
- `processing_time_ms`: Time taken to process this prompt
- `start_time`: When processing started
- `end_time`: When processing completed

**Summary Object:**
- `total`: Total number of prompts
- `succeeded`: Number of successful responses
- `failed`: Number of failed responses
- `total_time_ms`: Total batch processing time
- `average_time_ms`: Average processing time for successful prompts
- `max_workers_used`: Number of workers that were used

## Usage Examples

### Basic Batch Request

```bash
curl -X POST https://your-lambda-url/batch \
  -H "Content-Type: application/json" \
  -d '{
    "prompts": [
      "What is 2+2?",
      "Explain machine learning briefly",
      "List 3 programming languages"
    ]
  }'
```

### With Rate Limiting

```bash
curl -X POST https://your-lambda-url/batch \
  -H "Content-Type: application/json" \
  -d '{
    "prompts": [
      "Get current price of AAPL stock",
      "Get current price of MSFT stock",
      "Get current price of GOOGL stock"
    ],
    "max_workers": 2,
    "delay_between_ms": 500,
    "system_message": "Provide current stock prices with timestamps"
  }'
```

### Data Analysis Batch

```bash
curl -X POST https://your-lambda-url/batch \
  -H "Content-Type: application/json" \
  -d '{
    "prompts": [
      "Analyze Q4 sales data trends",
      "Generate customer satisfaction report",
      "Review inventory levels by category",
      "Calculate monthly revenue growth"
    ],
    "max_workers": 2,
    "delay_between_ms": 1000,
    "system_message": "You are a business analyst. Provide detailed insights."
  }'
```

## Error Handling

### HTTP Status Codes

- `200`: Success (some individual prompts may still have failed)
- `400`: Bad request (invalid JSON, no prompts, too many prompts)
- `500`: Server error

### Individual Prompt Errors

Individual prompts can fail without affecting the entire batch. Common error scenarios:

- OpenAI rate limits
- Tool/function execution failures
- Timeout errors
- Invalid prompts

Check the `error` field in each result object for details.

## Performance Considerations

### Concurrency

- Higher worker count = faster processing but more resource usage
- Consider your data provider's rate limits when setting worker count
- OpenAI rate limits apply per worker

### Delays

- Use `delay_between_ms` to respect rate limits
- Balance between speed and staying within limits
- Monitor your data provider's rate limit responses

### Lambda Limits

- 15-minute maximum execution time
- Memory limits may affect large batches
- Consider breaking very large batches into smaller chunks

## Best Practices

### Rate Limiting

1. Start with conservative settings (2-3 workers, 1000ms delay)
2. Monitor error rates and adjust accordingly
3. Implement exponential backoff for rate limit errors
4. Consider your data provider's specific limits

### Prompt Design

1. Keep prompts focused and specific
2. Use consistent system messages for related prompts
3. Consider prompt length for token usage
4. Test individual prompts before batching

### Error Handling

1. Always check both HTTP status and individual result errors
2. Implement retry logic for transient failures
3. Log failed prompts for debugging
4. Consider partial success scenarios

### Monitoring

1. Track success/failure rates
2. Monitor processing times
3. Watch for rate limit patterns
4. Set up alerts for high failure rates

## Integration Examples

### Python Client

```python
import requests
import json

def batch_process(prompts, max_workers=3, delay_ms=1000):
    url = "https://your-lambda-url/batch"
    payload = {
        "prompts": prompts,
        "max_workers": max_workers,
        "delay_between_ms": delay_ms
    }
    
    response = requests.post(url, json=payload)
    
    if response.status_code == 200:
        return response.json()
    else:
        raise Exception(f"Batch processing failed: {response.text}")

# Usage
prompts = [
    "Analyze sales data for Q1",
    "Generate marketing report",
    "Review customer feedback"
]

results = batch_process(prompts, max_workers=2, delay_ms=500)
print(f"Processed {results['summary']['succeeded']} of {results['summary']['total']} prompts")
```

### Node.js Client

```javascript
const axios = require('axios');

async function batchProcess(prompts, options = {}) {
    const url = 'https://your-lambda-url/batch';
    const payload = {
        prompts,
        max_workers: options.maxWorkers || 3,
        delay_between_ms: options.delayMs || 1000,
        system_message: options.systemMessage || ''
    };
    
    try {
        const response = await axios.post(url, payload);
        return response.data;
    } catch (error) {
        throw new Error(`Batch processing failed: ${error.message}`);
    }
}

// Usage
const prompts = [
    'Get weather for London',
    'Get weather for Paris',
    'Get weather for Tokyo'
];

batchProcess(prompts, { maxWorkers: 2, delayMs: 500 })
    .then(results => {
        console.log(`Successfully processed ${results.summary.succeeded} prompts`);
        results.results.forEach(result => {
            if (result.error) {
                console.error(`Prompt ${result.index} failed: ${result.error}`);
            } else {
                console.log(`Prompt ${result.index}: ${result.response.substring(0, 100)}...`);
            }
        });
    })
    .catch(error => console.error(error));
```

## Troubleshooting

### Common Issues

1. **High failure rates**: Reduce worker count or increase delays
2. **Timeouts**: Break large batches into smaller chunks
3. **Rate limit errors**: Increase `delay_between_ms` parameter
4. **Memory errors**: Reduce batch size or worker count

### Debugging

1. Check individual error messages in results
2. Monitor Lambda logs for worker-level errors
3. Test problematic prompts individually
4. Verify tool/function configurations

### Performance Tuning

1. Start with small batches to establish baseline performance
2. Gradually increase worker count while monitoring error rates
3. Adjust delays based on your data provider's rate limits
4. Consider caching for repeated similar requests