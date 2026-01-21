// Package testutil provides testing utilities for SQS integration tests
package testutil

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/localstack"
)

// LocalStackContainer wraps a LocalStack container for testing
type LocalStackContainer struct {
	Container *localstack.LocalStackContainer
	Endpoint  string
	SQSClient *sqs.Client
	QueueURL  string
}

// StartLocalStack starts a LocalStack container with SQS service
func StartLocalStack(ctx context.Context, t *testing.T) (*LocalStackContainer, error) {
	t.Helper()

	container, err := localstack.Run(ctx,
		"localstack/localstack:3.0",
		testcontainers.WithEnv(map[string]string{
			"SERVICES": "sqs",
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start localstack: %w", err)
	}

	// Get the endpoint
	endpoint, err := container.Endpoint(ctx, "")
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get endpoint: %w", err)
	}

	// Create SQS client with custom endpoint
	sqsClient, err := createSQSClient(ctx, "http://"+endpoint)
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to create SQS client: %w", err)
	}

	return &LocalStackContainer{
		Container: container,
		Endpoint:  "http://" + endpoint,
		SQSClient: sqsClient,
	}, nil
}

// createSQSClient creates an SQS client configured for LocalStack
func createSQSClient(ctx context.Context, endpoint string) (*sqs.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			"test", "test", "test",
		)),
	)
	if err != nil {
		return nil, err
	}

	// Create SQS client with custom endpoint
	sqsClient := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	return sqsClient, nil
}

// CreateQueue creates a standard test queue and returns the URL
func (l *LocalStackContainer) CreateQueue(ctx context.Context, name string) (string, error) {
	result, err := l.SQSClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(name),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create queue: %w", err)
	}
	l.QueueURL = *result.QueueUrl
	return l.QueueURL, nil
}

// CreateFIFOQueue creates a FIFO test queue with content-based deduplication
func (l *LocalStackContainer) CreateFIFOQueue(ctx context.Context, name string) (string, error) {
	result, err := l.SQSClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(name + ".fifo"),
		Attributes: map[string]string{
			"FifoQueue":                 "true",
			"ContentBasedDeduplication": "true",
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create FIFO queue: %w", err)
	}
	l.QueueURL = *result.QueueUrl
	return l.QueueURL, nil
}

// CreateFIFOQueueWithDeduplication creates a FIFO queue with explicit deduplication ID support
func (l *LocalStackContainer) CreateFIFOQueueWithDeduplication(ctx context.Context, name string) (string, error) {
	result, err := l.SQSClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(name + ".fifo"),
		Attributes: map[string]string{
			"FifoQueue":                 "true",
			"ContentBasedDeduplication": "false",
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create FIFO queue with deduplication: %w", err)
	}
	l.QueueURL = *result.QueueUrl
	return l.QueueURL, nil
}

// PurgeQueue purges all messages from the queue
func (l *LocalStackContainer) PurgeQueue(ctx context.Context) error {
	if l.QueueURL == "" {
		return fmt.Errorf("no queue URL set")
	}
	_, err := l.SQSClient.PurgeQueue(ctx, &sqs.PurgeQueueInput{
		QueueUrl: aws.String(l.QueueURL),
	})
	return err
}

// GetQueueAttributes returns queue attributes
func (l *LocalStackContainer) GetQueueAttributes(ctx context.Context) (map[string]string, error) {
	if l.QueueURL == "" {
		return nil, fmt.Errorf("no queue URL set")
	}
	result, err := l.SQSClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(l.QueueURL),
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameAll},
	})
	if err != nil {
		return nil, err
	}
	return result.Attributes, nil
}

// Terminate stops and removes the container
func (l *LocalStackContainer) Terminate(ctx context.Context) error {
	if l.Container != nil {
		return l.Container.Terminate(ctx)
	}
	return nil
}
