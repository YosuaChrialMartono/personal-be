package wishlist

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context) ([]Item, error) {
	return s.repository.List(ctx)
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Item, error) {
	input = normalizeCreate(input)
	if err := validateCreate(input); err != nil {
		return Item{}, err
	}
	return s.repository.Create(ctx, input)
}

func (s *Service) Update(ctx context.Context, id string, input UpdateInput) (Item, error) {
	if input.Title == nil && input.Description == nil && input.PriceMinor == nil && input.Currency == nil {
		return Item{}, ValidationError{Message: "at least one field is required"}
	}
	existing, err := s.repository.Get(ctx, id)
	if err != nil {
		return Item{}, err
	}
	merged := CreateInput{
		Title: existing.Title, Description: existing.Description,
		PriceMinor: existing.PriceMinor, Currency: existing.Currency,
	}
	if input.Title != nil {
		merged.Title = strings.TrimSpace(*input.Title)
	}
	if input.Description != nil {
		merged.Description = strings.TrimSpace(*input.Description)
	}
	if input.PriceMinor != nil {
		merged.PriceMinor = *input.PriceMinor
	}
	if input.Currency != nil {
		merged.Currency = *input.Currency
	}
	if err := validateCreate(merged); err != nil {
		return Item{}, err
	}
	return s.repository.Update(ctx, id, merged, timestamp())
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repository.Delete(ctx, id)
}

func (s *Service) Reorder(ctx context.Context, ids []string) error {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return ValidationError{Message: "itemIds must contain valid UUIDs"}
		}
		if _, exists := seen[id]; exists {
			return ValidationError{Message: "itemIds must not contain duplicates"}
		}
		seen[id] = struct{}{}
	}
	return s.repository.Reorder(ctx, ids)
}

func normalizeCreate(input CreateInput) CreateInput {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	return input
}

func validateCreate(input CreateInput) error {
	switch {
	case input.Title == "":
		return ValidationError{Message: "title is required"}
	case len([]rune(input.Title)) > 200:
		return ValidationError{Message: "title must be at most 200 characters"}
	case len([]rune(input.Description)) > 2000:
		return ValidationError{Message: "description must be at most 2000 characters"}
	case input.PriceMinor < 0 || input.PriceMinor > MaxPriceMinor:
		return ValidationError{Message: "priceMinor must be a non-negative safe integer"}
	case !supportedCurrencies[input.Currency]:
		return ValidationError{Message: "currency is not supported"}
	default:
		return nil
	}
}

var supportedCurrencies = map[string]bool{
	"IDR": true, "USD": true, "SGD": true, "MYR": true, "EUR": true,
}

func timestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func newID() string {
	return uuid.New().String()
}

func AsValidationError(err error, target *ValidationError) bool {
	if err == nil {
		return false
	}
	if validation, ok := err.(ValidationError); ok {
		*target = validation
		return true
	}
	return false
}
