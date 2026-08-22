package database

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/genesysflow/go-genesys/query"
)

// morphRegistry maps the value stored in a morph type column to the Go
// type it names. A morphTo relation cannot be resolved without it: Go
// has no way to turn a string into a struct type at runtime.
var morphRegistry sync.Map // string -> reflect.Type

// RegisterMorph registers a model under its table name, which is what
// the morph* relations store in the type column:
//
//	database.RegisterMorph[models.Post]()
//	database.RegisterMorph[models.Video]()
//
// Every type a morphTo can point at has to be registered, usually once
// at boot in a service provider.
func RegisterMorph[T any]() {
	RegisterMorphAs[T](TableNameFor[T]())
}

// RegisterMorphAs registers a model under an explicit alias, so the
// stored type string can differ from the table name - which is what lets
// a table be renamed without rewriting every stored row.
func RegisterMorphAs[T any](alias string) {
	morphRegistry.Store(alias, reflect.TypeOf((*T)(nil)).Elem())
}

// ClearMorphs forgets every registration, for tests.
func ClearMorphs() {
	morphRegistry.Range(func(key, _ any) bool {
		morphRegistry.Delete(key)
		return true
	})
}

// morphTypeFor resolves a stored morph type value to its Go type.
func morphTypeFor(alias string) (reflect.Type, bool) {
	stored, ok := morphRegistry.Load(alias)
	if !ok {
		return nil, false
	}
	return stored.(reflect.Type), true
}

// loadMorphTo loads the inverse of a polymorphic relation: each parent
// row names its own type, so the models are grouped by that type and
// each group is fetched in one query.
//
// A type nobody registered, or a parent row that no longer exists,
// leaves the relation empty rather than failing the whole load: one
// dangling reference should not take down the page.
func loadMorphTo(driver string, executor query.Executor, models reflect.Value, rel *relation, nested []string) error {
	meta, err := metaFor(models.Type().Elem())
	if err != nil {
		return err
	}

	typeField, ok := findField(meta, rel.morphTypeCol())
	if !ok {
		return fmt.Errorf("database: %s needs a %s column on the model", rel.name, rel.morphTypeCol())
	}
	idField, ok := findField(meta, rel.morphIDCol())
	if !ok {
		return fmt.Errorf("database: %s needs a %s column on the model", rel.name, rel.morphIDCol())
	}

	// Group the parents' keys by the type they name.
	keysByType := make(map[string]map[string]bool)
	for i := 0; i < models.Len(); i++ {
		model := models.Index(i)
		alias := fmt.Sprint(model.FieldByIndex(typeField.index).Interface())
		key := keyString(model.FieldByIndex(idField.index).Interface())
		if alias == "" || key == "" {
			continue
		}
		if keysByType[alias] == nil {
			keysByType[alias] = make(map[string]bool)
		}
		keysByType[alias][key] = true
	}

	// One query per distinct type, holding that type's related rows by key.
	relatedByType := make(map[string]map[string]reflect.Value, len(keysByType))
	for alias, keys := range keysByType {
		relatedType, known := morphTypeFor(alias)
		if !known {
			continue
		}

		relatedMeta, err := metaFor(relatedType)
		if err != nil {
			return err
		}

		values := make([]any, 0, len(keys))
		for key := range keys {
			values = append(values, key)
		}

		rows, err := query.New(driver, executor).
			Table(relatedMeta.table).
			WhereIn(rel.ownerKey, values...).
			Rows()
		if err != nil {
			return err
		}

		loaded, err := scanRowsIntoType(rows, relatedType)
		_ = rows.Close()
		if err != nil {
			return err
		}

		if len(nested) > 0 {
			if err := loadRelationsValue(driver, executor, loaded, nested); err != nil {
				return err
			}
		}

		byKey := make(map[string]reflect.Value, loaded.Len())
		for i := 0; i < loaded.Len(); i++ {
			element := loaded.Index(i)
			elementMeta, err := metaFor(relatedType)
			if err != nil {
				return err
			}
			ownerField, ok := findField(elementMeta, rel.ownerKey)
			if !ok {
				continue
			}
			byKey[keyString(element.FieldByIndex(ownerField.index).Interface())] = element
		}
		relatedByType[alias] = byKey
	}

	// Assign each parent its own related model, as a pointer in the
	// interface field.
	for i := 0; i < models.Len(); i++ {
		model := models.Index(i)
		alias := fmt.Sprint(model.FieldByIndex(typeField.index).Interface())
		key := keyString(model.FieldByIndex(idField.index).Interface())

		related, ok := relatedByType[alias][key]
		if !ok {
			continue
		}

		pointer := reflect.New(related.Type())
		pointer.Elem().Set(related)
		model.FieldByIndex(rel.fieldIndex).Set(pointer)
	}

	return nil
}
