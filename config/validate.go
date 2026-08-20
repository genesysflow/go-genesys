package config

import (
	"fmt"
	"strings"

	"github.com/genesysflow/go-genesys/contracts"
)

// Validate fails fast on missing configuration: it checks that every
// required key resolves to a non-empty value and reports all missing
// keys at once, so a misconfigured deployment dies at boot instead of
// at first use:
//
//	if err := config.Validate(cfg,
//	    "app.key", "database.default", "mail.from_address",
//	); err != nil {
//	    log.Fatal(err)
//	}
func Validate(cfg contracts.Config, required ...string) error {
	var missing []string
	for _, key := range required {
		value := cfg.Get(key)
		if value == nil {
			missing = append(missing, key)
			continue
		}
		if s, ok := value.(string); ok && strings.TrimSpace(s) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("config: missing required keys: %s", strings.Join(missing, ", "))
	}
	return nil
}

// MustValidate is Validate but panics, for use in bootstrap code.
func MustValidate(cfg contracts.Config, required ...string) {
	if err := Validate(cfg, required...); err != nil {
		panic(err)
	}
}
