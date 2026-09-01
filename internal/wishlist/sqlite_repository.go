package wishlist

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/YosuaChrialMartono/personal-be/internal/store"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

func (r *SQLiteRepository) List(ctx context.Context) ([]Item, error) {
	rows, err := store.New(r.db).ListWishlistItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("list wishlist items: %w", err)
	}
	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		item, err := fromStore(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *SQLiteRepository) Get(ctx context.Context, id string) (Item, error) {
	row, err := store.New(r.db).GetWishlistItem(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("get wishlist item: %w", err)
	}
	return fromStore(row)
}

func (r *SQLiteRepository) Create(ctx context.Context, input CreateInput) (Item, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, fmt.Errorf("begin create transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	queries := store.New(tx)
	maxPosition, err := queries.MaxWishlistPosition(ctx)
	if err != nil {
		return Item{}, fmt.Errorf("find append position: %w", err)
	}
	row, err := queries.CreateWishlistItem(ctx, store.CreateWishlistItemParams{
		ID: newID(), Title: input.Title, Description: input.Description,
		PriceMinor: input.PriceMinor, Currency: input.Currency,
		Position: maxPosition + 1, CreatedAt: timestamp(), UpdatedAt: timestamp(),
	})
	if err != nil {
		return Item{}, fmt.Errorf("create wishlist item: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Item{}, fmt.Errorf("commit create transaction: %w", err)
	}
	return fromStore(row)
}

func (r *SQLiteRepository) Update(ctx context.Context, id string, input CreateInput, updatedAt string) (Item, error) {
	row, err := store.New(r.db).UpdateWishlistItem(ctx, store.UpdateWishlistItemParams{
		Title: input.Title, Description: input.Description, PriceMinor: input.PriceMinor,
		Currency: input.Currency, UpdatedAt: updatedAt, ID: id,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("update wishlist item: %w", err)
	}
	return fromStore(row)
}

func (r *SQLiteRepository) Delete(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	queries := store.New(tx)
	result, err := queries.DeleteWishlistItem(ctx, id)
	if err != nil {
		return fmt.Errorf("delete wishlist item: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect delete result: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	if err := compactPositions(ctx, queries); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete transaction: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) Reorder(ctx context.Context, ids []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin reorder transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	queries := store.New(tx)
	current, err := queries.ListWishlistItems(ctx)
	if err != nil {
		return fmt.Errorf("list items for reorder: %w", err)
	}
	if len(current) != len(ids) {
		return ErrConflict
	}
	currentIDs := make(map[string]struct{}, len(current))
	for _, item := range current {
		currentIDs[item.ID] = struct{}{}
	}
	for _, id := range ids {
		if _, ok := currentIDs[id]; !ok {
			return ErrConflict
		}
	}

	if err := setTemporaryPositions(ctx, queries, current); err != nil {
		return err
	}
	for position, id := range ids {
		if err := queries.SetWishlistItemPosition(ctx, store.SetWishlistItemPositionParams{Position: int64(position), ID: id}); err != nil {
			return fmt.Errorf("set reordered position: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reorder transaction: %w", err)
	}
	return nil
}

func compactPositions(ctx context.Context, queries *store.Queries) error {
	rows, err := queries.ListWishlistItems(ctx)
	if err != nil {
		return fmt.Errorf("list items for compaction: %w", err)
	}
	if err := setTemporaryPositions(ctx, queries, rows); err != nil {
		return err
	}
	for position, row := range rows {
		if err := queries.SetWishlistItemPosition(ctx, store.SetWishlistItemPositionParams{Position: int64(position), ID: row.ID}); err != nil {
			return fmt.Errorf("set compacted position: %w", err)
		}
	}
	return nil
}

func setTemporaryPositions(ctx context.Context, queries *store.Queries, rows []store.WishlistItem) error {
	base := int64(len(rows)) + 1
	for index, row := range rows {
		if err := queries.SetWishlistItemPosition(ctx, store.SetWishlistItemPositionParams{Position: base + int64(index), ID: row.ID}); err != nil {
			return fmt.Errorf("set temporary position: %w", err)
		}
	}
	return nil
}

func fromStore(row store.WishlistItem) (Item, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, row.CreatedAt)
	if err != nil {
		return Item{}, fmt.Errorf("parse created_at: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, row.UpdatedAt)
	if err != nil {
		return Item{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return Item{ID: row.ID, Title: row.Title, Description: row.Description, PriceMinor: row.PriceMinor,
		Currency: row.Currency, Position: row.Position, CreatedAt: createdAt, UpdatedAt: updatedAt}, nil
}
