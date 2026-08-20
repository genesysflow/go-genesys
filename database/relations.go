package database

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/genesysflow/go-genesys/query"
	"github.com/genesysflow/go-genesys/support"
)

// Relationships are declared with a `rel` struct tag on a field that holds
// the related model(s). The tag names the relation kind, with optional
// comma-separated options; keys follow Laravel's conventions when omitted.
//
//	type User struct {
//	    database.Model
//	    Name  string  `db:"name"`
//	    Posts []*Post `rel:"hasMany"`                // posts.user_id
//	    Phone *Phone  `rel:"hasOne"`                 // phones.user_id
//	}
//
//	type Post struct {
//	    database.Model
//	    UserID int64  `db:"user_id"`
//	    Author *User  `rel:"belongsTo,fk:user_id"`   // default fk: author_id
//	    Tags   []*Tag `rel:"belongsToMany"`          // pivot post_tag
//	}
//
// Options: fk (foreign key), ok (owner/local key, default id),
// pivot (pivot table), pfk (pivot column referencing this model),
// prk (pivot column referencing the related model).
type relKind int

const (
	relHasOne relKind = iota
	relHasMany
	relBelongsTo
	relBelongsToMany
)

type relation struct {
	name       string
	kind       relKind
	fieldIndex []int
	fieldType  reflect.Type // declared type: []*Post, []Post, *User
	related    reflect.Type // related struct type: Post, User
	foreignKey string
	ownerKey   string
	pivotTable string
	pivotFK    string // pivot column referencing the parent
	pivotRK    string // pivot column referencing the related model
}

var relationCache sync.Map // reflect.Type -> map[string]*relation

// relationsFor parses (and caches) the relations declared on a model type.
func relationsFor(t reflect.Type) (map[string]*relation, error) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if cached, ok := relationCache.Load(t); ok {
		return cached.(map[string]*relation), nil
	}

	relations := make(map[string]*relation)
	if err := collectRelations(t, nil, relations); err != nil {
		return nil, err
	}
	relationCache.Store(t, relations)
	return relations, nil
}

func collectRelations(t reflect.Type, parentIndex []int, out map[string]*relation) error {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		index := append(append([]int(nil), parentIndex...), i)

		if field.Anonymous && field.Type.Kind() == reflect.Struct && field.Tag.Get("rel") == "" {
			if err := collectRelations(field.Type, index, out); err != nil {
				return err
			}
			continue
		}

		tag := field.Tag.Get("rel")
		if tag == "" {
			continue
		}

		rel, err := parseRelation(t, field, index, tag)
		if err != nil {
			return err
		}
		out[field.Name] = rel
	}
	return nil
}

func parseRelation(parent reflect.Type, field reflect.StructField, index []int, tag string) (*relation, error) {
	parts := strings.Split(tag, ",")

	rel := &relation{
		name:       field.Name,
		fieldIndex: index,
		fieldType:  field.Type,
		ownerKey:   "id",
	}

	switch parts[0] {
	case "hasOne":
		rel.kind = relHasOne
	case "hasMany":
		rel.kind = relHasMany
	case "belongsTo":
		rel.kind = relBelongsTo
	case "belongsToMany":
		rel.kind = relBelongsToMany
	default:
		return nil, fmt.Errorf("database: field %s.%s: unknown relation kind %q", parent.Name(), field.Name, parts[0])
	}

	related, err := relatedStructType(field.Type)
	if err != nil {
		return nil, fmt.Errorf("database: field %s.%s: %w", parent.Name(), field.Name, err)
	}
	rel.related = related

	if (rel.kind == relHasMany || rel.kind == relBelongsToMany) && field.Type.Kind() != reflect.Slice {
		return nil, fmt.Errorf("database: field %s.%s: %s relations need a slice field", parent.Name(), field.Name, parts[0])
	}
	if (rel.kind == relHasOne || rel.kind == relBelongsTo) && field.Type.Kind() == reflect.Slice {
		return nil, fmt.Errorf("database: field %s.%s: %s relations need a single-value field", parent.Name(), field.Name, parts[0])
	}

	// Convention defaults.
	parentKey := support.ToSnakeCase(parent.Name()) + "_id"
	relatedKey := support.ToSnakeCase(related.Name()) + "_id"
	switch rel.kind {
	case relHasOne, relHasMany:
		rel.foreignKey = parentKey // on the related table
	case relBelongsTo:
		rel.foreignKey = support.ToSnakeCase(field.Name) + "_id" // on the parent table
	case relBelongsToMany:
		names := []string{support.ToSnakeCase(parent.Name()), support.ToSnakeCase(related.Name())}
		sort.Strings(names)
		rel.pivotTable = names[0] + "_" + names[1]
		rel.pivotFK = parentKey
		rel.pivotRK = relatedKey
	}

	for _, opt := range parts[1:] {
		key, value, found := strings.Cut(strings.TrimSpace(opt), ":")
		if !found || value == "" {
			return nil, fmt.Errorf("database: field %s.%s: malformed rel option %q", parent.Name(), field.Name, opt)
		}
		switch key {
		case "fk":
			rel.foreignKey = value
		case "ok":
			rel.ownerKey = value
		case "pivot":
			rel.pivotTable = value
		case "pfk":
			rel.pivotFK = value
		case "prk":
			rel.pivotRK = value
		default:
			return nil, fmt.Errorf("database: field %s.%s: unknown rel option %q", parent.Name(), field.Name, key)
		}
	}

	return rel, nil
}

