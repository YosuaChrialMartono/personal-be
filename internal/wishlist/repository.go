package wishlist

import "context"

type Repository interface {
	List(ctx context.Context) ([]Item, error)
	Get(ctx context.Context, id string) (Item, error)
	Create(ctx context.Context, input CreateInput) (Item, error)
	Update(ctx context.Context, id string, input CreateInput, updatedAt string) (Item, error)
	Delete(ctx context.Context, id string) error
	Reorder(ctx context.Context, ids []string) error
}
