package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yinlerens/backpack-service/internal/store"
)

const (
	internalTokenHeader = "X-Internal-Token"
	userIDHeader        = "X-User-Id"
)

type Store interface {
	Ping(ctx context.Context) error
	ListInventoryItems(ctx context.Context, userID uuid.UUID, opts store.ListInventoryOptions) ([]store.InventoryItem, error)
	GetInventoryItem(ctx context.Context, userID uuid.UUID, itemID string) (store.InventoryItem, error)
	ListPullEvents(ctx context.Context, userID uuid.UUID, opts store.ListPullEventsOptions) ([]store.PullEvent, error)
	GetPullEvent(ctx context.Context, userID uuid.UUID, eventID uuid.UUID) (store.PullEvent, []store.PullRecord, error)
	ListPullRecords(ctx context.Context, userID uuid.UUID, opts store.ListPullRecordsOptions) ([]store.PullRecord, error)
}

type Options struct {
	InternalToken string
	MaxPageLimit  int
}

type Server struct {
	store         Store
	internalToken string
	maxPageLimit  int
}

func New(store Store, opts Options) *Server {
	maxPageLimit := opts.MaxPageLimit
	if maxPageLimit < 1 {
		maxPageLimit = 100
	}
	return &Server{
		store:         store,
		internalToken: opts.InternalToken,
		maxPageLimit:  maxPageLimit,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /ready", s.handleReady)
	mux.Handle("GET /v1/me/inventory", s.withGatewayAuth(http.HandlerFunc(s.handleListInventory)))
	mux.Handle("GET /v1/me/inventory/{item_id}", s.withGatewayAuth(http.HandlerFunc(s.handleGetInventoryItem)))
	mux.Handle("GET /v1/me/pull-events", s.withGatewayAuth(http.HandlerFunc(s.handleListPullEvents)))
	mux.Handle("GET /v1/me/pull-events/{event_id}", s.withGatewayAuth(http.HandlerFunc(s.handleGetPullEvent)))
	mux.Handle("GET /v1/me/pull-records", s.withGatewayAuth(http.HandlerFunc(s.handleListPullRecords)))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.store.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "database is unavailable")
		return
	}

	if strings.TrimSpace(s.internalToken) == "" {
		writeError(w, http.StatusServiceUnavailable, "internal_token_missing", "internal token is not configured")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleListInventory(w http.ResponseWriter, r *http.Request) {
	userID := mustUserID(r.Context())
	limit, err := parseLimit(r, s.maxPageLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_limit", err.Error())
		return
	}
	cursor, err := parseInventoryCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "cursor is invalid")
		return
	}

	items, err := s.store.ListInventoryItems(r.Context(), userID, store.ListInventoryOptions{
		Cursor: cursor,
		Limit:  limit + 1,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "inventory_unavailable", "inventory is unavailable")
		return
	}

	var nextCursor string
	if len(items) > limit {
		items = items[:limit]
		nextCursor = encodeInventoryCursor(items[len(items)-1])
	}

	response := inventoryListResponse{
		Items:      make([]inventoryItemResponse, 0, len(items)),
		NextCursor: nextCursor,
	}
	for _, item := range items {
		response.Items = append(response.Items, inventoryItemResponseFromStore(item))
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleGetInventoryItem(w http.ResponseWriter, r *http.Request) {
	userID := mustUserID(r.Context())
	itemID := strings.TrimSpace(r.PathValue("item_id"))
	if itemID == "" || len(itemID) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_item_id", "item_id is invalid")
		return
	}

	item, err := s.store.GetInventoryItem(r.Context(), userID, itemID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "item_not_found", "inventory item was not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "inventory_unavailable", "inventory is unavailable")
		return
	}

	writeJSON(w, http.StatusOK, inventoryItemResponseFromStore(item))
}

