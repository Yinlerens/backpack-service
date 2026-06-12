package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pg pool: %w", err)
	}

	store := &Store{pool: pool}
	if err := store.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return nil
}

func (s *Store) ApplyPullCompletedEvent(ctx context.Context, event PullCompletedEvent) (bool, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin transaction: %w", err)
	}
	defer rollback(ctx, tx)

	created, err := insertPullEvent(ctx, tx, event)
	if err != nil {
		return false, err
	}
	if !created {
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit duplicate event transaction: %w", err)
		}
		return false, nil
	}

	for _, record := range event.Records {
		record.EventID = event.EventID
		record.UserID = event.UserID
		if err := insertPullRecord(ctx, tx, record); err != nil {
			return false, err
		}
		if err := upsertInventoryItem(ctx, tx, record); err != nil {
			return false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit event transaction: %w", err)
	}

	return true, nil
}

func (s *Store) ListInventoryItems(ctx context.Context, userID uuid.UUID, opts ListInventoryOptions) ([]InventoryItem, error) {
	limit := normalizeLimit(opts.Limit)
	if opts.Cursor == nil {
		rows, err := s.pool.Query(ctx, `
			select user_id, item_id, item_name, item_type, rarity, quantity,
			       first_received_at, updated_at
			from backpack.inventory_items
			where user_id = $1
			order by updated_at desc, item_id desc
			limit $2
		`, userID, limit)
		return scanInventoryRows(rows, err, limit)
	}

	rows, err := s.pool.Query(ctx, `
		select user_id, item_id, item_name, item_type, rarity, quantity,
		       first_received_at, updated_at
		from backpack.inventory_items
		where user_id = $1
		  and (updated_at, item_id) < ($2, $3)
		order by updated_at desc, item_id desc
		limit $4
	`, userID, opts.Cursor.UpdatedAt, opts.Cursor.ItemID, limit)
	return scanInventoryRows(rows, err, limit)
}

func (s *Store) GetInventoryItem(ctx context.Context, userID uuid.UUID, itemID string) (InventoryItem, error) {
	item, err := scanInventoryItem(s.pool.QueryRow(ctx, `
		select user_id, item_id, item_name, item_type, rarity, quantity,
		       first_received_at, updated_at
		from backpack.inventory_items
		where user_id = $1 and item_id = $2
	`, userID, itemID))
	if errors.Is(err, pgx.ErrNoRows) {
		return InventoryItem{}, ErrNotFound
	}
	return item, err
}

