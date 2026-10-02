// Package events publishes domain events to SQS.
package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"shop-aws/services/orders-api/internal/orders"
)

type SQSPublisher struct {
	client   *sqs.Client
	queueURL string
}

func NewSQSPublisher(client *sqs.Client, queueURL string) *SQSPublisher {
	return &SQSPublisher{client: client, queueURL: queueURL}
}

func (p *SQSPublisher) Publish(ctx context.Context, evt orders.OrderCreatedEvent) error {
	body, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	attrs := map[string]types.MessageAttributeValue{
		"eventType": {DataType: aws.String("String"), StringValue: aws.String(evt.EventType)},
	}
	if evt.RequestID != "" {
		attrs["requestId"] = types.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(evt.RequestID)}
	}

	_, err = p.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:          aws.String(p.queueURL),
		MessageBody:       aws.String(string(body)),
		MessageAttributes: attrs,
	})
	if err != nil {
		return fmt.Errorf("sqs send message: %w", err)
	}
	return nil
}

// UnconfiguredPublisher is used when QUEUE_URL is empty: every publish fails,
// so orders are kept with eventPublished=false and an ERROR is logged.
type UnconfiguredPublisher struct{}

func (UnconfiguredPublisher) Publish(context.Context, orders.OrderCreatedEvent) error {
	return errors.New("QUEUE_URL is not configured")
}
