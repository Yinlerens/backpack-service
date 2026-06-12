package consumer

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"
)

type Reader interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
	CommitMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

type ReaderConfig struct {
	Brokers  []string
	Topic    string
	GroupID  string
	MinBytes int
	MaxBytes int
}

type Consumer struct {
	reader Reader
	store  EventStore
	logger *slog.Logger
}

func New(reader Reader, store EventStore, logger *slog.Logger) *Consumer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Consumer{
		reader: reader,
		store:  store,
		logger: logger,
	}
}

func NewKafkaReader(cfg ReaderConfig) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:  cfg.Brokers,
		Topic:    cfg.Topic,
		GroupID:  cfg.GroupID,
		MinBytes: cfg.MinBytes,
		MaxBytes: cfg.MaxBytes,
	})
}

func (c *Consumer) Step(ctx context.Context) error {
	message, err := c.reader.FetchMessage(ctx)
	if err != nil {
		return err
	}

	event, err := DecodePullCompletedEvent(message.Value)
	if err != nil {
		return err
	}

	if _, err := c.store.ApplyPullCompletedEvent(ctx, event); err != nil {
		return err
	}

	if err := c.reader.CommitMessages(ctx, message); err != nil {
		return err
	}

	return nil
}

func (c *Consumer) Run(ctx context.Context) error {
	for {
		if err := c.Step(ctx); err != nil {
			if ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return nil
			}
			c.logger.Error("failed to consume gacha event", "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
		}
	}
}
