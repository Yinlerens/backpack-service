package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultPort          = "8080"
	defaultMaxPageLimit  = 100
	defaultKafkaTopic    = "gacha.pull_completed.v1"
	defaultKafkaGroupID  = "backpack-service"
	defaultKafkaMinBytes = 1
	defaultKafkaMaxBytes = 10 << 20
)

type Config struct {
	Addr            string
	DatabaseURL     string
	InternalToken   string
	MaxPageLimit    int
	KafkaBrokers    []string
	KafkaTopic      string
	KafkaGroupID    string
	KafkaMinBytes   int
	KafkaMaxBytes   int
	ConsumerEnabled bool
}

func Load() (Config, error) {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = defaultPort
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	internalToken := strings.TrimSpace(os.Getenv("INTERNAL_TOKEN"))
	if internalToken == "" {
		return Config{}, errors.New("INTERNAL_TOKEN is required")
	}

	consumerEnabled := boolEnv("CONSUMER_ENABLED", true)
	kafkaBrokers := splitCSV(os.Getenv("KAFKA_BROKERS"))
	if consumerEnabled && len(kafkaBrokers) == 0 {
		return Config{}, errors.New("KAFKA_BROKERS is required when consumer is enabled")
	}

	kafkaTopic := strings.TrimSpace(os.Getenv("KAFKA_TOPIC"))
	if kafkaTopic == "" {
		kafkaTopic = defaultKafkaTopic
	}

	kafkaGroupID := strings.TrimSpace(os.Getenv("KAFKA_GROUP_ID"))
	if kafkaGroupID == "" {
		kafkaGroupID = defaultKafkaGroupID
	}

	maxPageLimit, err := intEnv("MAX_PAGE_LIMIT", defaultMaxPageLimit)
	if err != nil {
		return Config{}, err
	}
	kafkaMinBytes, err := intEnv("KAFKA_MIN_BYTES", defaultKafkaMinBytes)
	if err != nil {
		return Config{}, err
	}
	kafkaMaxBytes, err := intEnv("KAFKA_MAX_BYTES", defaultKafkaMaxBytes)
	if err != nil {
		return Config{}, err
	}
	if kafkaMaxBytes < kafkaMinBytes {
		return Config{}, errors.New("KAFKA_MAX_BYTES must be greater than or equal to KAFKA_MIN_BYTES")
	}

	return Config{
		Addr:            ":" + port,
		DatabaseURL:     databaseURL,
		InternalToken:   internalToken,
		MaxPageLimit:    maxPageLimit,
		KafkaBrokers:    kafkaBrokers,
		KafkaTopic:      kafkaTopic,
		KafkaGroupID:    kafkaGroupID,
		KafkaMinBytes:   kafkaMinBytes,
		KafkaMaxBytes:   kafkaMaxBytes,
		ConsumerEnabled: consumerEnabled,
	}, nil
}

func intEnv(name string, defaultValue int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func boolEnv(name string, defaultValue bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return defaultValue
	}
	return !(value == "0" || value == "false" || value == "no" || value == "off")
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
