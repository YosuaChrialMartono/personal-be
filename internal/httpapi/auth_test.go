package httpapi

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/oauth2"
)

type mockVerifier struct {
	claims identity
	err    error
}

func (m mockVerifier) Verify(_ context.Context, token string) (identity, error) {
	if token != "mock-id-token" {
		return identity{}, errors.New("bad token")
	}
	return m.claims, m.err
}

func TestAuthenticationFlow(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.Form.Get("code_verifier") == "" {
			t.Error("missing PKCE verifier")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"mock-access","token_type":"Bearer","id_token":"mock-id-token"}`)
	}))
	defer provider.Close()
	now := time.Now()
	a := &Auth{sessions: map[string]time.Time{}, transactions: map[string]transaction{}, origin: "http://localhost:3000", owner: "owner@example.com", now: func() time.Time { return now }, oauth: oauth2.Config{ClientID: "test", ClientSecret: "test", Endpoint: oauth2.Endpoint{AuthURL: provider.URL + "/authorize", TokenURL: provider.URL}, RedirectURL: "http://localhost:3000/auth/callback"}}
	login := func(claims identity, verifyErr error) (*httptest.ResponseRecorder, string) {
		a.verifier = mockVerifier{claims: claims, err: verifyErr}
		start := httptest.NewRecorder()
		a.start(start, httptest.NewRequest("GET", "/auth/login", nil))
		u, _ := url.Parse(start.Header().Get("Location"))
		state := u.Query().Get("state")
		if state == "" || u.Query().Get("code_challenge_method") != "S256" {
			t.Fatal("missing state or PKCE challenge")
		}
		tx := a.transactions[state]
		claims.Nonce = tx.nonce
		a.verifier = mockVerifier{claims: claims, err: verifyErr}
		r := httptest.NewRequest("GET", "/auth/callback?state="+state+"&code=test", nil)
		r.AddCookie(start.Result().Cookies()[0])
		w := httptest.NewRecorder()
		a.callback(w, r)
		return w, state
	}
	for _, tc := range []struct {
		name   string
		claims identity
		err    error
	}{
		{"nonowner", identity{Email: "other@example.com", Verified: true}, nil},
		{"unverified", identity{Email: "owner@example.com"}, nil},
		{"invalid token", identity{Email: "owner@example.com", Verified: true}, errors.New("invalid")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, _ := login(tc.claims, tc.err)
			if w.Header().Get("Location") != "/login?error=unauthorized" || len(a.sessions) != 0 {
				t.Fatal("identity accepted")
			}
		})
	}
	w, state := login(identity{Email: "owner@example.com", Verified: true}, nil)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" || len(a.sessions) != 1 {
		t.Fatal("owner not signed in")
	}
	var session *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			session = c
		}
	}
	if session == nil || !session.HttpOnly || session.MaxAge <= 0 {
		t.Fatal("session cookie invalid")
	}
	request := httptest.NewRequest("GET", "/auth/session", nil)
	request.AddCookie(session)
	if !a.authorized(request) {
		t.Fatal("session not accepted")
	}
	bad := httptest.NewRecorder()
	a.callback(bad, httptest.NewRequest("GET", "/auth/callback?state="+state, nil))
	if bad.Code != 400 {
		t.Fatal("state replay accepted")
	}
	logout := httptest.NewRequest("POST", "/auth/logout", nil)
	logout.AddCookie(session)
	bad = httptest.NewRecorder()
	a.logout(bad, logout)
	if bad.Code != 403 || !a.authorized(request) {
		t.Fatal("cross-site logout accepted")
	}
	logout.Header.Set("Origin", a.origin)
	a.logout(httptest.NewRecorder(), logout)
	if a.authorized(request) {
		t.Fatal("logout did not invalidate")
	}
	w, _ = login(identity{Email: "owner@example.com", Verified: true}, nil)
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			session = c
		}
	}
	request = httptest.NewRequest("GET", "/auth/session", nil)
	request.AddCookie(session)
	now = now.Add(13 * time.Hour)
	if a.authorized(request) {
		t.Fatal("expired session accepted")
	}
}

func TestAuthProtectsWishlist(t *testing.T) {
	a := &Auth{sessions: map[string]time.Time{"valid": time.Now().Add(time.Hour)}, transactions: map[string]transaction{}, origin: "http://localhost:3000", now: time.Now}
	h := NewRouter(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, a)
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		path := "/api/v1/wishlist/items"
		if method == "PUT" {
			path += "/order"
		}
		if method == "PATCH" || method == "DELETE" {
			path += "/id"
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader("{}")))
		if w.Code != 401 {
			t.Fatalf("%s: got %d", method, w.Code)
		}
		if method != "GET" {
			r := httptest.NewRequest(method, path, nil)
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "valid"})
			r.Header.Set("Origin", "https://attacker.example")
			w = httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 403 {
				t.Fatalf("%s CSRF: got %d", method, w.Code)
			}
		}
	}
	r := httptest.NewRequest("GET", "/auth/session", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "invalid"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("invalid session accepted")
	}
	unconfigured, err := NewAuth(context.Background(), "", "", "", "")
	if err != nil || unconfigured.verifier != nil {
		t.Fatal("empty config must fail closed")
	}
	w = httptest.NewRecorder()
	unconfigured.start(w, httptest.NewRequest("GET", "/auth/login", nil))
	if w.Code != 503 {
		t.Fatal("unconfigured auth enabled")
	}
}

func TestRejectedTransactionsAndSecureCookies(t *testing.T) {
	now := time.Now()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"access","token_type":"Bearer","id_token":"mock-id-token"}`)
	}))
	defer provider.Close()
	a := &Auth{sessions: map[string]time.Time{}, transactions: map[string]transaction{}, now: func() time.Time { return now }, secure: true, owner: "owner@example.com", verifier: mockVerifier{claims: identity{Email: "owner@example.com", Verified: true, Nonce: "wrong"}}, oauth: oauth2.Config{ClientID: "test", Endpoint: oauth2.Endpoint{TokenURL: provider.URL}}}
	for _, tc := range []struct {
		name, state, cookie string
		expiry              time.Time
		want                int
	}{
		{"mismatched state", "state", "other", now.Add(time.Minute), 400},
		{"expired state", "state", "state", now.Add(-time.Second), 400},
		{"nonce mismatch", "state", "state", now.Add(time.Minute), 303},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a.transactions[tc.state] = transaction{verifier: "verifier", nonce: "expected", expires: tc.expiry}
			r := httptest.NewRequest("GET", "/auth/callback?state="+tc.state+"&code=test", nil)
			r.AddCookie(&http.Cookie{Name: "__Host-" + stateCookie, Value: tc.cookie})
			w := httptest.NewRecorder()
			a.callback(w, r)
			if w.Code != tc.want || len(a.sessions) != 0 {
				t.Fatalf("unexpected status or session: %d", w.Code)
			}
			if tc.name == "nonce mismatch" && w.Header().Get("Location") != "/login?error=unauthorized" {
				t.Fatal("nonce mismatch accepted")
			}
		})
	}
	w := httptest.NewRecorder()
	a.cookie(w, sessionCookie, "value", 10)
	c := w.Result().Cookies()[0]
	if c.Name != "__Host-"+sessionCookie || !c.Secure || !c.HttpOnly || c.Path != "/" || c.Domain != "" {
		t.Fatalf("insecure production cookie: %+v", c)
	}
}

