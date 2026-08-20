package middleware

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/genesysflow/go-genesys/http"
	"github.com/gofiber/fiber/v2"
)

// MaintenancePayload is the content of the down file written by
// `genesys down` and read by the Maintenance middleware.
type MaintenancePayload struct {
	// Message is shown to visitors (default "Service Unavailable").
	Message string `json:"message,omitempty"`

	// RetryAfter is the Retry-After header value in seconds.
	RetryAfter int `json:"retry_after,omitempty"`

	// Secret lets holders bypass maintenance mode via ?secret=... once,
	// after which a cookie keeps them in.
	Secret string `json:"secret,omitempty"`
}

// MaintenanceConfig configures the maintenance middleware.
type MaintenanceConfig struct {
	// Path is the down file location (default
	// "storage/framework/down").
	Path string
}

const maintenanceCookie = "genesys_maintenance"

// DefaultDownFilePath is where `genesys down` writes the marker.
const DefaultDownFilePath = "storage/framework/down"

// Maintenance returns 503 for every request while the down file exists,
// Laravel's maintenance mode. Requests carrying the secret (query param
// or cookie) pass through, so operators can inspect the app while it is
// down. Removing the file (genesys up) restores traffic instantly.
func Maintenance(config ...MaintenanceConfig) http.MiddlewareFunc {
	path := DefaultDownFilePath
	if len(config) > 0 && config[0].Path != "" {
		path = config[0].Path
	}

	return func(ctx *http.Context, next func() error) error {
		raw, err := os.ReadFile(path)
		if err != nil {
			return next() // no down file: the app is up
		}

		var payload MaintenancePayload
		json.Unmarshal(raw, &payload)

		// Secret bypass: the query param plants a cookie so subsequent
		// requests keep working.
		if payload.Secret != "" {
			if ctx.Query("secret") == payload.Secret {
				ctx.FiberCtx().Cookie(&fiber.Cookie{
					Name:     maintenanceCookie,
					Value:    payload.Secret,
					Path:     "/",
					HTTPOnly: true,
				})
				return next()
			}
			if ctx.FiberCtx().Cookies(maintenanceCookie) == payload.Secret {
				return next()
			}
		}

		message := payload.Message
		if message == "" {
			message = "Service Unavailable"
		}
		if payload.RetryAfter > 0 {
			ctx.Header("Retry-After", fmt.Sprintf("%d", payload.RetryAfter))
		}
		return ctx.AbortWithJSON(503, map[string]any{"message": message})
	}
}
