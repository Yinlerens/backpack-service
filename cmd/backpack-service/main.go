package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yinlerens/backpack-service/internal/config"
	"github.com/yinlerens/backpack-service/internal/consumer"
	"github.com/yinlerens/backpack-service/internal/httpapi"
	"github.com/yinlerens/backpack-service/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("backpack service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dataStore, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer dataStore.Close()

	api := httpapi.New(dataStore, httpapi.Options{
		InternalToken: cfg.InternalToken,
		MaxPageLimit:  cfg.MaxPageLimit,
	})

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() {
		slog.Info("backpack service listening", "addr", cfg.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	var reader consumer.Reader
	if cfg.ConsumerEnabled {
		reader = consumer.NewKafkaReader(consumer.ReaderConfig{
			Brokers:  cfg.KafkaBrokers,
			Topic:    cfg.KafkaTopic,
			GroupID:  cfg.KafkaGroupID,
			MinBytes: cfg.KafkaMinBytes,
			MaxBytes: cfg.KafkaMaxBytes,
		})
		defer reader.Close()

		eventConsumer := consumer.New(reader, dataStore, slog.Default())
		go func() {
			slog.Info("backpack consumer started", "topic", cfg.KafkaTopic, "group_id", cfg.KafkaGroupID)
			errCh <- eventConsumer.Run(ctx)
		}()
	}

	select {
	case err := <-errCh:
		if err != nil {
			stop()
			_ = shutdownServer(server)
			return err
		}
		return nil
	case <-ctx.Done():
	}

	return shutdownServer(server)
}

func shutdownServer(server *http.Server) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
