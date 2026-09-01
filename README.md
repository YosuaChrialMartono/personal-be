# Personal BE

Go backend for personal tools. The first module is the wishlist API.

## Requirements

- Go 1.26
- Docker and Docker Compose for the container workflow
- sqlc through the Go tool dependency declared in `go.mod`

## Local Development

```bash
cp .env.example .env
make generate
make migrate-up
make run
```

The API listens on `:8080` by default and uses `./data/personal.db`. Set `PORT`,
`DATABASE_PATH`, and comma-separated `ALLOWED_ORIGINS` to override defaults.

Run `make migrate-down` only for local development. The API does not alter the
database schema at startup.

## Docker

```bash
docker compose up --build
```

Compose runs migrations before the API, stores SQLite in the `personal-data`
named volume, and binds the API to `127.0.0.1:49173`. A Cloudflare Tunnel can
later route a subdomain to `http://localhost:49173`.

Set `HOST_PORT` and `ALLOWED_ORIGINS` in `.env` for local or later VPS use.

## Wishlist API

```text
GET    /healthz
GET    /api/v1/wishlist/items
POST   /api/v1/wishlist/items
PATCH  /api/v1/wishlist/items/{id}
DELETE /api/v1/wishlist/items/{id}
PUT    /api/v1/wishlist/items/order
```

Items are returned in ascending `position` order. Prices use integer minor
units: IDR is stored in rupiah, while USD, SGD, MYR, and EUR are stored in
cents. New items append, deletes compact positions, and reorder requests must
include every current item ID exactly once.

Errors use this shape:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "title is required"
  }
}
```

## Verification

```bash
make generate
go test ./...
go vet ./...
```
