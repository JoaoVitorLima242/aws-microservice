// Package worker consumes OrderCreated events from SQS and "sends" the
// customer notification (for now, it only logs it).
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"shop-aws/services/notifier-worker/internal/logging"
)

const (
	receiveErrorBackoff = 2 * time.Second
	deleteTimeout       = 5 * time.Second
)

var errSimulatedFailure = errors.New("simulated failure (FAIL_RATE)")

type Config struct {
	QueueURL        string
	MaxMessages     int32
	WaitTimeSeconds int32
	FailRate        float64
}

type Worker struct {
	client *sqs.Client
	cfg    Config
	logger *slog.Logger

	// processed holds the eventIds already handled, for simple in-memory
	// idempotency. Only the polling goroutine touches it, so no lock is
	// needed. It is lost on restart and not shared between replicas.
	processed map[string]struct{}
}

func New(client *sqs.Client, cfg Config, logger *slog.Logger) *Worker {
	return &Worker{client: client, cfg: cfg, logger: logger, processed: make(map[string]struct{})}
}

// Run long-polls the queue until ctx is cancelled. Cancelling ctx interrupts
// an in-progress poll, but a batch that was already received is always
// processed to the end before Run returns.
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("worker started",
		slog.Int("maxMessages", int(w.cfg.MaxMessages)),
		slog.Int("waitTimeSeconds", int(w.cfg.WaitTimeSeconds)),
		slog.Float64("failRate", w.cfg.FailRate),
	)

	for ctx.Err() == nil {
		out, err := w.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(w.cfg.QueueURL),
			MaxNumberOfMessages: w.cfg.MaxMessages,
			WaitTimeSeconds:     w.cfg.WaitTimeSeconds,
			MessageSystemAttributeNames: []types.MessageSystemAttributeName{
				types.MessageSystemAttributeNameApproximateReceiveCount,
			},
		})
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			w.logger.Error("failed to receive messages", slog.Any("error", err))
			select {
			case <-time.After(receiveErrorBackoff):
			case <-ctx.Done():
			}
			continue
		}

		if len(out.Messages) > 0 {
			w.logger.Debug("received batch", slog.Int("messages", len(out.Messages)))
		}
		// The batch is processed with a context that is not cancelled by
		// SIGTERM, so in-flight messages are finished and deleted.
		batchCtx := context.WithoutCancel(ctx)
		for _, msg := range out.Messages {
			w.handle(batchCtx, msg)
		}
	}

	w.logger.Info("worker stopped polling")
	return nil
}

func (w *Worker) handle(ctx context.Context, msg types.Message) {
	log := w.logger.With(
		slog.String("messageId", aws.ToString(msg.MessageId)),
		slog.String("receiveCount", msg.Attributes[string(types.MessageSystemAttributeNameApproximateReceiveCount)]),
	)

	var evt OrderCreatedEvent
	if err := json.Unmarshal([]byte(aws.ToString(msg.Body)), &evt); err != nil {
		log.Error("invalid message: malformed JSON, leaving it in the queue", slog.Any("error", err))
		return
	}

	if evt.RequestID != "" {
		ctx = logging.WithRequestID(ctx, evt.RequestID)
	}
	log = log.With(slog.String("eventId", evt.EventID), slog.String("orderId", evt.Data.OrderID))

	if err := evt.validate(); err != nil {
		log.ErrorContext(ctx, "invalid message: unexpected format, leaving it in the queue", slog.Any("error", err))
		return
	}

	if _, seen := w.processed[evt.EventID]; seen {
		log.InfoContext(ctx, "duplicate event ignored")
		w.delete(ctx, msg, log)
		return
	}

	if err := w.notify(ctx, evt, log); err != nil {
		log.ErrorContext(ctx, "failed to process message, it will be retried", slog.Any("error", err))
		return
	}

	// Mark before deleting: if the delete fails, the redelivered copy is
	// recognised as a duplicate and simply deleted.
	w.processed[evt.EventID] = struct{}{}
	w.delete(ctx, msg, log)
}

func (w *Worker) notify(ctx context.Context, evt OrderCreatedEvent, log *slog.Logger) error {
	if w.cfg.FailRate > 0 && rand.Float64() < w.cfg.FailRate {
		return errSimulatedFailure
	}
	log.InfoContext(ctx, "notification sent",
		slog.String("customerEmail", evt.Data.CustomerEmail),
		slog.String("productId", evt.Data.ProductID),
		slog.Int("quantity", evt.Data.Quantity),
		slog.String("total", evt.Data.Total.String()),
	)
	return nil
}

func (w *Worker) delete(ctx context.Context, msg types.Message, log *slog.Logger) {
	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()
	_, err := w.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(w.cfg.QueueURL),
		ReceiptHandle: msg.ReceiptHandle,
	})
	if err != nil {
		log.ErrorContext(ctx, "failed to delete message, it will be redelivered", slog.Any("error", err))
		return
	}
	log.DebugContext(ctx, "message deleted")
}
