package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yinlerens/backpack-service/internal/store"
)

type fakeStore struct {
	inventory []store.InventoryItem
	events    []store.PullEvent
	records   []store.PullRecord
}

func (f *fakeStore) Ping(ctx context.Context) error {
	return nil
}

func (f *fakeStore) ListInventoryItems(ctx context.Context, userID uuid.UUID, opts store.ListInventoryOptions) ([]store.InventoryItem, error) {
	return f.inventory, nil
}

func (f *fakeStore) GetInventoryItem(ctx context.Context, userID uuid.UUID, itemID string) (store.InventoryItem, error) {
	for _, item := range f.inventory {
		if item.ItemID == itemID {
			return item, nil
		}
	}
	return store.InventoryItem{}, store.ErrNotFound
}

func (f *fakeStore) ListPullEvents(ctx context.Context, userID uuid.UUID, opts store.ListPullEventsOptions) ([]store.PullEvent, error) {
	return f.events, nil
}

func (f *fakeStore) GetPullEvent(ctx context.Context, userID uuid.UUID, eventID uuid.UUID) (store.PullEvent, []store.PullRecord, error) {
	for _, event := range f.events {
		if event.EventID == eventID {
			return event, f.records, nil
		}
	}
	return store.PullEvent{}, nil, store.ErrNotFound
}

func (f *fakeStore) ListPullRecords(ctx context.Context, userID uuid.UUID, opts store.ListPullRecordsOptions) ([]store.PullRecord, error) {
	return f.records, nil
}

func TestInventoryRequiresGatewayAuth(t *testing.T) {
	api := New(&fakeStore{}, Options{InternalToken: "secret"})
	request := httptest.NewRequest(http.MethodGet, "/v1/me/inventory", nil)
	response := httptest.NewRecorder()

	api.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestInventoryRejectsInvalidUserID(t *testing.T) {
	api := New(&fakeStore{}, Options{InternalToken: "secret"})
	request := httptest.NewRequest(http.MethodGet, "/v1/me/inventory", nil)
	request.Header.Set(internalTokenHeader, "secret")
	request.Header.Set(userIDHeader, "not-a-uuid")
	response := httptest.NewRecorder()

	api.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
}

func TestListInventoryReturnsItemsAndCursor(t *testing.T) {
	userID := uuid.New()
	now := time.Unix(1710000000, 0).UTC()
	api := New(&fakeStore{inventory: []store.InventoryItem{
		{
			UserID:          userID,
			ItemID:          "char-luoxian",
			ItemName:        "洛弦",
			ItemType:        "character",
			Rarity:          5,
			Quantity:        2,
			FirstReceivedAt: now,
			UpdatedAt:       now,
		},
		{
			UserID:          userID,
			ItemID:          "weapon-tide",
			ItemName:        "潮汐笔记",
			ItemType:        "weapon",
			Rarity:          3,
			Quantity:        1,
			FirstReceivedAt: now,
			UpdatedAt:       now.Add(-time.Second),
		},
	}}, Options{InternalToken: "secret", MaxPageLimit: 100})

	request := httptest.NewRequest(http.MethodGet, "/v1/me/inventory?limit=1", nil)
	request.Header.Set(internalTokenHeader, "secret")
	request.Header.Set(userIDHeader, userID.String())
	response := httptest.NewRecorder()

	api.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}

	var body inventoryListResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(body.Items))
	}
	if body.NextCursor == "" {
		t.Fatal("expected next cursor")
	}
}

func TestGetPullEventReturnsRecords(t *testing.T) {
	userID := uuid.New()
	eventID := uuid.New()
	recordID := uuid.New()
	now := time.Unix(1710000000, 0).UTC()
	api := New(&fakeStore{
		events: []store.PullEvent{
			{
				EventID:      eventID,
				UserID:       userID,
				EventType:    "gacha.pull_completed.v1",
				BannerID:     "limited-character-001",
				Seed:         "seed",
				StateVersion: 1,
				PreviousPity: json.RawMessage(`{"version":0}`),
				NextPity:     json.RawMessage(`{"version":1}`),
				ReceivedAt:   now,
			},
		},
		records: []store.PullRecord{
			{
				ID:         recordID,
				EventID:    eventID,
				UserID:     userID,
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
		},
	}, Options{InternalToken: "secret"})

	request := httptest.NewRequest(http.MethodGet, "/v1/me/pull-events/"+eventID.String(), nil)
	request.Header.Set(internalTokenHeader, "secret")
	request.Header.Set(userIDHeader, userID.String())
	response := httptest.NewRecorder()

	api.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}

	var body pullEventDetailResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Event.EventID != eventID.String() {
		t.Fatalf("expected event %s, got %s", eventID, body.Event.EventID)
	}
	if len(body.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(body.Records))
	}
}

func TestListPullRecordsRejectsInvalidFilters(t *testing.T) {
	userID := uuid.New()
	api := New(&fakeStore{}, Options{InternalToken: "secret"})
	request := httptest.NewRequest(http.MethodGet, "/v1/me/pull-records?rarity=6", nil)
	request.Header.Set(internalTokenHeader, "secret")
	request.Header.Set(userIDHeader, userID.String())
	response := httptest.NewRecorder()

	api.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
}

func TestCursorRoundTrips(t *testing.T) {
	now := time.Unix(1710000000, 123).UTC()
	item := store.InventoryItem{ItemID: "char-luoxian", UpdatedAt: now}
	parsedInventory, err := parseInventoryCursor(encodeInventoryCursor(item))
	if err != nil {
		t.Fatalf("parse inventory cursor: %v", err)
	}
	if parsedInventory.ItemID != item.ItemID || !parsedInventory.UpdatedAt.Equal(now) {
		t.Fatal("inventory cursor did not round trip")
	}

	eventID := uuid.New()
	event := store.PullEvent{EventID: eventID, ReceivedAt: now}
	parsedEvent, err := parsePullEventCursor(encodePullEventCursor(event))
	if err != nil {
		t.Fatalf("parse pull event cursor: %v", err)
	}
	if parsedEvent.EventID != eventID || !parsedEvent.ReceivedAt.Equal(now) {
		t.Fatal("pull event cursor did not round trip")
	}
}
