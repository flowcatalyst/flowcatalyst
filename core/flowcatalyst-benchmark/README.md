# FlowCatalyst Message Router Benchmark Tool

A Quarkus Picocli CLI tool for benchmarking the FlowCatalyst Message Router's throughput and performance.

## Features

- **Configurable message generation**: Specify total messages, message groups, pool codes, and target endpoints
- **SQS batch publishing**: Efficiently publishes messages in batches (up to 10 messages per batch)
- **Rate limiting**: Optional rate limiting to simulate realistic load patterns
- **FIFO message groups**: Tests message router's FIFO ordering per message group
- **Dry run mode**: Preview sample messages without actually sending them
- **Real-time metrics**: Reports throughput and success/failure rates

## Prerequisites

- Java 21+
- AWS SQS queue (FIFO queue recommended for message group testing)
- Running FlowCatalyst Message Router instance
- AWS credentials configured (via environment variables, AWS CLI, or IAM role)

## Building

```bash
./gradlew :core:flowcatalyst-benchmark:quarkusBuild
```

The executable JAR will be created at:
```
core/flowcatalyst-benchmark/build/quarkus-app/quarkus-run.jar
```

## Usage

### Basic Usage

```bash
java -jar core/flowcatalyst-benchmark/build/quarkus-app/quarkus-run.jar \
  --queue-url https://sqs.eu-west-1.amazonaws.com/123456789/my-queue.fifo \
  --target-endpoint http://localhost:8080/api/benchmark/process \
  --num-messages 1000 \
  --num-groups 10
```

### All Options

```
Usage: benchmark [-hV] -q=<queueUrl> -t=<targetEndpoint> [-a=<authToken>]
                 [-b=<batchSize>] [--dry-run] [-g=<numGroups>] [-n=<numMessages>]
                 [-p=<poolCode>] [-r=<rateLimit>]

FlowCatalyst Message Router Benchmark Tool

Options:
  -q, --queue-url=<queueUrl>
                           SQS Queue URL
  -n, --num-messages=<numMessages>
                           Total number of messages to send (default: 1000)
  -g, --num-groups=<numGroups>
                           Number of unique message groups (default: 10)
  -p, --pool-code=<poolCode>
                           Pool code for message routing (default: BENCHMARK-POOL)
  -t, --target-endpoint=<targetEndpoint>
                           Target endpoint URL (e.g., http://localhost:8080/api/benchmark/process)
  -a, --auth-token=<authToken>
                           Optional auth token for the target endpoint
  -b, --batch-size=<batchSize>
                           Batch size for SQS SendMessageBatch (1-10, default: 10)
  -r, --rate-limit=<rateLimit>
                           Optional rate limit (messages per second).
                           If not set, sends as fast as possible
      --dry-run            Print sample messages without sending (default: false)
  -h, --help               Show this help message and exit.
  -V, --version            Print version information and exit.
```

## Examples

### Example 1: High-throughput test (1000 messages, unlimited rate)

```bash
java -jar benchmark.jar \
  -q https://sqs.eu-west-1.amazonaws.com/123456789/router-queue.fifo \
  -t http://localhost:8080/api/benchmark/process \
  -n 1000 \
  -g 50 \
  -p BENCHMARK-POOL
```

### Example 2: Rate-limited test (simulate 100 msg/s load)

```bash
java -jar benchmark.jar \
  -q https://sqs.eu-west-1.amazonaws.com/123456789/router-queue.fifo \
  -t http://localhost:8080/api/benchmark/process \
  -n 10000 \
  -g 100 \
  -r 100 \
  -p PROD-POOL
```

### Example 3: Dry run (preview messages without sending)

```bash
java -jar benchmark.jar \
  -q https://sqs.eu-west-1.amazonaws.com/123456789/router-queue.fifo \
  -t http://localhost:8080/api/benchmark/process \
  -n 100 \
  -g 10 \
  --dry-run
```

