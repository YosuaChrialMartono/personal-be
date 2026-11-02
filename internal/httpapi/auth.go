package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const sessionCookie = "personal_session"
const stateCookie = "personal_oauth_state"

type identity struct {
	Email    string `json:"email"`
	Verified bool   `json:"email_verified"`
	Nonce    string `json:"nonce"`
}
type verifier interface {
	Verify(context.Context, string) (identity, error)
}
type googleVerifier struct{ verifier *oidc.IDTokenVerifier }

func (v googleVerifier) Verify(ctx context.Context, raw string) (identity, error) {
	token, err := v.verifier.Verify(ctx, raw)
	if err != nil {
		return identity{}, err
	}
	var claims identity
	err = token.Claims(&claims)
	return claims, err
}

type transaction struct {
	verifier, nonce string
	expires         time.Time
}
type Auth struct {
	mu            sync.Mutex
	sessions      map[string]time.Time
	transactions  map[string]transaction
	oauth         oauth2.Config
	verifier      verifier
	owner, origin string
	secure        bool
	now           func() time.Time
}

// NewAuth configures the single public origin. No proxy headers are trusted.
// Empty configuration disables authentication and therefore fails closed.
func NewAuth(ctx context.Context, clientID, clientSecret, owner, origin string) (*Auth, error) {
	a := &Auth{sessions: map[string]time.Time{}, transactions: map[string]transaction{}, now: time.Now}
	if clientID == "" && clientSecret == "" && owner == "" && origin == "" {
		return a, nil
	}
	if clientID == "" || clientSecret == "" || owner == "" || origin == "" {
		return nil, errors.New("all GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, OWNER_EMAIL and PUBLIC_ORIGIN are required")
	}
	addr, err := mail.ParseAddress(owner)
	if err != nil || addr.Address != owner || strings.ContainsAny(owner, " \t\r\n,") {
		return nil, errors.New("OWNER_EMAIL must be one email address")
	}
	u, err := url.Parse(origin)
	if err != nil || u.User != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"))) {
		return nil, errors.New("PUBLIC_ORIGIN must be an HTTPS origin (HTTP permitted only on localhost)")
	}
	provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		return nil, fmt.Errorf("initialize Google OIDC: %w", err)
	}
	a.owner, a.origin, a.secure = strings.ToLower(owner), origin, u.Scheme == "https"
	a.oauth = oauth2.Config{ClientID: clientID, ClientSecret: clientSecret, Endpoint: provider.Endpoint(), RedirectURL: origin + "/auth/callback", Scopes: []string{oidc.ScopeOpenID, "email"}}
	a.verifier = googleVerifier{provider.Verifier(&oidc.Config{ClientID: clientID})}
	return a, nil
}

func randomValue() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func (a *Auth) cookie(w http.ResponseWriter, name, value string, maxAge int) {
	name = a.cookieName(name)
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}
func (a *Auth) cookieName(name string) string {
	if a.secure {
		return "__Host-" + name
	}
	return name
}
func (a *Auth) cleanup() {
	now := a.now()
	for k, v := range a.sessions {
		if !now.Before(v) {
			delete(a.sessions, k)
		}
	}
	for k, v := range a.transactions {
		if !now.Before(v.expires) {
			delete(a.transactions, k)
		}
	}
}
func (a *Auth) start(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if a.verifier == nil {
		http.Error(w, "Authentication unavailable", http.StatusServiceUnavailable)
		return
	}
	state, err := randomValue()
	if err != nil {
		http.Error(w, "Authentication unavailable", 500)
		return
	}
	verifier, err := randomValue()
	if err != nil {
		http.Error(w, "Authentication unavailable", 500)
		return
	}
	nonce, err := randomValue()
	if err != nil {
		http.Error(w, "Authentication unavailable", 500)
		return
	}
	a.mu.Lock()
	a.cleanup()
	if len(a.transactions) >= 1024 {
		a.mu.Unlock()
		http.Error(w, "Authentication temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	a.transactions[state] = transaction{verifier, nonce, a.now().Add(5 * time.Minute)}
	a.mu.Unlock()
	a.cookie(w, stateCookie, state, 300)
	http.Redirect(w, r, a.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce)), http.StatusFound)
}
func (a *Auth) callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	state := r.URL.Query().Get("state")
	cookie, err := r.Cookie(a.cookieName(stateCookie))
	a.cookie(w, stateCookie, "", -1)
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
		http.Error(w, "Invalid login request", 400)
		return
	}
	a.mu.Lock()
	tx, ok := a.transactions[state]
	delete(a.transactions, state)
	a.cleanup()
	a.mu.Unlock()
	if !ok || !a.now().Before(tx.expires) || a.verifier == nil {
		http.Error(w, "Invalid login request", 400)
		return
	}
	token, err := a.oauth.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(tx.verifier))
	if err != nil {
		http.Redirect(w, r, "/login?error=login", http.StatusSeeOther)
		return
	}
	raw, _ := token.Extra("id_token").(string)
	claims, err := a.verifier.Verify(r.Context(), raw)
	if err != nil || claims.Nonce != tx.nonce || !claims.Verified || !strings.EqualFold(claims.Email, a.owner) {
		http.Redirect(w, r, "/login?error=unauthorized", http.StatusSeeOther)
		return
	}
	session, err := randomValue()
	if err != nil {
		http.Error(w, "Authentication unavailable", 500)
		return
	}
	a.mu.Lock()
	a.sessions[session] = a.now().Add(12 * time.Hour)
	a.mu.Unlock()
	a.cookie(w, sessionCookie, session, 12*60*60)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (a *Auth) authorized(r *http.Request) bool {
	c, err := r.Cookie(a.cookieName(sessionCookie))
	if err != nil || c.Value == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cleanup()
	exp, ok := a.sessions[c.Value]
	return ok && a.now().Before(exp)
}
func (a *Auth) sameOrigin(r *http.Request) bool {
	return a.origin != "" && r.Header.Get("Origin") == a.origin
}
func (a *Auth) require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !a.authorized(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "sign in required")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !a.sameOrigin(r) {
			writeError(w, http.StatusForbidden, "forbidden", "invalid origin")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (a *Auth) session(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !a.authorized(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !a.sameOrigin(r) {
		http.Error(w, "Invalid origin", http.StatusForbidden)
		return
	}
	if c, err := r.Cookie(a.cookieName(sessionCookie)); err == nil {
		a.mu.Lock()
		delete(a.sessions, c.Value)
		a.mu.Unlock()
	}
	a.cookie(w, sessionCookie, "", -1)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
