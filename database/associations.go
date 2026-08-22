package database

import (
	"fmt"
	"reflect"

	"github.com/genesysflow/go-genesys/query"
)

// Relationship write operations - Laravel's attach/detach/sync/toggle
// for pivot tables, plus create-through-relation helpers.

// relationOn resolves a named relation of the parent's type.
func relationOn(parentType reflect.Type, name string) (*relation, error) {
	relations, err := relationsFor(parentType)
	if err != nil {
		return nil, err
	}
	rel, ok := relations[name]
	if !ok {
		return nil, fmt.Errorf("database: %s has no relation %q (declare it with a rel tag)", parentType.Name(), name)
	}
	return rel, nil
}

// ownerKeyValue reads the parent's owner-key column (usually id).
func ownerKeyValue[T any](parent *T, rel *relation) (any, error) {
	meta, err := metaFor(reflect.TypeOf(parent).Elem())
	if err != nil {
		return nil, err
	}
	field, ok := meta.byCol[rel.ownerKey]
	if !ok {
		return nil, fmt.Errorf("database: owner key %q is not a column of %s", rel.ownerKey, meta.table)
	}
	value := reflect.ValueOf(parent).Elem().FieldByIndex(field.index).Interface()
	if keyString(value) == "" || keyString(value) == "0" {
		return nil, fmt.Errorf("database: cannot modify relations of an unsaved model (empty %s)", rel.ownerKey)
	}
	return value, nil
}

// relatedID extracts a primary key from a raw id or a related model.
func relatedID(value any) any {
	v := reflect.ValueOf(value)
	for v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return value
	}
	meta, err := metaFor(v.Type())
	if err != nil || meta.pkIndex < 0 {
		return value
	}
	return v.FieldByIndex(meta.fields[meta.pkIndex].index).Interface()
}

// pivotQuery starts a query against the relation's pivot table, scoped
// to this parent. A polymorphic pivot is shared by every model that uses
// it, so the type column is part of "this parent" - without it a write
// or a delete would reach into another model's links.
func pivotQuery(driver string, executor query.Executor, rel *relation, parentKey any, parentTable string) *query.Builder {
	builder := query.New(driver, executor).Table(rel.pivotTable).Where(rel.pivotFK, parentKey)
	if rel.kind == relMorphToMany {
		builder = builder.Where(rel.morphTypeCol(), parentTable)
	}
	return builder
}

// pivotRow renders the columns identifying one link.
func pivotRow(rel *relation, parentKey, relatedID any, parentTable string) map[string]any {
	row := map[string]any{
		rel.pivotFK: parentKey,
		rel.pivotRK: relatedID,
	}
	if rel.kind == relMorphToMany {
		row[rel.morphTypeCol()] = parentTable
	}
	return row
}

// pivotState loads the currently attached related keys.
func pivotState(driver string, executor query.Executor, rel *relation, parentKey any, parentTable string) (map[string]any, error) {
	rows, err := pivotQuery(driver, executor, rel, parentKey, parentTable).Get()
	if err != nil {
		return nil, err
	}
	attached := make(map[string]any, len(rows))
	for _, row := range rows {
		attached[keyString(row[rel.pivotRK])] = row[rel.pivotRK]
	}
	return attached, nil
}

func belongsToManyOn[T any](parent *T, relationName string, scope []*TxScope) (*relation, any, string, string, query.Executor, error) {
	rel, err := relationOn(reflect.TypeOf(parent).Elem(), relationName)
	if err != nil {
		return nil, nil, "", "", nil, err
	}
	// morphToMany is the same pivot with a type column, so it is written
	// through the same helpers.
	if rel.kind != relBelongsToMany && rel.kind != relMorphToMany {
		return nil, nil, "", "", nil, fmt.Errorf("database: relation %q is not belongsToMany or morphToMany", relationName)
	}
	parentKey, err := ownerKeyValue(parent, rel)
	if err != nil {
		return nil, nil, "", "", nil, err
	}

	// A polymorphic pivot stores what kind of parent each link belongs
	// to; that is the parent's table name, the same value the reads
	// filter on.
	meta, err := metaFor(reflect.TypeOf(parent).Elem())
	if err != nil {
		return nil, nil, "", "", nil, err
	}

	driver, executor := executorFor[T](scope)
	return rel, parentKey, meta.table, driver, executor, nil
}