// relatedStructType unwraps []*R, []R, *R and R to the struct type R.
func relatedStructType(t reflect.Type) (reflect.Type, error) {
	if t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("relation field must reference a model struct, got %s", t.Kind())
	}
	return t, nil
}

// --- eager loading ---

// loadRelationsValue loads the given relation paths ("Posts", "Posts.Tags")
// for every element of models, an addressable slice of model structs.
func loadRelationsValue(driver string, executor query.Executor, models reflect.Value, paths []string) error {
	if models.Len() == 0 || len(paths) == 0 {
		return nil
	}

	elemType := models.Type().Elem()
	relations, err := relationsFor(elemType)
	if err != nil {
		return err
	}

	// Group nested paths under their first segment.
	nested := make(map[string][]string)
	var heads []string
	for _, path := range paths {
		head, tail, _ := strings.Cut(path, ".")
		if _, seen := nested[head]; !seen {
			heads = append(heads, head)
			nested[head] = nil
		}
		if tail != "" {
			nested[head] = append(nested[head], tail)
		}
	}

	for _, head := range heads {
		rel, ok := relations[head]
		if !ok {
			return fmt.Errorf("database: %s has no relation %q (declare it with a rel tag)", elemType.Name(), head)
		}
		if err := loadRelation(driver, executor, models, rel, nested[head]); err != nil {
			return fmt.Errorf("database: loading %s.%s: %w", elemType.Name(), head, err)
		}
	}
	return nil
}

func loadRelation(driver string, executor query.Executor, models reflect.Value, rel *relation, nested []string) error {
	switch rel.kind {
	case relHasOne, relHasMany:
		return loadHasMany(driver, executor, models, rel, nested)
	case relBelongsTo:
		return loadBelongsTo(driver, executor, models, rel, nested)
	case relBelongsToMany:
		return loadBelongsToMany(driver, executor, models, rel, nested)
	}
	return fmt.Errorf("unsupported relation kind")
}

// loadHasMany also serves hasOne (a hasMany capped at one per parent).
func loadHasMany(driver string, executor query.Executor, models reflect.Value, rel *relation, nested []string) error {
	parentMeta, err := metaFor(models.Type().Elem())
	if err != nil {
		return err
	}
	ownerField, ok := parentMeta.byCol[rel.ownerKey]
	if !ok {
		return fmt.Errorf("owner key %q is not a column of %s", rel.ownerKey, parentMeta.table)
	}

	keys := collectKeys(models, ownerField.index)
	if len(keys) == 0 {
		clearRelationField(models, rel)
		return nil
	}

	relatedMeta, err := metaFor(rel.related)
	if err != nil {
		return err
	}
	relatedSlice, err := fetchInto(driver, executor, rel.related,
		query.New(driver, executor).Table(relatedMeta.table).WhereIn(rel.foreignKey, keys...))
	if err != nil {
		return err
	}
	if err := loadRelationsValue(driver, executor, relatedSlice, nested); err != nil {
		return err
	}

	fkField, ok := relatedMeta.byCol[rel.foreignKey]
	if !ok {
		return fmt.Errorf("foreign key %q is not a column of %s", rel.foreignKey, relatedMeta.table)
	}

	// Bucket related rows by foreign key.
	buckets := make(map[string][]int)
	for i := 0; i < relatedSlice.Len(); i++ {
		key := keyString(relatedSlice.Index(i).FieldByIndex(fkField.index).Interface())
		buckets[key] = append(buckets[key], i)
	}

	for i := 0; i < models.Len(); i++ {
		parent := models.Index(i)
		key := keyString(parent.FieldByIndex(ownerField.index).Interface())
		assignRelated(parent.FieldByIndex(rel.fieldIndex), relatedSlice, buckets[key], rel.kind == relHasOne)
	}
	return nil
}

