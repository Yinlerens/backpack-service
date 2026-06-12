package consumer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/yinlerens/backpack-service/internal/store"
)

const PullCompletedEventType = "gacha.pull_completed.v1"

type eventEnvelope struct {
	EventID      string          `json:"event_id"`
	EventType    string          `json:"event_type"`
	UserID       string          `json:"user_id"`
	BannerID     string          `json:"banner_id"`
	Seed         string          `json:"seed"`
	Records      []recordPayload `json:"records"`
	PreviousPity json.RawMessage `json:"previous_pity"`
	NextPity     json.RawMessage `json:"next_pity"`
	StateVersion int64           `json:"state_version"`
}

type recordPayload struct {
	ID         string `json:"id"`
	Index      int    `json:"index"`
	ItemID     string `json:"item_id"`
	ItemName   string `json:"item_name"`
	ItemType   string `json:"item_type"`
	Rarity     int    `json:"rarity"`
	BannerID   string `json:"banner_id"`
	BannerName string `json:"banner_name"`
	PityAtFive int    `json:"pity_at_five"`
	PityAtFour int    `json:"pity_at_four"`
	IsFeatured bool   `json:"is_featured"`
}

type EventStore interface {
	ApplyPullCompletedEvent(ctx context.Context, event store.PullCompletedEvent) (bool, error)
}

func DecodePullCompletedEvent(value []byte) (store.PullCompletedEvent, error) {
	rawEvent, err := normalizeJSONObject(value)
	if err != nil {
		return store.PullCompletedEvent{}, fmt.Errorf("raw event must be a JSON object: %w", err)
	}

	var payload eventEnvelope
	decoder := json.NewDecoder(bytes.NewReader(value))
	if err := decoder.Decode(&payload); err != nil {
		return store.PullCompletedEvent{}, fmt.Errorf("decode event: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return store.PullCompletedEvent{}, fmt.Errorf("event must contain a single JSON object")
	}

	eventID, err := uuid.Parse(strings.TrimSpace(payload.EventID))
	if err != nil {
		return store.PullCompletedEvent{}, fmt.Errorf("event_id must be a UUID")
	}
	userID, err := uuid.Parse(strings.TrimSpace(payload.UserID))
	if err != nil {
		return store.PullCompletedEvent{}, fmt.Errorf("user_id must be a UUID")
	}
	if payload.EventType != PullCompletedEventType {
		return store.PullCompletedEvent{}, fmt.Errorf("event_type must be %s", PullCompletedEventType)
	}
	if err := validateText("banner_id", payload.BannerID, 100); err != nil {
		return store.PullCompletedEvent{}, err
	}
	if err := validateText("seed", payload.Seed, 200); err != nil {
		return store.PullCompletedEvent{}, err
	}
	if payload.StateVersion < 0 {
		return store.PullCompletedEvent{}, fmt.Errorf("state_version must be non-negative")
	}

	previousPity, err := normalizeJSONObject(payload.PreviousPity)
	if err != nil {
		return store.PullCompletedEvent{}, fmt.Errorf("previous_pity must be a JSON object: %w", err)
	}
	nextPity, err := normalizeJSONObject(payload.NextPity)
	if err != nil {
		return store.PullCompletedEvent{}, fmt.Errorf("next_pity must be a JSON object: %w", err)
	}

	records, err := decodeRecords(payload.Records)
	if err != nil {
		return store.PullCompletedEvent{}, err
	}

	return store.PullCompletedEvent{
		EventID:      eventID,
		EventType:    payload.EventType,
		UserID:       userID,
		BannerID:     payload.BannerID,
		Seed:         payload.Seed,
		StateVersion: payload.StateVersion,
		PreviousPity: previousPity,
		NextPity:     nextPity,
		RawEvent:     rawEvent,
		Records:      records,
	}, nil
}

func decodeRecords(payloads []recordPayload) ([]store.PullRecord, error) {
	if len(payloads) == 0 {
		return nil, fmt.Errorf("records must contain at least one record")
	}

	records := make([]store.PullRecord, 0, len(payloads))
	for _, payload := range payloads {
		id, err := uuid.Parse(strings.TrimSpace(payload.ID))
		if err != nil {
			return nil, fmt.Errorf("record id must be a UUID")
		}
		if payload.Index < 0 {
			return nil, fmt.Errorf("record index must be non-negative")
		}
		if err := validateText("item_id", payload.ItemID, 100); err != nil {
			return nil, err
		}
		if err := validateText("item_name", payload.ItemName, 200); err != nil {
			return nil, err
		}
		if payload.ItemType != "character" && payload.ItemType != "weapon" {
			return nil, fmt.Errorf("item_type must be character or weapon")
		}
		if payload.Rarity != 3 && payload.Rarity != 4 && payload.Rarity != 5 {
			return nil, fmt.Errorf("rarity must be 3, 4, or 5")
		}
		if err := validateText("banner_id", payload.BannerID, 100); err != nil {
			return nil, err
		}
		if err := validateText("banner_name", payload.BannerName, 200); err != nil {
			return nil, err
		}
		if payload.PityAtFive < 1 || payload.PityAtFour < 1 {
			return nil, fmt.Errorf("pity counters must be positive")
		}

		records = append(records, store.PullRecord{
			ID:         id,
			Index:      payload.Index,
			ItemID:     payload.ItemID,
			ItemName:   payload.ItemName,
			ItemType:   payload.ItemType,
			Rarity:     payload.Rarity,
			BannerID:   payload.BannerID,
			BannerName: payload.BannerName,
			PityAtFive: payload.PityAtFive,
			PityAtFour: payload.PityAtFour,
			IsFeatured: payload.IsFeatured,
		})
	}
	return records, nil
}

func normalizeJSONObject(raw []byte) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("value is empty")
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("value must contain a single JSON object")
	}

	if _, ok := value.(map[string]any); !ok {
		return nil, fmt.Errorf("value is not an object")
	}

	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func validateText(field string, value string, maxLength int) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if len(value) > maxLength {
		return fmt.Errorf("%s must be %d characters or fewer", field, maxLength)
	}
	return nil
}
