package database

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/genesysflow/go-genesys/crypt"
)

// Attribute casting, Laravel's $casts: a db tag may carry a cast option
// after the column name.
//
//	type User struct {
//	    database.Model
//	    Settings map[string]any `db:"settings,json"`   // JSON column
//	    Prefs    Preferences    `db:"prefs,json"`      // any marshalable type
//	    SSN      string         `db:"ssn,encrypted"`   // encrypted at rest
//	}
//
// json-cast fields marshal to a JSON string on write and unmarshal on
// scan. encrypted-cast fields (string only) are encrypted with the
// application key on write and decrypted on scan; the struct always
// holds plaintext. Encrypted casts need database.SetEncrypter, which
// the CryptServiceProvider wires automatically.
const (
	castJSON      = "json"
	castEncrypted = "encrypted"
)

var (
	encrypterMu sync.RWMutex
	encrypter   *crypt.Encrypter
)

// SetEncrypter installs the encrypter used by encrypted-cast fields.
func SetEncrypter(e *crypt.Encrypter) {
	encrypterMu.Lock()
	defer encrypterMu.Unlock()
	encrypter = e
}

func currentEncrypter() (*crypt.Encrypter, error) {
	encrypterMu.RLock()
	defer encrypterMu.RUnlock()
	if encrypter == nil {
		return nil, fmt.Errorf("database: no encrypter configured for encrypted casts - register the CryptServiceProvider or call database.SetEncrypter")
	}
	return encrypter, nil
}

// marshalJSONCast serializes a json-cast field value; a marshal failure
// is a misdeclared model (unsupported field type), reported loudly.
func marshalJSONCast(owner reflect.Type, f fieldMeta, value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("database: %s column %q cannot marshal to JSON: %v", owner.Name(), f.column, err))
	}
	return string(encoded)
}

// encryptWriteValues replaces the plaintext of encrypted-cast columns
// with fresh ciphertext, in place. Call it on any value map headed for
// an INSERT or UPDATE.
func (m *modelMeta) encryptWriteValues(values map[string]any) error {
	for _, f := range m.fields {
		if f.cast != castEncrypted {
			continue
		}
		plain, ok := values[f.column]
		if !ok {
			continue
		}
		text, ok := plain.(string)
		if !ok {
			continue
		}
		if text == "" {
			continue // empty stays empty, so zero values stay readable
		}
		enc, err := currentEncrypter()
		if err != nil {
			return err
		}
		ciphertext, err := enc.EncryptString(text)
		if err != nil {
			return fmt.Errorf("database: encrypting column %q: %w", f.column, err)
		}
		values[f.column] = ciphertext
	}
	return nil
}

// assignCastValue scans a driver value into a cast field.
func assignCastValue(field reflect.Value, value any, cast string) error {
	if value == nil {
		field.Set(reflect.Zero(field.Type()))
		return nil
	}
	text := ""
	switch v := value.(type) {
	case string:
		text = v
	case []byte:
		text = string(v)
	default:
		return fmt.Errorf("cast %q needs a text column, driver returned %T", cast, value)
	}
	if text == "" {
		field.Set(reflect.Zero(field.Type()))
		return nil
	}

	switch cast {
	case castJSON:
		if err := json.Unmarshal([]byte(text), field.Addr().Interface()); err != nil {
			return fmt.Errorf("cannot unmarshal JSON column: %w", err)
		}
		return nil
	case castEncrypted:
		enc, err := currentEncrypter()
		if err != nil {
			return err
		}
		plain, err := enc.DecryptString(text)
		if err != nil {
			return fmt.Errorf("cannot decrypt column: %w", err)
		}
		field.SetString(plain)
		return nil
	}
	return fmt.Errorf("unknown cast %q", cast)
}