func loadBelongsTo(driver string, executor query.Executor, models reflect.Value, rel *relation, nested []string) error {
	parentMeta, err := metaFor(models.Type().Elem())
	if err != nil {
		return err
	}
	fkField, ok := parentMeta.byCol[rel.foreignKey]
	if !ok {
		return fmt.Errorf("foreign key %q is not a column of %s", rel.foreignKey, parentMeta.table)
	}

	keys := collectKeys(models, fkField.index)
	if len(keys) == 0 {
		return nil
	}

	relatedMeta, err := metaFor(rel.related)
	if err != nil {
		return err
	}
	relatedSlice, err := fetchInto(driver, executor, rel.related,
		query.New(driver, executor).Table(relatedMeta.table).WhereIn(rel.ownerKey, keys...))
	if err != nil {
		return err
	}
	if err := loadRelationsValue(driver, executor, relatedSlice, nested); err != nil {
		return err
	}

	ownerField, ok := relatedMeta.byCol[rel.ownerKey]
	if !ok {
		return fmt.Errorf("owner key %q is not a column of %s", rel.ownerKey, relatedMeta.table)
	}
	byKey := make(map[string]int)
	for i := 0; i < relatedSlice.Len(); i++ {
		byKey[keyString(relatedSlice.Index(i).FieldByIndex(ownerField.index).Interface())] = i
	}

	for i := 0; i < models.Len(); i++ {
		parent := models.Index(i)
		key := keyString(parent.FieldByIndex(fkField.index).Interface())
		if idx, ok := byKey[key]; ok {
			assignRelated(parent.FieldByIndex(rel.fieldIndex), relatedSlice, []int{idx}, true)
		}
	}
	return nil
}

func loadBelongsToMany(driver string, executor query.Executor, models reflect.Value, rel *relation, nested []string) error {
	parentMeta, err := metaFor(models.Type().Elem())
	if err != nil {
		return err
	}
	ownerField, ok := parentMeta.byCol[rel.ownerKey]
	if !ok {
		return fmt.Errorf("owner key %q is not a column of %s", rel.ownerKey, parentMeta.table)
	}

	keys := collectKeys(models, ownerField.index)
	if len(keys) == 0 {
		return nil
	}

	// Pivot rows: parent key -> related keys.
	pivotRows, err := query.New(driver, executor).Table(rel.pivotTable).WhereIn(rel.pivotFK, keys...).Get()
	if err != nil {
		return err
	}
	links := make(map[string][]string)
	relatedKeySet := make(map[string]bool)
	var relatedKeys []any
	for _, row := range pivotRows {
		parentKey := keyString(row[rel.pivotFK])
		relatedKey := keyString(row[rel.pivotRK])
		links[parentKey] = append(links[parentKey], relatedKey)
		if !relatedKeySet[relatedKey] {
			relatedKeySet[relatedKey] = true
			relatedKeys = append(relatedKeys, row[rel.pivotRK])
		}
	}
	if len(relatedKeys) == 0 {
		// Nothing linked: clear the field so a reload never leaves
		// stale, previously-loaded relations behind.
		clearRelationField(models, rel)
		return nil
	}

	relatedMeta, err := metaFor(rel.related)
	if err != nil {
		return err
	}
	relatedSlice, err := fetchInto(driver, executor, rel.related,
		query.New(driver, executor).Table(relatedMeta.table).WhereIn(rel.ownerKey, relatedKeys...))
	if err != nil {
		return err
	}
	if err := loadRelationsValue(driver, executor, relatedSlice, nested); err != nil {
		return err
	}

	relOwnerField, ok := relatedMeta.byCol[rel.ownerKey]
	if !ok {
		return fmt.Errorf("owner key %q is not a column of %s", rel.ownerKey, relatedMeta.table)
	}
	byKey := make(map[string]int)
	for i := 0; i < relatedSlice.Len(); i++ {
		byKey[keyString(relatedSlice.Index(i).FieldByIndex(relOwnerField.index).Interface())] = i
	}

	for i := 0; i < models.Len(); i++ {
		parent := models.Index(i)
		parentKey := keyString(parent.FieldByIndex(ownerField.index).Interface())
		var indices []int
		for _, relatedKey := range links[parentKey] {
			if idx, ok := byKey[relatedKey]; ok {
				indices = append(indices, idx)
			}
		}
		assignRelated(parent.FieldByIndex(rel.fieldIndex), relatedSlice, indices, false)
	}
	return nil
}

