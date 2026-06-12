package httpapi

import (
	"encoding/json"
	"time"

	"github.com/yinlerens/backpack-service/internal/store"
)

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorResponse struct {
	Error apiError `json:"error"`
}

type inventoryItemResponse struct {
	UserID          string    `json:"user_id"`
	ItemID          string    `json:"item_id"`
	ItemName        string    `json:"item_name"`
	ItemType        string    `json:"item_type"`
	Rarity          int       `json:"rarity"`
	Quantity        int64     `json:"quantity"`
	FirstReceivedAt time.Time `json:"first_received_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type inventoryListResponse struct {
	Items      []inventoryItemResponse `json:"items"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

type pullEventResponse struct {
	EventID      string          `json:"event_id"`
	UserID       string          `json:"user_id"`
	EventType    string          `json:"event_type"`
	BannerID     string          `json:"banner_id"`
	Seed         string          `json:"seed"`
	StateVersion int64           `json:"state_version"`
	PreviousPity json.RawMessage `json:"previous_pity"`
	NextPity     json.RawMessage `json:"next_pity"`
	ReceivedAt   time.Time       `json:"received_at"`
}

type pullEventListResponse struct {
	Items      []pullEventResponse `json:"items"`
	NextCursor string              `json:"next_cursor,omitempty"`
}

type pullEventDetailResponse struct {
	Event   pullEventResponse    `json:"event"`
	Records []pullRecordResponse `json:"records"`
}

type pullRecordResponse struct {
	ID         string    `json:"id"`
	EventID    string    `json:"event_id"`
	UserID     string    `json:"user_id"`
	Index      int       `json:"index"`
	ItemID     string    `json:"item_id"`
	ItemName   string    `json:"item_name"`
	ItemType   string    `json:"item_type"`
	Rarity     int       `json:"rarity"`
	BannerID   string    `json:"banner_id"`
	BannerName string    `json:"banner_name"`
	PityAtFive int       `json:"pity_at_five"`
	PityAtFour int       `json:"pity_at_four"`
	IsFeatured bool      `json:"is_featured"`
	ReceivedAt time.Time `json:"received_at"`
}

type pullRecordListResponse struct {
	Items      []pullRecordResponse `json:"items"`
	NextCursor string               `json:"next_cursor,omitempty"`
}

func inventoryItemResponseFromStore(item store.InventoryItem) inventoryItemResponse {
	return inventoryItemResponse{
		UserID:          item.UserID.String(),
		ItemID:          item.ItemID,
		ItemName:        item.ItemName,
		ItemType:        item.ItemType,
		Rarity:          item.Rarity,
		Quantity:        item.Quantity,
		FirstReceivedAt: item.FirstReceivedAt,
		UpdatedAt:       item.UpdatedAt,
	}
}

func pullEventResponseFromStore(event store.PullEvent) pullEventResponse {
	return pullEventResponse{
		EventID:      event.EventID.String(),
		UserID:       event.UserID.String(),
		EventType:    event.EventType,
		BannerID:     event.BannerID,
		Seed:         event.Seed,
		StateVersion: event.StateVersion,
		PreviousPity: normalizeRawJSON(event.PreviousPity),
		NextPity:     normalizeRawJSON(event.NextPity),
		ReceivedAt:   event.ReceivedAt,
	}
}

func pullRecordResponseFromStore(record store.PullRecord) pullRecordResponse {
	return pullRecordResponse{
		ID:         record.ID.String(),
		EventID:    record.EventID.String(),
		UserID:     record.UserID.String(),
		Index:      record.Index,
		ItemID:     record.ItemID,
		ItemName:   record.ItemName,
		ItemType:   record.ItemType,
		Rarity:     record.Rarity,
		BannerID:   record.BannerID,
		BannerName: record.BannerName,
		PityAtFive: record.PityAtFive,
		PityAtFour: record.PityAtFour,
		IsFeatured: record.IsFeatured,
		ReceivedAt: record.ReceivedAt,
	}
}

func normalizeRawJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	return value
}
