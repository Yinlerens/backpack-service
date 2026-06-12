package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestApplyPullCompletedEventIsIdempotent(t *testing.T) {
	ctx, store := openIntegrationStore(t)
	defer store.Close()

	event := samplePullCompletedEvent()
	created, err := store.ApplyPullCompletedEvent(ctx, event)
	if err != nil {
		t.Fatalf("apply event: %v", err)
	}
	if !created {
		t.Fatal("expected event to be created")
	}

	created, err = store.ApplyPullCompletedEvent(ctx, event)
	if err != nil {
		t.Fatalf("apply duplicate event: %v", err)
	}
	if created {
		t.Fatal("expected duplicate event to be ignored")
	}

	items, err := store.ListInventoryItems(ctx, event.UserID, ListInventoryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("list inventory: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 inventory items, got %d", len(items))
	}

	loaded, records, err := store.GetPullEvent(ctx, event.UserID, event.EventID)
	if err != nil {
		t.Fatalf("get pull event: %v", err)
	}
	if loaded.EventID != event.EventID {
		t.Fatalf("expected event id %s, got %s", event.EventID, loaded.EventID)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
}

func TestListPullRecordsFiltersByBannerAndRarity(t *testing.T) {
	ctx, store := openIntegrationStore(t)
	defer store.Close()

	event := samplePullCompletedEvent()
	if _, err := store.ApplyPullCompletedEvent(ctx, event); err != nil {
		t.Fatalf("seed event: %v", err)
	}

	records, err := store.ListPullRecords(ctx, event.UserID, ListPullRecordsOptions{
		Limit:    10,
		BannerID: event.BannerID,
		Rarity:   intPtr(5),
	})
	if err != nil {
		t.Fatalf("list pull records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
}

func openIntegrationStore(t *testing.T) (context.Context, *Store) {
	t.Helper()

	databaseURL := strings.TrimSpace(os.Getenv("BACKPACK_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("BACKPACK_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()

	if err := applyMigrationSQL(ctx, pool, "migrations/000001_init.up.sql", "../../migrations/000001_init.up.sql"); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		truncate table backpack.pull_records, backpack.pull_events, backpack.inventory_items restart identity cascade
	`); err != nil {
		t.Fatalf("truncate tables: %v", err)
	}

	store, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	return ctx, store
}

func applyMigrationSQL(ctx context.Context, pool *pgxpool.Pool, paths ...string) error {
	var (
		content []byte
		err     error
	)
	for _, path := range paths {
		content, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}

	statements := strings.Split(string(content), ";")
	for _, statement := range statements {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := pool.Exec(ctx, statement); err != nil {
			return fmt.Errorf("exec migration statement: %w", err)
		}
	}
	return nil
}

func samplePullCompletedEvent() PullCompletedEvent {
	userID := uuid.MustParse("ae6b9d2e-9bb0-42c7-950f-c38ab6d7195e")
	eventID := uuid.MustParse("f7db8d82-41d2-4b43-9678-22ed0d07ffba")
	now := time.Unix(1710000000, 0).UTC()
	return PullCompletedEvent{
		EventID:      eventID,
		EventType:    "gacha.pull_completed.v1",
		UserID:       userID,
		BannerID:     "limited-character-001",
		Seed:         "stable-seed",
		StateVersion: 1,
		PreviousPity: json.RawMessage(`{"since_five":79,"since_four":0,"guaranteed_featured_five":true,"version":0}`),
		NextPity:     json.RawMessage(`{"since_five":1,"since_four":1,"guaranteed_featured_five":false,"version":1}`),
		RawEvent:     json.RawMessage(`{"event_type":"gacha.pull_completed.v1"}`),
		Records: []PullRecord{
			{
				ID:         uuid.MustParse("37f1f86c-2d5b-4f69-b4cd-33e090065f95"),
				Index:      0,
				ItemID:     "char-luoxian",
				ItemName:   "洛弦",
				ItemType:   "character",
				Rarity:     5,
				BannerID:   "limited-character-001",
				BannerName: "归潮观测",
				PityAtFive: 80,
				PityAtFour: 1,
				IsFeatured: true,
				ReceivedAt: now,
			},
			{
				ID:         uuid.MustParse("b561b1c3-4413-4ae8-a9e4-6f7204e4533f"),
				Index:      1,
				ItemID:     "weapon-tide",
				ItemName:   "潮汐笔记",
				ItemType:   "weapon",
				Rarity:     3,
				BannerID:   "limited-character-001",
				BannerName: "归潮观测",
				PityAtFive: 1,
				PityAtFour: 1,
				IsFeatured: false,
				ReceivedAt: now.Add(time.Second),
			},
		},
	}
}

func intPtr(value int) *int {
	return &value
}