// Attach links related models (or raw ids) through the pivot table,
// skipping links that already exist:
//
//	database.Attach(post, "Tags", tagGo, tagWeb)
//	database.Attach(post, "Tags", 3, 4)
func Attach[T any](parent *T, relationName string, related ...any) error {
	return AttachScoped(nil, parent, relationName, related...)
}

// AttachScoped is Attach inside a transaction scope (nil scope uses the
// default connection).
func AttachScoped[T any](tx *TxScope, parent *T, relationName string, related ...any) error {
	rel, parentKey, parentTable, driver, executor, err := belongsToManyOn(parent, relationName, scopeSlice(tx))
	if err != nil {
		return err
	}
	attached, err := pivotState(driver, executor, rel, parentKey, parentTable)
	if err != nil {
		return err
	}
	for _, item := range related {
		id := relatedID(item)
		if _, exists := attached[keyString(id)]; exists {
			continue
		}
		if err := query.New(driver, executor).Table(rel.pivotTable).
			Insert(pivotRow(rel, parentKey, id, parentTable)); err != nil {
			return err
		}
		attached[keyString(id)] = id
	}
	return nil
}

// Detach unlinks the given related models (or every link when none are
// given) and returns how many pivot rows were removed.
func Detach[T any](parent *T, relationName string, related ...any) (int64, error) {
	return DetachScoped(nil, parent, relationName, related...)
}

// DetachScoped is Detach inside a transaction scope.
func DetachScoped[T any](tx *TxScope, parent *T, relationName string, related ...any) (int64, error) {
	rel, parentKey, parentTable, driver, executor, err := belongsToManyOn(parent, relationName, scopeSlice(tx))
	if err != nil {
		return 0, err
	}
	builder := pivotQuery(driver, executor, rel, parentKey, parentTable)
	if len(related) > 0 {
		ids := make([]any, len(related))
		for i, item := range related {
			ids[i] = relatedID(item)
		}
		builder.WhereIn(rel.pivotRK, ids...)
	}
	return builder.Delete()
}

// Sync makes the pivot table contain exactly the given related models:
// missing links are attached, surplus links detached.
func Sync[T any](parent *T, relationName string, related ...any) error {
	return SyncScoped(nil, parent, relationName, related...)
}

// SyncScoped is Sync inside a transaction scope.
func SyncScoped[T any](tx *TxScope, parent *T, relationName string, related ...any) error {
	rel, parentKey, parentTable, driver, executor, err := belongsToManyOn(parent, relationName, scopeSlice(tx))
	if err != nil {
		return err
	}
	attached, err := pivotState(driver, executor, rel, parentKey, parentTable)
	if err != nil {
		return err
	}

	wanted := make(map[string]any, len(related))
	for _, item := range related {
		id := relatedID(item)
		wanted[keyString(id)] = id
	}

	for key, id := range wanted {
		if _, exists := attached[key]; exists {
			continue
		}
		if err := query.New(driver, executor).Table(rel.pivotTable).
			Insert(pivotRow(rel, parentKey, id, parentTable)); err != nil {
			return err
		}
	}
	for key, id := range attached {
		if _, keep := wanted[key]; keep {
			continue
		}
		if _, err := pivotQuery(driver, executor, rel, parentKey, parentTable).
			Where(rel.pivotRK, id).Delete(); err != nil {
			return err
		}
	}
	return nil
}

// Toggle attaches the given related models that are missing and
// detaches the ones already linked.
func Toggle[T any](parent *T, relationName string, related ...any) error {
	return ToggleScoped(nil, parent, relationName, related...)
}

// ToggleScoped is Toggle inside a transaction scope.
func ToggleScoped[T any](tx *TxScope, parent *T, relationName string, related ...any) error {
	rel, parentKey, parentTable, driver, executor, err := belongsToManyOn(parent, relationName, scopeSlice(tx))
	if err != nil {
		return err
	}
	attached, err := pivotState(driver, executor, rel, parentKey, parentTable)
	if err != nil {
		return err
	}
	for _, item := range related {
		id := relatedID(item)
		if _, exists := attached[keyString(id)]; exists {
			if _, err := pivotQuery(driver, executor, rel, parentKey, parentTable).
				Where(rel.pivotRK, id).Delete(); err != nil {
				return err
			}
			continue
		}
		if err := query.New(driver, executor).Table(rel.pivotTable).
			Insert(pivotRow(rel, parentKey, id, parentTable)); err != nil {
			return err
		}
	}
	return nil
}