func (s *Server) handleListPullEvents(w http.ResponseWriter, r *http.Request) {
	userID := mustUserID(r.Context())
	limit, err := parseLimit(r, s.maxPageLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_limit", err.Error())
		return
	}
	cursor, err := parsePullEventCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "cursor is invalid")
		return
	}
	bannerID, err := parseOptionalText(r.URL.Query().Get("banner_id"), 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_banner_id", "banner_id is invalid")
		return
	}

	events, err := s.store.ListPullEvents(r.Context(), userID, store.ListPullEventsOptions{
		Cursor:   cursor,
		Limit:    limit + 1,
		BannerID: bannerID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pull_events_unavailable", "pull events are unavailable")
		return
	}

	var nextCursor string
	if len(events) > limit {
		events = events[:limit]
		nextCursor = encodePullEventCursor(events[len(events)-1])
	}

	response := pullEventListResponse{
		Items:      make([]pullEventResponse, 0, len(events)),
		NextCursor: nextCursor,
	}
	for _, event := range events {
		response.Items = append(response.Items, pullEventResponseFromStore(event))
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleGetPullEvent(w http.ResponseWriter, r *http.Request) {
	userID := mustUserID(r.Context())
	eventID, err := uuid.Parse(strings.TrimSpace(r.PathValue("event_id")))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_event_id", "event_id must be a UUID")
		return
	}

	event, records, err := s.store.GetPullEvent(r.Context(), userID, eventID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "pull_event_not_found", "pull event was not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "pull_event_unavailable", "pull event is unavailable")
		return
	}

	response := pullEventDetailResponse{
		Event:   pullEventResponseFromStore(event),
		Records: make([]pullRecordResponse, 0, len(records)),
	}
	for _, record := range records {
		response.Records = append(response.Records, pullRecordResponseFromStore(record))
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleListPullRecords(w http.ResponseWriter, r *http.Request) {
	userID := mustUserID(r.Context())
	limit, err := parseLimit(r, s.maxPageLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_limit", err.Error())
		return
	}
	cursor, err := parsePullRecordCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "cursor is invalid")
		return
	}
	bannerID, err := parseOptionalText(r.URL.Query().Get("banner_id"), 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_banner_id", "banner_id is invalid")
		return
	}
	rarity, err := parseRarity(r.URL.Query().Get("rarity"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_rarity", "rarity must be 3, 4, or 5")
		return
	}
	itemType, err := parseItemType(r.URL.Query().Get("item_type"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_item_type", "item_type must be character or weapon")
		return
	}

	records, err := s.store.ListPullRecords(r.Context(), userID, store.ListPullRecordsOptions{
		Cursor:   cursor,
		Limit:    limit + 1,
		BannerID: bannerID,
		Rarity:   rarity,
		ItemType: itemType,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pull_records_unavailable", "pull records are unavailable")
		return
	}

	var nextCursor string
	if len(records) > limit {
		records = records[:limit]
		nextCursor = encodePullRecordCursor(records[len(records)-1])
	}

	response := pullRecordListResponse{
		Items:      make([]pullRecordResponse, 0, len(records)),
		NextCursor: nextCursor,
	}
	for _, record := range records {
		response.Items = append(response.Items, pullRecordResponseFromStore(record))
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) withGatewayAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !constantTimeEqual(r.Header.Get(internalTokenHeader), s.internalToken) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "request is not authorized")
			return
		}

		userIDValue := strings.TrimSpace(r.Header.Get(userIDHeader))
		userID, err := uuid.Parse(userIDValue)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_user_id", "X-User-Id must be a UUID")
			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDContextKey{}, userID)))
	})
}

func constantTimeEqual(left string, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

type userIDContextKey struct{}

func mustUserID(ctx context.Context) uuid.UUID {
	userID, ok := ctx.Value(userIDContextKey{}).(uuid.UUID)
	if !ok {
		panic("user id missing from request context")
	}
	return userID
}

func parseLimit(r *http.Request, maxLimit int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return min(50, maxLimit), nil
	}

	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 {
		return 0, fmt.Errorf("limit must be a positive integer")
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	return limit, nil
}

func parseOptionalText(value string, maxLength int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > maxLength {
		return "", fmt.Errorf("value too long")
	}
	return value, nil
}

func parseRarity(value string) (*int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	rarity, err := strconv.Atoi(value)
	if err != nil || (rarity != 3 && rarity != 4 && rarity != 5) {
		return nil, fmt.Errorf("invalid rarity")
	}
	return &rarity, nil
}

func parseItemType(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if value != "character" && value != "weapon" {
		return "", fmt.Errorf("invalid item type")
	}
	return value, nil
}

func parseInventoryCursor(value string) (*store.InventoryCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts, err := decodeCursorParts(value)
	if err != nil || len(parts) != 2 {
		return nil, fmt.Errorf("invalid cursor")
	}
	updatedAt, err := unixNanoPart(parts[0])
	if err != nil {
		return nil, err
	}
	if parts[1] == "" {
		return nil, fmt.Errorf("invalid item id")
	}
	return &store.InventoryCursor{UpdatedAt: updatedAt, ItemID: parts[1]}, nil
}

func parsePullEventCursor(value string) (*store.PullEventCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts, err := decodeCursorParts(value)
	if err != nil || len(parts) != 2 {
		return nil, fmt.Errorf("invalid cursor")
	}
	receivedAt, err := unixNanoPart(parts[0])
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return nil, err
	}
	return &store.PullEventCursor{ReceivedAt: receivedAt, EventID: id}, nil
}

func parsePullRecordCursor(value string) (*store.PullRecordCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts, err := decodeCursorParts(value)
	if err != nil || len(parts) != 2 {
		return nil, fmt.Errorf("invalid cursor")
	}
	receivedAt, err := unixNanoPart(parts[0])
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return nil, err
	}
	return &store.PullRecordCursor{ReceivedAt: receivedAt, ID: id}, nil
}

func encodeInventoryCursor(item store.InventoryItem) string {
	return encodeCursorParts(item.UpdatedAt.UTC().UnixNano(), item.ItemID)
}

func encodePullEventCursor(event store.PullEvent) string {
	return encodeCursorParts(event.ReceivedAt.UTC().UnixNano(), event.EventID.String())
}

func encodePullRecordCursor(record store.PullRecord) string {
	return encodeCursorParts(record.ReceivedAt.UTC().UnixNano(), record.ID.String())
}

func encodeCursorParts(unixNano int64, id string) string {
	value := fmt.Sprintf("%d|%s", unixNano, id)
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeCursorParts(value string) ([]string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	return strings.SplitN(string(decoded), "|", 2), nil
}

func unixNanoPart(value string) (time.Time, error) {
	unixNano, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, unixNano).UTC(), nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, errorResponse{
		Error: apiError{
			Code:    code,
			Message: message,
		},
	})
}
