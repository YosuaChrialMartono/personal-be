-- name: ListWishlistItems :many
SELECT id, title, description, price_minor, currency, position, created_at, updated_at
FROM wishlist_items
ORDER BY position ASC;

-- name: GetWishlistItem :one
SELECT id, title, description, price_minor, currency, position, created_at, updated_at
FROM wishlist_items
WHERE id = ?;

-- name: CreateWishlistItem :one
INSERT INTO wishlist_items (id, title, description, price_minor, currency, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, title, description, price_minor, currency, position, created_at, updated_at;

-- name: UpdateWishlistItem :one
UPDATE wishlist_items
SET title = ?, description = ?, price_minor = ?, currency = ?, updated_at = ?
WHERE id = ?
RETURNING id, title, description, price_minor, currency, position, created_at, updated_at;

-- name: DeleteWishlistItem :execresult
DELETE FROM wishlist_items WHERE id = ?;

-- name: MaxWishlistPosition :one
SELECT CAST(COALESCE(MAX(position), -1) AS INTEGER) AS max_position FROM wishlist_items;

-- name: SetWishlistItemPosition :exec
UPDATE wishlist_items SET position = ? WHERE id = ?;
