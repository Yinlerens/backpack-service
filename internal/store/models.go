package store

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type PullCompletedEvent struct {
	EventID      uuid.UUID
	EventType    string
	UserID       uuid.UUID
	BannerID     string
	Seed         string
	StateVersion int64
	PreviousPity json.RawMessage
	NextPity     json.RawMessage
	RawEvent     json.RawMessage
	Records      []PullRecord
}

type PullRecord struct {
	ID         uuid.UUID
	EventID    uuid.UUID
	UserID     uuid.UUID
	Index      int
	ItemID     string
	ItemName   string
	ItemType   string
	Rarity     int
	BannerID   string
	BannerName string
	PityAtFive int
	PityAtFour int
	IsFeatured bool
	ReceivedAt time.Time
}

type PullEvent struct {
	EventID      uuid.UUID
	UserID       uuid.UUID
	EventType    string
	BannerID     string
	Seed         string
	StateVersion int64
	PreviousPity json.RawMessage
	NextPity     json.RawMessage
	RawEvent     json.RawMessage
	ReceivedAt   time.Time
}

type InventoryItem struct {
	UserID          uuid.UUID
	ItemID          string
	ItemName        string
	ItemType        string
	Rarity          int
	Quantity        int64
	FirstReceivedAt time.Time
	UpdatedAt       time.Time
}

type InventoryCursor struct {
	UpdatedAt time.Time
	ItemID    string
}

type PullEventCursor struct {
	ReceivedAt time.Time
	EventID    uuid.UUID
}

type PullRecordCursor struct {
	ReceivedAt time.Time
	ID         uuid.UUID
}

type ListInventoryOptions struct {
	Cursor *InventoryCursor
	Limit  int
}

type ListPullEventsOptions struct {
	Cursor   *PullEventCursor
	Limit    int
	BannerID string
}

type ListPullRecordsOptions struct {
	Cursor   *PullRecordCursor
	Limit    int
	BannerID string
	Rarity   *int
	ItemType string
}