### Example 4: Small batch sizes (test single-message performance)

```bash
java -jar benchmark.jar \
  -q https://sqs.eu-west-1.amazonaws.com/123456789/router-queue.fifo \
  -t http://localhost:8080/api/benchmark/process \
  -n 1000 \
  -g 10 \
  -b 1
```

## Mock Endpoint in Message Router

The message router includes mock endpoints for benchmarking:

### Immediate Response (fast path)
```
POST /api/benchmark/process
```
Returns 200 OK immediately without any processing.

### Simulated Processing Delay
```
POST /api/benchmark/process-slow?delayMs=100
```
Sleeps for specified milliseconds before returning.

### View Benchmark Stats
```
GET /api/benchmark/stats
```
Returns current throughput metrics:
```json
{
  "totalRequests": 1000,
  "elapsedMs": 5243,
  "throughputPerSecond": 190.73
}
```

### Reset Stats
```
POST /api/benchmark/reset
```
Resets benchmark counters to zero.

## Message Format

Each message sent to SQS follows the MessagePointer format:

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "poolCode": "BENCHMARK-POOL",
  "authToken": null,
  "mediationType": "HTTP",
  "mediationTarget": "http://localhost:8080/api/benchmark/process",
  "messageGroupId": "group-5"
}
```

## Interpreting Results

```
=== Benchmark Complete ===
Total Messages Sent: 1000
Failed: 0
Duration: 2.45 seconds
Throughput: 408.16 msg/s
==============================
```

**Key Metrics:**
- **Total Messages Sent**: Successfully published to SQS
- **Failed**: Messages that failed to publish
- **Duration**: Time taken to publish all messages
- **Throughput**: Messages per second published to SQS

**Note**: This measures SQS publishing throughput, not end-to-end message processing. Monitor the message router's logs and `/api/benchmark/stats` endpoint for actual processing throughput.

## Testing Scenarios

### 1. Baseline Throughput
Test maximum throughput without rate limiting:
```bash
java -jar benchmark.jar -q <queue-url> -t <endpoint> -n 10000 -g 100
```

### 2. FIFO Ordering
Test FIFO ordering with few message groups:
```bash
java -jar benchmark.jar -q <queue-url> -t <endpoint> -n 1000 -g 5
```
Each group should process sequentially, different groups process concurrently.

### 3. High Concurrency
Test many concurrent message groups:
```bash
java -jar benchmark.jar -q <queue-url> -t <endpoint> -n 10000 -g 500
```

### 4. Sustained Load
Test sustained load over time:
```bash
java -jar benchmark.jar -q <queue-url> -t <endpoint> -n 100000 -g 100 -r 500
```

## Configuration

### AWS Credentials

The tool uses the default AWS credential provider chain:
1. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`)
2. AWS credentials file (`~/.aws/credentials`)
3. IAM role (if running on EC2/ECS/Fargate)

### SQS Endpoint Override (LocalStack)

For local development with LocalStack:
```bash
export SQS_ENDPOINT_OVERRIDE=http://localhost:4566
java -jar benchmark.jar -q http://localhost:4566/000000000000/test-queue.fifo -t http://localhost:8080/api/benchmark/process -n 100
```

## Troubleshooting

**Error: "Could not find queue"**
- Verify the queue URL is correct
- Ensure AWS credentials have SQS permissions
- Check the queue exists in the correct region

**Error: "Rate exceeded"**
- Reduce batch size or rate limit
- Check SQS quota limits for your account

**Low throughput**
- Increase batch size (up to 10)
- Remove rate limiting
- Check network latency to SQS

## Architecture

```
Benchmark CLI
    |
    v
SQS Queue (FIFO)
    |
    v
Message Router
    |
    v
Mock Endpoint (/api/benchmark/process)
```

The benchmark tool focuses on SQS publishing performance. Monitor the message router's processing performance separately using its built-in metrics.