// fetchInto runs the builder and scans its rows into an addressable slice
// of the given struct type. Soft-deleted related rows are excluded, like
// Laravel's relations.
func fetchInto(driver string, executor query.Executor, structType reflect.Type, builder *query.Builder) (reflect.Value, error) {
	if meta, err := metaFor(structType); err == nil && meta.softDeletes {
		builder.WhereNull("deleted_at")
	}
	rows, err := builder.Rows()
	if err != nil {
		return reflect.Value{}, err
	}
	return scanRowsIntoType(rows, structType)
}

// clearRelationField zeroes the relation field on every model, so
// reloading after a detach reflects the empty state.
func clearRelationField(models reflect.Value, rel *relation) {
	for i := 0; i < models.Len(); i++ {
		field := models.Index(i).FieldByIndex(rel.fieldIndex)
		field.Set(reflect.Zero(field.Type()))
	}
}

// collectKeys gathers the distinct, non-zero key values of a struct field
// across the models slice, preserving order.
func collectKeys(models reflect.Value, fieldIndex []int) []any {
	seen := make(map[string]bool)
	var keys []any
	for i := 0; i < models.Len(); i++ {
		value := models.Index(i).FieldByIndex(fieldIndex).Interface()
		key := keyString(value)
		if key == "" || key == "0" || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, value)
	}
	return keys
}

// keyString normalizes a key value for map lookups, so an int64 scanned
// from the database matches an int struct field, etc.
func keyString(v any) string {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return fmt.Sprint(v)
}

// assignRelated sets a relation field from the chosen elements of the
// related slice, adapting to the field's declared shape
// ([]*R, []R, *R, or R).
func assignRelated(field reflect.Value, relatedSlice reflect.Value, indices []int, single bool) {
	if single {
		if len(indices) == 0 {
			// No related row: zero the field so reloads never keep a
			// stale pointer from an earlier load.
			field.Set(reflect.Zero(field.Type()))
			return
		}
		element := relatedSlice.Index(indices[0])
		if field.Kind() == reflect.Pointer {
			ptr := reflect.New(element.Type())
			ptr.Elem().Set(element)
			field.Set(ptr)
		} else {
			field.Set(element)
		}
		return
	}

	elemType := field.Type().Elem()
	out := reflect.MakeSlice(field.Type(), 0, len(indices))
	for _, idx := range indices {
		element := relatedSlice.Index(idx)
		if elemType.Kind() == reflect.Pointer {
			ptr := reflect.New(element.Type())
			ptr.Elem().Set(element)
			out = reflect.Append(out, ptr)
		} else {
			out = reflect.Append(out, element)
		}
	}
	field.Set(out)
}

// --- public entry points ---

// Load lazily loads relations onto an already-fetched model:
//
//	user, _ := database.Find[User](1)
//	database.Load(user, "Posts", "Posts.Tags")
func Load[T any](model *T, paths ...string) error {
	driver, executor := connectionFor[T]()
	return LoadOn(driver, executor, model, paths...)
}

// LoadOn is Load against an explicit executor (e.g. a transaction).
func LoadOn[T any](driver string, executor query.Executor, model *T, paths ...string) error {
	slice := reflect.New(reflect.SliceOf(reflect.TypeOf(*model))).Elem()
	slice.Set(reflect.Append(slice, reflect.ValueOf(*model)))
	if err := loadRelationsValue(driver, executor, slice, paths); err != nil {
		return err
	}
	*model = slice.Index(0).Interface().(T)
	return nil
}

// LoadAll lazily loads relations for a slice of models in batch.
func LoadAll[T any](models []T, paths ...string) error {
	driver, executor := connectionFor[T]()
	slice := reflect.ValueOf(&models).Elem()
	return loadRelationsValue(driver, executor, slice, paths)
}