func TestGoogleVerifierValidatesSignedTokens(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	v := googleVerifier{oidc.NewVerifier("https://accounts.google.com", &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}, &oidc.Config{ClientID: "client"})}
	for _, tc := range []struct {
		name, issuer, audience string
		expiry                 time.Time
		signingKey             *rsa.PrivateKey
		valid                  bool
	}{
		{"valid", "https://accounts.google.com", "client", time.Now().Add(time.Hour), key, true},
		{"bad issuer", "https://attacker.example", "client", time.Now().Add(time.Hour), key, false},
		{"bad audience", "https://accounts.google.com", "other", time.Now().Add(time.Hour), key, false},
		{"expired", "https://accounts.google.com", "client", time.Now().Add(-time.Hour), key, false},
		{"bad signature", "https://accounts.google.com", "client", time.Now().Add(time.Hour), other, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: tc.signingKey}, nil)
			if err != nil {
				t.Fatal(err)
			}
			token, err := jwt.Signed(signer).Claims(map[string]any{"iss": tc.issuer, "aud": tc.audience, "sub": "123", "exp": tc.expiry.Unix(), "iat": time.Now().Unix(), "email": "owner@example.com", "email_verified": true, "nonce": "test"}).Serialize()
			if err != nil {
				t.Fatal(err)
			}
			claims, err := v.Verify(context.Background(), token)
			if (err == nil) != tc.valid {
				t.Fatalf("verification error = %v", err)
			}
			if tc.valid && (!claims.Verified || claims.Email != "owner@example.com" || claims.Nonce != "test") {
				t.Fatal("claims not decoded")
			}
		})
	}
}
