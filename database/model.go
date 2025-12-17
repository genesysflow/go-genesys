// Package database provides a Laravel-inspired model system.
package database

import (
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/genesysflow/go-genesys/facades/db"
)

// Model is the base struct for Eloquent-style models.
// Embed this in your model structs to get active record functionality.
type Model struct {
	ID        int64      `json:"id" db:"id"`
	CreatedAt *time.Time `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty" db:"updated_at"`

	// Internal fields
	tableName  string
	primaryKey string
	timestamps bool
	exists     bool
}

// ModelInterface defines the interface that all models should implement.
type ModelInterface interface {
	TableName() string
	PrimaryKey() string
	GetID() int64
	SetID(id int64)
	UsesTimestamps() bool
}

// BaseModel provides default implementations for ModelInterface.
type BaseModel struct {
	Model
}

// TableName returns the table name for the model.
// Override this in your model to customize.
func (m *BaseModel) TableName() string {
	if m.tableName != "" {
		return m.tableName
	}
	// Default: snake_case of struct name + 's'
	return ""
}

// PrimaryKey returns the primary key column name.
func (m *BaseModel) PrimaryKey() string {
	if m.primaryKey != "" {
		return m.primaryKey
	}
	return "id"
}

// GetID returns the model's ID.
func (m *BaseModel) GetID() int64 {
	return m.ID
}

// SetID sets the model's ID.
func (m *BaseModel) SetID(id int64) {
	m.ID = id
}

// UsesTimestamps returns whether the model uses timestamps.
func (m *BaseModel) UsesTimestamps() bool {
	return m.timestamps
}

// getTableName returns the table name for the model.
func getTableName[T any]() string {
	var t T
	// Check if T implements ModelInterface
	if m, ok := any(&t).(ModelInterface); ok {
		if name := m.TableName(); name != "" {
			return name
		}
	}
	// Fallback to snake_case of struct name + 's'
	name := reflect.TypeOf(t).Name()
	return strings.ToLower(name) + "s"
}

// All retrieves all records.
func All[T any]() ([]T, error) {
	return AllContext[T](context.Background())
}

// AllContext retrieves all records with context.
func AllContext[T any](ctx context.Context) ([]T, error) {
	tableName := getTableName[T]()
	results, err := db.Table(tableName).WithContext(ctx).Get()
	if err != nil {
		return nil, err
	}
	return mapToStructs[T](results)
}

// Find retrieves a record by ID.
func Find[T any](id int64) (*T, error) {
	return FindContext[T](context.Background(), id)
}

// FindContext retrieves a record by ID with context.
func FindContext[T any](ctx context.Context, id int64) (*T, error) {
	tableName := getTableName[T]()
	result, err := db.Table(tableName).WithContext(ctx).Find(id)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return mapToStruct[T](result)
}

// Create creates a new record from a struct.
func Create[T any](model *T) (int64, error) {
	tableName := getTableName[T]()
	values := structToMap(model)
	now := time.Now()

	// Add timestamps if not present
	if _, ok := values["created_at"]; !ok {
		values["created_at"] = now
	}
	if _, ok := values["updated_at"]; !ok {
		values["updated_at"] = now
	}

	// Remove id if zero
	if id, ok := values["id"].(int64); ok && id == 0 {
		delete(values, "id")
	}

	return db.Table(tableName).InsertGetId(values)
}

// Update updates an existing record.
func Update[T any](id int64, model *T) (int64, error) {
	tableName := getTableName[T]()
	values := structToMap(model)
	values["updated_at"] = time.Now()

	// Remove id from update values
	delete(values, "id")
	delete(values, "created_at")

	return db.Table(tableName).Where("id", "=", id).Update(values)
}

// Delete deletes a record by ID.
func Delete[T any](id int64) (int64, error) {
	tableName := getTableName[T]()
	return db.Table(tableName).Where("id", "=", id).Delete()
}

// structToMap converts a struct to a map for database operations.
func structToMap(s any) map[string]any {
	result := make(map[string]any)
	v := reflect.ValueOf(s)

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return result
	}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Handle anonymous structs (embedded fields)
		if field.Anonymous && value.Kind() == reflect.Struct {
			embedded := structToMap(value.Interface())
			for k, v := range embedded {
				result[k] = v
			}
			continue
		}

		// Get db tag or use lowercase field name
		tag := field.Tag.Get("db")
		if tag == "" {
			tag = strings.ToLower(field.Name)
		}
		if tag == "-" {
			continue
		}

		// Handle pointers
		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				continue
			}
			value = value.Elem()
		}

		result[tag] = value.Interface()
	}

	return result
}

// mapToStruct converts a map to a struct.
func mapToStruct[T any](m map[string]any) (*T, error) {
	var result T
	v := reflect.ValueOf(&result).Elem()
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		if !fieldValue.CanSet() {
			continue
		}

		// Get db tag or use lowercase field name
		tag := field.Tag.Get("db")
		if tag == "" {
			tag = strings.ToLower(field.Name)
		}
		if tag == "-" {
			continue
		}

		if val, ok := m[tag]; ok && val != nil {
			setFieldValue(fieldValue, val)
		}
	}

	return &result, nil
}

// mapToStructs converts a slice of maps to a slice of structs.
func mapToStructs[T any](maps []map[string]any) ([]T, error) {
	result := make([]T, 0, len(maps))
	for _, m := range maps {
		s, err := mapToStruct[T](m)
		if err != nil {
			return nil, err
		}
		result = append(result, *s)
	}
	return result, nil
}

// setFieldValue sets a reflect.Value from an interface{}.
func setFieldValue(field reflect.Value, val any) {
	if val == nil {
		return
	}

	fieldType := field.Type()
	valValue := reflect.ValueOf(val)

	// Handle pointer types
	if fieldType.Kind() == reflect.Ptr {
		if valValue.Kind() == reflect.Ptr {
			if valValue.IsNil() {
				return
			}
			valValue = valValue.Elem()
		}
		ptr := reflect.New(fieldType.Elem())
		setFieldValue(ptr.Elem(), val)
		field.Set(ptr)
		return
	}

	// Handle time.Time
	if fieldType == reflect.TypeOf(time.Time{}) {
		switch v := val.(type) {
		case time.Time:
			field.Set(reflect.ValueOf(v))
		case string:
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				field.Set(reflect.ValueOf(t))
			}
		}
		return
	}

	// Type conversions
	if valValue.Type().ConvertibleTo(fieldType) {
		field.Set(valValue.Convert(fieldType))
		return
	}

	// Handle int64 to int conversion
	if fieldType.Kind() == reflect.Int && valValue.Kind() == reflect.Int64 {
		field.SetInt(valValue.Int())
		return
	}

	// Direct set if types match
	if valValue.Type().AssignableTo(fieldType) {
		field.Set(valValue)
	}
}
