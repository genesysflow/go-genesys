package providers

import (
	"os"
	"path/filepath"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/log"
)

// LogServiceProvider registers logging services.
type LogServiceProvider struct {
	BaseProvider
}

// Register registers the logging services.
func (p *LogServiceProvider) Register(app contracts.Application) error {
	p.app = app

	cfg := app.GetConfig()

	// Get log configuration
	channel := cfg.GetString("logging.default")
	if channel == "" {
		channel = "console"
	}

	var logger *log.Logger

	switch channel {
	case "file":
		logPath := cfg.GetString("logging.channels.file.path")
		if logPath == "" {
			logPath = filepath.Join(app.StoragePath(), "logs", "app.log")
		}

		// Ensure directory exists
		dir := filepath.Dir(logPath)
		os.MkdirAll(dir, 0755)

		var err error
		logger, err = log.NewFile(logPath)
		if err != nil {
			// Fallback to console
			logger = log.New()
		}

	case "json":
		logger = log.NewJSON()

	default:
		logger = log.New()
	}

	// Set log level from config
	level := cfg.GetString("logging.level")
	if level != "" {
		logger.SetLevel(parseLogLevel(level))
	}

	app.BindValue("logger", logger)
	app.BindValue("log.manager", log.NewManager())

	return nil
}

// Boot bootstraps the logging services.
func (p *LogServiceProvider) Boot(app contracts.Application) error {
	return nil
}

// Provides returns the services this provider registers.
func (p *LogServiceProvider) Provides() []string {
	return []string{
		"logger",
		"log.manager",
	}
}

// parseLogLevel parses a string log level.
func parseLogLevel(level string) contracts.LogLevel {
	switch level {
	case "debug":
		return contracts.LogLevelDebug
	case "info":
		return contracts.LogLevelInfo
	case "warn", "warning":
		return contracts.LogLevelWarn
	case "error":
		return contracts.LogLevelError
	case "fatal":
		return contracts.LogLevelFatal
	case "panic":
		return contracts.LogLevelPanic
	default:
		return contracts.LogLevelInfo
	}
}
