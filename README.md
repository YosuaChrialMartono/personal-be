# Personal BE

Go backend for personal tools. The first module is the wishlist API.

## Requirements

- Go 1.26.10 or newer (including the compiler used to build production binaries)
- Docker and Docker Compose for the container workflow
- sqlc through the Go tool dependency declared in `go.mod`

## Local Development

```bash
cp .env.example .env
make generate
make migrate-up
make run
```

The `make run` target does not load `.env`; export the variables in your shell
or use Compose, which reads `.env`. See **Owner authentication** below before
trying to sign in. With no auth configuration, the server starts but all
wishlist requests fail closed; partial configuration prevents startup.

The API listens on `:8080` by default and uses `./data/personal.db`. Set `PORT`,
`DATABASE_PATH`, and comma-separated `ALLOWED_ORIGINS` to override defaults.
For a directly running local API, set `LISTEN_HOST=127.0.0.1` to bind only to
loopback. Compose instead binds its published host port to loopback.

Run `make migrate-down` only for local development. The API does not alter the
database schema at startup.

## Docker

```bash
docker compose up --build
```

Compose runs migrations before the API, stores SQLite in the `personal-data`
named volume, and binds the API to `127.0.0.1:49173`. Keep the API private;
route public traffic to Next.js, not directly to the API.

Set `HOST_PORT` and `ALLOWED_ORIGINS` in `.env` for local or later VPS use.

## Owner authentication

Create a Google Cloud OAuth **Web application** client and configure its
consent screen. Register exactly `<PUBLIC_ORIGIN>/auth/callback` as an authorized
redirect URI, e.g. `http://localhost:3000/auth/callback` for development or
`https://tools.example.com/auth/callback` in production. If the consent screen
is in testing mode, add the owner as a test user in Google Cloud. Set these
variables privately on the backend (example values, not credentials):

```text
PUBLIC_ORIGIN=http://localhost:3000
GOOGLE_CLIENT_ID=<Google web client ID>
GOOGLE_CLIENT_SECRET=<Google web client secret>
OWNER_EMAIL=<one verified Google account email>
```

Configure the frontend `BACKEND_ORIGIN` as documented in its README. The
public origin is the exact browser-facing scheme, host and port, with no path
or trailing slash. It is fixed configuration, not inferred from forwarded
headers. HTTPS is required except on localhost/127.0.0.1; HTTPS enables the
Secure cookie flag. HTTP development cookies are HttpOnly and SameSite=Lax,
but not Secure. Do not use HTTP on a network. Wishlist mutations and logout
require an exact `Origin` match; reverse proxies must preserve this header.

Google ID tokens are verified by go-oidc and the email must be verified and
match the one configured owner. OAuth state, nonce and PKCE bind each login.
An opaque, HttpOnly session expires after 12 hours; logout invalidates it.
Sessions and login transactions live in backend memory: a restart logs everyone
out and a deployment must use **one backend instance** (no load-balanced
replicas). Keep the backend loopback/private, protect the TLS-to-Next.js and
Next.js-to-backend links, and configure the edge proxy to accept only the
intended host and discard untrusted forwarding headers. Do not cache protected
responses, log OAuth query strings/cookies, or expose `.env` files. `/healthz`
remains public and does not disclose account information.

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
