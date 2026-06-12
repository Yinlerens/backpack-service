package consumer

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"github.com/yinlerens/backpack-service/internal/store"
)

type fakeReader struct {
	message   kafka.Message
	committed bool
}

func (f *fakeReader) FetchMessage(ctx context.Context) (kafka.Message, error) {
	return f.message, nil
}

func (f *fakeReader) CommitMessages(ctx context.Context, msgs ...kafka.Message) error {
	f.committed = true
	return nil
}

func (f *fakeReader) Close() error {
	return nil
}

type fakeStore struct {
	applied bool
	err     error
	event   store.PullCompletedEvent
}

func (f *fakeStore) ApplyPullCompletedEvent(ctx context.Context, event store.PullCompletedEvent) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	f.applied = true
	f.event = event
	return true, nil
}

func TestDecodePullCompletedEvent(t *testing.T) {
	event, err := DecodePullCompletedEvent(sampleEventJSON())
	if err != nil {
		t.Fatalf("decode event: %v", err)
	}

	if event.EventType != PullCompletedEventType {
		t.Fatalf("expected event type %s, got %s", PullCompletedEventType, event.EventType)
	}
	if event.UserID.String() != "ae6b9d2e-9bb0-42c7-950f-c38ab6d7195e" {
		t.Fatalf("unexpected user id %s", event.UserID)
	}
	if len(event.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(event.Records))
	}
}

func TestStepCommitsAfterStoreSuccess(t *testing.T) {
	reader := &fakeReader{message: kafka.Message{Value: sampleEventJSON()}}
	store := &fakeStore{}
	consumer := New(reader, store, slog.Default())

	if err := consumer.Step(context.Background()); err != nil {
		t.Fatalf("step: %v", err)
	}

	if !store.applied {
		t.Fatal("expected store to apply event")
	}
	if !reader.committed {
		t.Fatal("expected message to be committed")
	}
}

func TestStepDoesNotCommitWhenStoreFails(t *testing.T) {
	reader := &fakeReader{message: kafka.Message{Value: sampleEventJSON()}}
	store := &fakeStore{err: errors.New("database unavailable")}
	consumer := New(reader, store, slog.Default())

	if err := consumer.Step(context.Background()); err == nil {
		t.Fatal("expected step to fail")
	}

	if reader.committed {
		t.Fatal("did not expect message to be committed")
	}
}

func TestDecodeRejectsUnknownEventType(t *testing.T) {
	value := []byte(`{
		"event_id":"` + uuid.NewString() + `",
		"event_type":"other",
		"user_id":"ae6b9d2e-9bb0-42c7-950f-c38ab6d7195e",
		"banner_id":"limited-character-001",
		"seed":"seed",
		"records":[],
		"previous_pity":{},
		"next_pity":{},
		"state_version":1
	}`)

	if _, err := DecodePullCompletedEvent(value); err == nil {
		t.Fatal("expected decode to reject unknown event type")
	}
}

func sampleEventJSON() []byte {
	return []byte(`{
		"event_id":"f7db8d82-41d2-4b43-9678-22ed0d07ffba",
		"event_type":"gacha.pull_completed.v1",
		"user_id":"ae6b9d2e-9bb0-42c7-950f-c38ab6d7195e",
		"banner_id":"limited-character-001",
		"seed":"stable-seed",
		"records":[
			{
				"id":"37f1f86c-2d5b-4f69-b4cd-33e090065f95",
				"index":0,
				"item_id":"char-luoxian",
				"item_name":"洛弦",
				"item_type":"character",
				"rarity":5,
				"banner_id":"limited-character-001",
				"banner_name":"归潮观测",
				"pity_at_five":80,
				"pity_at_four":1,
				"is_featured":true
			},
			{
				"id":"b561b1c3-4413-4ae8-a9e4-6f7204e4533f",
				"index":1,
				"item_id":"weapon-tide",
				"item_name":"潮汐笔记",
				"item_type":"weapon",
				"rarity":3,
				"banner_id":"limited-character-001",
				"banner_name":"归潮观测",
				"pity_at_five":1,
				"pity_at_four":1,
				"is_featured":false
			}
		],
		"previous_pity":{"since_five":79,"since_four":0,"guaranteed_featured_five":true,"version":0},
		"next_pity":{"since_five":1,"since_four":1,"guaranteed_featured_five":false,"version":1},
		"state_version":1
	}`)
}
