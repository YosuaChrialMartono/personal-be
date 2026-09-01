CREATE TABLE wishlist_items (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    price_minor INTEGER NOT NULL CHECK (price_minor >= 0),
    currency TEXT NOT NULL CHECK (currency IN ('IDR', 'USD', 'SGD', 'MYR', 'EUR')),
    position INTEGER NOT NULL CHECK (position >= 0) UNIQUE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
