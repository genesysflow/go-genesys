package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/http"
)

// RememberTokenProvider is implemented by user providers that support
// remember-me tokens.
type RememberTokenProvider interface {
	// RetrieveByRememberToken returns the user with the given id whose
	// stored remember token matches, or ErrUserNotFound.
	RetrieveByRememberToken(id any, token string) (Authenticatable, error)

	// UpdateRememberToken stores a new remember token for the user.
	UpdateRememberToken(user Authenticatable, token string) error
}

// rememberCookieName is the guard's persistent-login cookie.
func (g *SessionGuard) rememberCookieName() string {
	if g.RememberCookie != "" {
		return g.RememberCookie
	}
	return "remember_" + g.name
}

func (g *SessionGuard) rememberTTL() time.Duration {
	if g.RememberTTL > 0 {
		return g.RememberTTL
	}
	return 30 * 24 * time.Hour
}

func newRememberToken() string {
	raw := make([]byte, 30)
	rand.Read(raw)
	return hex.EncodeToString(raw)
}

// Remember logs the user in and issues a persistent-login cookie, so
// the session guard can re-authenticate them after the session expires.
// The user provider must implement RememberTokenProvider.
func (g *SessionGuard) Remember(ctx *http.Context, user Authenticatable) error {
	rememberer, ok := g.provider.(RememberTokenProvider)
	if !ok {
		return fmt.Errorf("auth: user provider %T does not support remember tokens", g.provider)
	}
	if err := g.Login(ctx, user); err != nil {
		return err
	}

	token := newRememberToken()
	if err := rememberer.UpdateRememberToken(user, token); err != nil {
		return err
	}
	ctx.Cookie(&contracts.Cookie{
		Name:     g.rememberCookieName(),
		Value:    fmt.Sprint(user.GetAuthIdentifier()) + "|" + token,
		Path:     "/",
		MaxAge:   int(g.rememberTTL().Seconds()),
		Secure:   !g.RememberCookieInsecure,
		HTTPOnly: true,
		SameSite: "Lax",
	})
	return nil
}

// AttemptRemember is Attempt followed by Remember on success.
func (g *SessionGuard) AttemptRemember(ctx *http.Context, credentials map[string]any) (Authenticatable, error) {
	user, err := g.Attempt(ctx, credentials)
	if err != nil {
		return nil, err
	}
	if err := g.Remember(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// userFromRememberCookie recovers the user from the persistent-login
// cookie when the session holds no login. On success the session is
// re-established so later requests skip the cookie path.
func (g *SessionGuard) userFromRememberCookie(ctx *http.Context) Authenticatable {
	rememberer, ok := g.provider.(RememberTokenProvider)
	if !ok {
		return nil
	}
	cookie := ctx.FiberCtx().Cookies(g.rememberCookieName())
	if cookie == "" {
		return nil
	}
	id, token, found := strings.Cut(cookie, "|")
	if !found || id == "" || token == "" {
		return nil
	}
	user, err := rememberer.RetrieveByRememberToken(id, token)
	if err != nil || user == nil {
		return nil
	}
	if sess := g.session(ctx); sess != nil {
		sess.Regenerate()
		sess.Set(sessionKey, fmt.Sprint(user.GetAuthIdentifier()))
	}
	return user
}

// forgetRememberCookie clears the persistent login: the cookie is
// expired and the stored token cycled so old cookies die everywhere.
func (g *SessionGuard) forgetRememberCookie(ctx *http.Context, user Authenticatable) {
	ctx.Cookie(&contracts.Cookie{
		Name:     g.rememberCookieName(),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   !g.RememberCookieInsecure,
		HTTPOnly: true,
		SameSite: "Lax",
	})
	if rememberer, ok := g.provider.(RememberTokenProvider); ok && user != nil {
		rememberer.UpdateRememberToken(user, newRememberToken())
	}
}