// CreateFor creates a child through a hasOne/hasMany relation, setting
// its foreign key from the parent before inserting:
//
//	database.CreateFor(author, "Posts", &Article{Title: "New"})
func CreateFor[T, R any](parent *T, relationName string, child *R, scope ...*TxScope) error {
	rel, err := relationOn(reflect.TypeOf(parent).Elem(), relationName)
	if err != nil {
		return err
	}
	if rel.kind != relHasOne && rel.kind != relHasMany {
		return fmt.Errorf("database: CreateFor needs a hasOne/hasMany relation, %q is not", relationName)
	}
	childType := reflect.TypeOf(child).Elem()
	if rel.related != childType {
		return fmt.Errorf("database: relation %q holds %s, not %s", relationName, rel.related.Name(), childType.Name())
	}
	parentKey, err := ownerKeyValue(parent, rel)
	if err != nil {
		return err
	}

	childMeta, err := metaFor(childType)
	if err != nil {
		return err
	}
	fkField, ok := childMeta.byCol[rel.foreignKey]
	if !ok {
		return fmt.Errorf("database: foreign key %q is not a column of %s", rel.foreignKey, childMeta.table)
	}
	if err := assignValue(reflect.ValueOf(child).Elem().FieldByIndex(fkField.index), parentKey); err != nil {
		return err
	}
	return Create(child, scope...)
}

// Associate points a belongsTo child at a parent by filling the child's
// foreign key (and its relation field). The child is not saved:
//
//	database.Associate(article, "Author", author)
//	database.Update(article)
func Associate[T, R any](child *T, relationName string, parent *R) error {
	rel, err := relationOn(reflect.TypeOf(child).Elem(), relationName)
	if err != nil {
		return err
	}
	if rel.kind != relBelongsTo {
		return fmt.Errorf("database: Associate needs a belongsTo relation, %q is not", relationName)
	}
	parentType := reflect.TypeOf(parent).Elem()
	if rel.related != parentType {
		return fmt.Errorf("database: relation %q holds %s, not %s", relationName, rel.related.Name(), parentType.Name())
	}
	parentKey, err := ownerKeyValue(parent, rel)
	if err != nil {
		return err
	}

	childMeta, err := metaFor(reflect.TypeOf(child).Elem())
	if err != nil {
		return err
	}
	fkField, ok := childMeta.byCol[rel.foreignKey]
	if !ok {
		return fmt.Errorf("database: foreign key %q is not a column of %s", rel.foreignKey, childMeta.table)
	}
	childValue := reflect.ValueOf(child).Elem()
	if err := assignValue(childValue.FieldByIndex(fkField.index), parentKey); err != nil {
		return err
	}

	// Mirror onto the relation field so the in-memory model is coherent.
	relField := childValue.FieldByIndex(rel.fieldIndex)
	if relField.Kind() == reflect.Pointer {
		ptr := reflect.New(parentType)
		ptr.Elem().Set(reflect.ValueOf(parent).Elem())
		relField.Set(ptr)
	}
	return nil
}

// Dissociate clears a belongsTo relation's foreign key and field. The
// child is not saved.
func Dissociate[T any](child *T, relationName string) error {
	rel, err := relationOn(reflect.TypeOf(child).Elem(), relationName)
	if err != nil {
		return err
	}
	if rel.kind != relBelongsTo {
		return fmt.Errorf("database: Dissociate needs a belongsTo relation, %q is not", relationName)
	}
	childMeta, err := metaFor(reflect.TypeOf(child).Elem())
	if err != nil {
		return err
	}
	fkField, ok := childMeta.byCol[rel.foreignKey]
	if !ok {
		return fmt.Errorf("database: foreign key %q is not a column of %s", rel.foreignKey, childMeta.table)
	}
	childValue := reflect.ValueOf(child).Elem()
	childValue.FieldByIndex(fkField.index).Set(reflect.Zero(childValue.FieldByIndex(fkField.index).Type()))
	childValue.FieldByIndex(rel.fieldIndex).Set(reflect.Zero(childValue.FieldByIndex(rel.fieldIndex).Type()))
	return nil
}

// scopeSlice adapts a nilable scope to the variadic form the internal
// helpers use.
func scopeSlice(tx *TxScope) []*TxScope {
	if tx == nil {
		return nil
	}
	return []*TxScope{tx}
}
