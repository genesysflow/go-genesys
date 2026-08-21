package database

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/genesysflow/go-genesys/support"
	"github.com/jinzhu/inflection"
)

// Model is the base struct to embed in database models:
//
//	type User struct {
//	    database.Model
//	    Name  string `db:"name" json:"name"`
//	    Email string `db:"email" json:"email"`
//	}
//
// It provides the primary key and timestamp columns managed by the ORM.
type Model struct {
	ID        int64     `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	// original holds the column values as last read from or written to
	// the database, powering IsDirty/GetDirty and partial updates.
	original map[string]any
}

// originalHolder is implemented by *Model and promoted to every model
// that embeds it, giving the ORM a place to stash the pristine copy.
type originalHolder interface {
	getOriginal() map[string]any
	setOriginal(map[string]any)
}

func (m *Model) getOriginal() map[string]any       { return m.original }
func (m *Model) setOriginal(values map[string]any) { m.original = values }

// SoftDeletes adds a deleted_at column to a model, switching the ORM to
// soft deletion: Delete marks the row instead of removing it, and queries
// exclude trashed rows unless WithTrashed/OnlyTrashed is used.
//
//	type User struct {
//	    database.Model
//	    database.SoftDeletes
//	    Name string `db:"name"`
//	}
type SoftDeletes struct {
	DeletedAt *time.Time `db:"deleted_at" json:"deleted_at,omitempty"`
}

// TableNamer lets a model override its inferred table name.
type TableNamer interface {
	TableName() string
}

// ConnectionNamer lets a model choose a non-default connection.
type ConnectionNamer interface {
	ConnectionName() string
}

type fieldMeta struct {
	column    string
	index     []int
	cast      string // "", "json", or "encrypted" (see casts.go)
	isPK      bool
	isCreated bool
	isUpdated bool
	isDeleted bool
}

type modelMeta struct {
	table       string
	fields      []fieldMeta
	byCol       map[string]*fieldMeta
	pkIndex     int // index into fields, -1 when absent
	softDeletes bool
}

var metaCache sync.Map // reflect.Type -> *modelMeta

// metaFor builds (and caches) column metadata for a model type.
func metaFor(t reflect.Type) (*modelMeta, error) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("database: model must be a struct, got %s", t.Kind())
	}

	if cached, ok := metaCache.Load(t); ok {
		return cached.(*modelMeta), nil
	}

	meta := &modelMeta{
		table:   inferTableName(t),
		byCol:   make(map[string]*fieldMeta),
		pkIndex: -1,
	}
	collectFields(t, nil, meta)

	for i := range meta.fields {
		f := &meta.fields[i]
		meta.byCol[f.column] = f
		if f.isPK {
			meta.pkIndex = i
		}
		if f.isDeleted {
			meta.softDeletes = true
		}
	}

	metaCache.Store(t, meta)
	return meta, nil
}

// collectFields walks the struct type, flattening embedded structs.
func collectFields(t reflect.Type, parentIndex []int, meta *modelMeta) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue // unexported
		}
		index := append(append([]int(nil), parentIndex...), i)

		if field.Anonymous && field.Type.Kind() == reflect.Struct && field.Tag.Get("db") == "" {
			collectFields(field.Type, index, meta)
			continue
		}

		tag := field.Tag.Get("db")
		column, cast := tag, ""
		if idx := strings.Index(tag, ","); idx >= 0 {
			column, cast = tag[:idx], strings.TrimSpace(tag[idx+1:])
		}
		if column == "-" {
			continue
		}
		if field.Tag.Get("rel") != "" {
			continue // relation fields are loaded separately, never columns
		}
		if column == "" {
			column = support.ToSnakeCase(field.Name)
		}
		switch cast {
		case "", castJSON:
		case castEncrypted:
			if field.Type.Kind() != reflect.String {
				panic(fmt.Sprintf("database: %s.%s: the encrypted cast needs a string field", t.Name(), field.Name))
			}
		default:
			panic(fmt.Sprintf("database: %s.%s: unknown cast %q (want json or encrypted)", t.Name(), field.Name, cast))
		}

		// Shadowed columns: the shallower field wins (an outer field
		// overrides an embedded one, like encoding/json). Embedded
		// structs are walked first, so on conflict the outer field must
		// replace the recorded index, not be skipped.
		if existing, exists := findField(meta, column); exists {
			if len(index) < len(existing.index) {
				existing.index = index
			}
			continue
		}

		meta.fields = append(meta.fields, fieldMeta{
			column:    column,
			index:     index,
			cast:      cast,
			isPK:      column == "id",
			isCreated: column == "created_at",
			isUpdated: column == "updated_at",
			isDeleted: column == "deleted_at",
		})
	}
}

func findField(meta *modelMeta, column string) (*fieldMeta, bool) {
	for i := range meta.fields {
		if meta.fields[i].column == column {
			return &meta.fields[i], true
		}
	}
	return nil, false
}

// inferTableName derives the table name from the struct name: User -> users,
// BlogPost -> blog_posts. Models can override via TableName().
func inferTableName(t reflect.Type) string {
	if namer, ok := reflect.New(t).Interface().(TableNamer); ok {
		return namer.TableName()
	}
	return inflection.Plural(support.ToSnakeCase(t.Name()))
}

// TableNameFor returns the table a model type maps to.
func TableNameFor[T any]() string {
	meta, err := metaFor(reflect.TypeOf((*T)(nil)).Elem())
	if err != nil {
		panic(err)
	}
	return meta.table
}

// values extracts column -> value pairs from a model, optionally skipping
// the primary key (for inserts with auto-increment ids).
func (m *modelMeta) values(v reflect.Value, skipPK bool) map[string]any {
	for v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	out := make(map[string]any, len(m.fields))
	for _, f := range m.fields {
		if skipPK && f.isPK {
			continue
		}
		value := v.FieldByIndex(f.index).Interface()
		if f.cast == castJSON {
			// Deterministic serialized form: dirty comparison and the
			// written column value are the same string.
			value = marshalJSONCast(v.Type(), f, value)
		}
		// Encrypted fields stay plaintext here (ciphertexts differ on
		// every encryption, which would break dirty tracking); write
		// paths encrypt via encryptWriteValues.
		out[f.column] = value
	}
	return out
}

// scanRowsInto scans all rows into a slice of T using column metadata.
func scanRowsInto[T any](rows *sql.Rows) ([]T, error) {
	slice, err := scanRowsIntoType(rows, reflect.TypeOf((*T)(nil)).Elem())
	if err != nil {
		return nil, err
	}
	return slice.Interface().([]T), nil
}

// scanRowsIntoType is the reflection core of scanRowsInto: it scans all
// rows into an addressable slice of the given struct type, letting the
// relation loader work with types only known at runtime.
func scanRowsIntoType(rows *sql.Rows, structType reflect.Type) (reflect.Value, error) {
	defer rows.Close()

	results := reflect.New(reflect.SliceOf(structType)).Elem()

	meta, err := metaFor(structType)
	if err != nil {
		return results, err
	}

	columns, err := rows.Columns()
	if err != nil {
		return results, err
	}

	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return results, err
		}

		item := reflect.New(structType).Elem()
		for i, column := range columns {
			field, ok := meta.byCol[column]
			if !ok {
				continue
			}
			if field.cast != "" {
				if err := assignCastValue(item.FieldByIndex(field.index), values[i], field.cast); err != nil {
					return results, fmt.Errorf("database: column %q: %w", column, err)
				}
				continue
			}
			if err := assignValue(item.FieldByIndex(field.index), values[i]); err != nil {
				return results, fmt.Errorf("database: column %q: %w", column, err)
			}
		}
		if holder, ok := item.Addr().Interface().(originalHolder); ok {
			holder.setOriginal(meta.values(item, false))
		}
		results.Set(reflect.Append(results, item))
	}
	return results, rows.Err()
}

// timeFormats are the layouts tried when a driver returns timestamps as text.
var timeFormats = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// assignValue assigns a driver value to a struct field, converting between
// the loosely-typed values drivers return and the field's static type.
func assignValue(field reflect.Value, value any) error {
	if value == nil {
		field.Set(reflect.Zero(field.Type()))
		return nil
	}
	if b, ok := value.([]byte); ok {
		value = string(b)
	}

	// time.Time fields
	if field.Type() == reflect.TypeOf(time.Time{}) {
		switch v := value.(type) {
		case time.Time:
			field.Set(reflect.ValueOf(v))
			return nil
		case string:
			for _, layout := range timeFormats {
				if parsed, err := time.Parse(layout, v); err == nil {
					field.Set(reflect.ValueOf(parsed))
					return nil
				}
			}
			return fmt.Errorf("cannot parse time %q", v)
		case int64:
			field.Set(reflect.ValueOf(time.Unix(v, 0)))
			return nil
		}
	}

	// sql.Null* and other Scanner fields
	if scanner, ok := field.Addr().Interface().(sql.Scanner); ok {
		return scanner.Scan(value)
	}

	// Pointer fields: allocate and assign to the element.
	if field.Kind() == reflect.Pointer {
		elem := reflect.New(field.Type().Elem())
		if err := assignValue(elem.Elem(), value); err != nil {
			return err
		}
		field.Set(elem)
		return nil
	}

	rv := reflect.ValueOf(value)
	if rv.Type().AssignableTo(field.Type()) {
		field.Set(rv)
		return nil
	}
	if rv.Type().ConvertibleTo(field.Type()) {
		field.Set(rv.Convert(field.Type()))
		return nil
	}

	// Common driver mismatches
	switch field.Kind() {
	case reflect.Bool:
		switch v := value.(type) {
		case int64:
			field.SetBool(v != 0)
			return nil
		case string:
			field.SetBool(v == "1" || strings.EqualFold(v, "true"))
			return nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v, ok := value.(string); ok {
			var n int64
			if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
				field.SetInt(n)
				return nil
			}
		}
	case reflect.Float32, reflect.Float64:
		if v, ok := value.(string); ok {
			var n float64
			if _, err := fmt.Sscanf(v, "%f", &n); err == nil {
				field.SetFloat(n)
				return nil
			}
		}
	case reflect.String:
		field.SetString(fmt.Sprint(value))
		return nil
	}

	return fmt.Errorf("cannot assign %T to %s", value, field.Type())
}
