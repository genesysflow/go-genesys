package auth

import (
	"strings"
	"time"

	genesysauth "github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/cache"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/http/middleware"
)

// Routes registers the authentication routes. Guests reach the forms;
// logout needs a session.
func (c *Controller) Routes(router *http.Router) {
	guest := genesysauth.GuestMiddleware(c.Guard, c.home())
	limit := c.throttle()

	router.GET("/login", c.ShowLogin, guest).Name("login")
	router.POST("/login", c.Login, guest, limit)

	router.GET("/register", c.ShowRegister, guest).Name("register")
	router.POST("/register", c.Register, guest, limit)

	router.POST("/logout", c.Logout).Name("logout")

	router.GET("/forgot-password", c.ShowForgotPassword, guest).Name("password.request")
	router.POST("/forgot-password", c.SendResetLink, guest, limit)

	router.GET("/reset-password", c.ShowResetPassword, guest).Name("password.reset")
	router.POST("/reset-password", c.ResetPassword, guest, limit)
}

// throttle rate-limits the endpoints that check a credential. An
// unthrottled login is a password guessed at machine speed, so this is
// on by default; the forms themselves are not limited, since reloading
// a page is not an attack.
//
// Set Controller.Limiter to the application's shared cache store so the
// limit holds across restarts and across instances - a per-process
// counter is only as good as one process.
func (c *Controller) throttle() http.MiddlewareFunc {
	store := c.Limiter
	if store == nil {
		store = cache.NewMemoryStore()
	}

	return middleware.Throttle(middleware.ThrottleConfig{
		Store:       store,
		Name:        "auth",
		MaxRequests: 10,
		Window:      time.Minute,

		// Keyed by address as well as IP, so one attacker cannot lock
		// every account out from a single address, and a shared office
		// NAT does not lock out the whole office.
		KeyFor: func(ctx *http.Context) string {
			return ctx.IP() + "|" + strings.ToLower(strings.TrimSpace(ctx.Input("email")))
		},
	})
}