func (s *Store) ListPullEvents(ctx context.Context, userID uuid.UUID, opts ListPullEventsOptions) ([]PullEvent, error) {
	limit := normalizeLimit(opts.Limit)
	args := []any{userID}
	conditions := []string{"user_id = $1"}

	if opts.BannerID != "" {
		args = append(args, opts.BannerID)
		conditions = append(conditions, fmt.Sprintf("banner_id = $%d", len(args)))
	}
	if opts.Cursor != nil {
		args = append(args, opts.Cursor.ReceivedAt, opts.Cursor.EventID)
		conditions = append(conditions, fmt.Sprintf("(received_at, event_id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, limit)

	query := fmt.Sprintf(`
		select event_id, user_id, event_type, banner_id, seed, state_version,
		       previous_pity, next_pity, raw_event, received_at
		from backpack.pull_events
		where %s
		order by received_at desc, event_id desc
		limit $%d
	`, strings.Join(conditions, " and "), len(args))

	rows, err := s.pool.Query(ctx, query, args...)
	return scanPullEventRows(rows, err, limit)
}

func (s *Store) GetPullEvent(ctx context.Context, userID uuid.UUID, eventID uuid.UUID) (PullEvent, []PullRecord, error) {
	event, err := scanPullEvent(s.pool.QueryRow(ctx, `
		select event_id, user_id, event_type, banner_id, seed, state_version,
		       previous_pity, next_pity, raw_event, received_at
		from backpack.pull_events
		where user_id = $1 and event_id = $2
	`, userID, eventID))
	if errors.Is(err, pgx.ErrNoRows) {
		return PullEvent{}, nil, ErrNotFound
	}
	if err != nil {
		return PullEvent{}, nil, err
	}

	rows, err := s.pool.Query(ctx, `
		select id, event_id, user_id, record_index, item_id, item_name, item_type,
		       rarity, banner_id, banner_name, pity_at_five, pity_at_four,
		       is_featured, received_at
		from backpack.pull_records
		where user_id = $1 and event_id = $2
		order by record_index asc
	`, userID, eventID)
	records, err := scanPullRecordRows(rows, err, 100)
	if err != nil {
		return PullEvent{}, nil, err
	}

	return event, records, nil
}

func (s *Store) ListPullRecords(ctx context.Context, userID uuid.UUID, opts ListPullRecordsOptions) ([]PullRecord, error) {
	limit := normalizeLimit(opts.Limit)
	args := []any{userID}
	conditions := []string{"user_id = $1"}

	if opts.BannerID != "" {
		args = append(args, opts.BannerID)
		conditions = append(conditions, fmt.Sprintf("banner_id = $%d", len(args)))
	}
	if opts.Rarity != nil {
		args = append(args, *opts.Rarity)
		conditions = append(conditions, fmt.Sprintf("rarity = $%d", len(args)))
	}
	if opts.ItemType != "" {
		args = append(args, opts.ItemType)
		conditions = append(conditions, fmt.Sprintf("item_type = $%d", len(args)))
	}
	if opts.Cursor != nil {
		args = append(args, opts.Cursor.ReceivedAt, opts.Cursor.ID)
		conditions = append(conditions, fmt.Sprintf("(received_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, limit)

	query := fmt.Sprintf(`
		select id, event_id, user_id, record_index, item_id, item_name, item_type,
		       rarity, banner_id, banner_name, pity_at_five, pity_at_four,
		       is_featured, received_at
		from backpack.pull_records
		where %s
		order by received_at desc, id desc
		limit $%d
	`, strings.Join(conditions, " and "), len(args))

	rows, err := s.pool.Query(ctx, query, args...)
	return scanPullRecordRows(rows, err, limit)
}

func insertPullEvent(ctx context.Context, tx pgx.Tx, event PullCompletedEvent) (bool, error) {
	tag, err := tx.Exec(ctx, `
		insert into backpack.pull_events (
			event_id, user_id, event_type, banner_id, seed, state_version,
			previous_pity, next_pity, raw_event
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		on conflict (event_id) do nothing
	`, event.EventID, event.UserID, event.EventType, event.BannerID, event.Seed,
		event.StateVersion, event.PreviousPity, event.NextPity, event.RawEvent)
	if err != nil {
		return false, fmt.Errorf("insert pull event: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func insertPullRecord(ctx context.Context, tx pgx.Tx, record PullRecord) error {
	if _, err := tx.Exec(ctx, `
		insert into backpack.pull_records (
			id, event_id, user_id, record_index, item_id, item_name, item_type,
			rarity, banner_id, banner_name, pity_at_five, pity_at_four, is_featured
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, record.ID, record.EventID, record.UserID, record.Index, record.ItemID,
		record.ItemName, record.ItemType, record.Rarity, record.BannerID,
		record.BannerName, record.PityAtFive, record.PityAtFour, record.IsFeatured); err != nil {
		return fmt.Errorf("insert pull record: %w", err)
	}
	return nil
}

func upsertInventoryItem(ctx context.Context, tx pgx.Tx, record PullRecord) error {
	if _, err := tx.Exec(ctx, `
		insert into backpack.inventory_items (
			user_id, item_id, item_name, item_type, rarity, quantity
		)
		values ($1, $2, $3, $4, $5, 1)
		on conflict (user_id, item_id) do update
		set quantity = backpack.inventory_items.quantity + 1,
		    item_name = excluded.item_name,
		    item_type = excluded.item_type,
		    rarity = excluded.rarity,
		    updated_at = now()
	`, record.UserID, record.ItemID, record.ItemName, record.ItemType, record.Rarity); err != nil {
		return fmt.Errorf("upsert inventory item: %w", err)
	}
	return nil
}

func scanInventoryRows(rows pgx.Rows, err error, limit int) ([]InventoryItem, error) {
	if err != nil {
		return nil, fmt.Errorf("query inventory items: %w", err)
	}
	defer rows.Close()

	items := make([]InventoryItem, 0, limit)
	for rows.Next() {
		item, err := scanInventoryItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate inventory items: %w", err)
	}
	return items, nil
}

func scanPullEventRows(rows pgx.Rows, err error, limit int) ([]PullEvent, error) {
	if err != nil {
		return nil, fmt.Errorf("query pull events: %w", err)
	}
	defer rows.Close()

	events := make([]PullEvent, 0, limit)
	for rows.Next() {
		event, err := scanPullEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pull events: %w", err)
	}
	return events, nil
}

func scanPullRecordRows(rows pgx.Rows, err error, limit int) ([]PullRecord, error) {
	if err != nil {
		return nil, fmt.Errorf("query pull records: %w", err)
	}
	defer rows.Close()

	records := make([]PullRecord, 0, limit)
	for rows.Next() {
		record, err := scanPullRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pull records: %w", err)
	}
	return records, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanInventoryItem(row rowScanner) (InventoryItem, error) {
	var item InventoryItem
	if err := row.Scan(
		&item.UserID,
		&item.ItemID,
		&item.ItemName,
		&item.ItemType,
		&item.Rarity,
		&item.Quantity,
		&item.FirstReceivedAt,
		&item.UpdatedAt,
	); err != nil {
		return InventoryItem{}, fmt.Errorf("scan inventory item: %w", err)
	}
	return item, nil
}

func scanPullEvent(row rowScanner) (PullEvent, error) {
	var event PullEvent
	if err := row.Scan(
		&event.EventID,
		&event.UserID,
		&event.EventType,
		&event.BannerID,
		&event.Seed,
		&event.StateVersion,
		&event.PreviousPity,
		&event.NextPity,
		&event.RawEvent,
		&event.ReceivedAt,
	); err != nil {
		return PullEvent{}, fmt.Errorf("scan pull event: %w", err)
	}
	return event, nil
}

func scanPullRecord(row rowScanner) (PullRecord, error) {
	var record PullRecord
	if err := row.Scan(
		&record.ID,
		&record.EventID,
		&record.UserID,
		&record.Index,
		&record.ItemID,
		&record.ItemName,
		&record.ItemType,
		&record.Rarity,
		&record.BannerID,
		&record.BannerName,
		&record.PityAtFive,
		&record.PityAtFour,
		&record.IsFeatured,
		&record.ReceivedAt,
	); err != nil {
		return PullRecord{}, fmt.Errorf("scan pull record: %w", err)
	}
	return record, nil
}

func normalizeLimit(limit int) int {
	if limit < 1 {
		return 50
	}
	return limit
}

func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
