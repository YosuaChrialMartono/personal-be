package wishlist

import (
	"errors"
	"time"
)

const MaxPriceMinor int64 = 9007199254740991

var (
	ErrNotFound   = errors.New("wishlist item not found")
	ErrConflict   = errors.New("wishlist order conflicts with current items")
	ErrValidation = errors.New("wishlist validation failed")
)

type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string { return e.Message }
func (e ValidationError) Unwrap() error { return ErrValidation }

type Item struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	PriceMinor  int64     `json:"priceMinor"`
	Currency    string    `json:"currency"`
	Position    int64     `json:"position"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CreateInput struct {
	Title       string
	Description string
	PriceMinor  int64
	Currency    string
}

type UpdateInput struct {
	Title       *string
	Description *string
	PriceMinor  *int64
	Currency    *string
}
