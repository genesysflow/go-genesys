package session

import (
	"bytes"
	"context"
	"encoding/gob"

	"github.com/gofiber/fiber/v3"
)

// legacyPayloadStorage reads sessions written by Fiber v2's session store.
//
// Fiber v2 encoded a session as a gob map[string]any; v3 encodes and
// expects a gob map[any]any, and gob refuses to decode one into the other.
// A session saved before the upgrade would otherwise fail to load, and the
// session middleware would answer every request carrying its cookie with an
// error. Get re-encodes such a payload in the v3 form; the next Save writes
// it back in that form for good.
type legacyPayloadStorage struct {
	fiber.Storage
}

// Get retrieves a session payload, converting a v2 payload to the v3 form.
func (s *legacyPayloadStorage) Get(key string) ([]byte, error) {
	raw, err := s.Storage.Get(key)
	if err != nil {
		return nil, err
	}
	return upgradeLegacyPayload(raw), nil
}

// GetWithContext is Get bounded by ctx.
func (s *legacyPayloadStorage) GetWithContext(ctx context.Context, key string) ([]byte, error) {
	raw, err := s.Storage.GetWithContext(ctx, key)
	if err != nil {
		return nil, err
	}
	return upgradeLegacyPayload(raw), nil
}

// upgradeLegacyPayload returns raw in the v3 encoding when it holds a v2
// payload, and raw itself otherwise. gob checks the wire type before it
// decodes any values, so a v3 payload is told apart without decoding it.
func upgradeLegacyPayload(raw []byte) []byte {
	if len(raw) == 0 {
		return raw
	}
	var legacy map[string]any
	if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&legacy); err != nil {
		return raw
	}
	current := make(map[any]any, len(legacy))
	for key, value := range legacy {
		current[key] = value
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(&current); err != nil {
		return raw
	}
	return buf.Bytes()
}
