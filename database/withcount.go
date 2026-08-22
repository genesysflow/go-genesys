package database

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/genesysflow/go-genesys/query"
)

// WithCount batch-counts related rows per model - Laravel's
// withCount(). Each counted relation populates a struct field named
// "<Relation>Count", which must be excluded from column mapping:
//
//	type Author struct {
//	    database.Model
//	    Posts      []*Post `rel:"hasMany"`
//	    PostsCount int64   `db:"-"`
//	}
//
//	authors, _ := database.Query[Author]().WithCount("Posts").Get()
func (q *ModelQuery[T]) WithCount(relations ...string) *ModelQuery[T] {
	q.withCounts = append(q.withCounts, relations...)
	return q
}

// loadRelationCounts fills the "<Relation>Count" fields for every model.
func loadRelationCounts(driver string, executor query.Executor, models reflect.Value, names []string) error {
	if models.Len() == 0 || len(names) == 0 {
		return nil
	}
	elemType := models.Type().Elem()
	relations, err := relationsFor(elemType)
	if err != nil {
		return err
	}
	parentMeta, err := metaFor(elemType)
	if err != nil {
		return err
	}

	for _, name := range names {
		rel, ok := relations[name]
		if !ok {
			return fmt.Errorf("database: %s has no relation %q (declare it with a rel tag)", elemType.Name(), name)
		}
		countField, ok := elemType.FieldByName(name + "Count")
		if !ok {
			return fmt.Errorf("database: WithCount(%q) needs a %s.%sCount field tagged db:\"-\"", name, elemType.Name(), name)
		}
		switch countField.Type.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		default:
			return fmt.Errorf("database: %s.%sCount must be an integer field", elemType.Name(), name)
		}

		counts, keyField, err := countRelation(driver, executor, parentMeta, models, rel)
		if err != nil {
			return fmt.Errorf("database: counting %s.%s: %w", elemType.Name(), name, err)
		}

		for i := 0; i < models.Len(); i++ {
			parent := models.Index(i)
			key := keyString(parent.FieldByIndex(keyField.index).Interface())
			parent.FieldByIndex(countField.Index).SetInt(counts[key])
		}
	}
	return nil
}

// countRelation runs one grouped count query for a relation and returns
// parent-key -> count plus the parent field the keys came from.
func countRelation(driver string, executor query.Executor, parentMeta *modelMeta, models reflect.Value, rel *relation) (map[string]int64, *fieldMeta, error) {
	relatedMeta, err := metaFor(rel.related)
	if err != nil {
		return nil, nil, err
	}

	// belongsTo counts by the parent's foreign key; everything else by
	// the parent's owner key.
	parentKeyCol := rel.ownerKey
	if rel.kind == relBelongsTo {
		parentKeyCol = rel.foreignKey
	}
	keyField, ok := parentMeta.byCol[parentKeyCol]
	if !ok {
		return nil, nil, fmt.Errorf("key %q is not a column of %s", parentKeyCol, parentMeta.table)
	}
	keys := collectKeys(models, keyField.index)
	if len(keys) == 0 {
		return map[string]int64{}, keyField, nil
	}

	var builder *query.Builder
	var groupCol string

	switch rel.kind {
	case relHasOne, relHasMany, relMorphOne, relMorphMany:
		groupCol = rel.foreignKey
		builder = query.New(driver, executor).Table(relatedMeta.table).
			WhereIn(groupCol, keys...)
		if rel.kind == relMorphOne || rel.kind == relMorphMany {
			builder.Where(rel.morphTypeCol(), parentMeta.table)
		}
		if relatedMeta.softDeletes {
			builder.WhereNull("deleted_at")
		}
	case relBelongsTo:
		groupCol = rel.ownerKey
		builder = query.New(driver, executor).Table(relatedMeta.table).
			WhereIn(groupCol, keys...)
		if relatedMeta.softDeletes {
			builder.WhereNull("deleted_at")
		}
	case relBelongsToMany, relMorphToMany:
		groupCol = rel.pivotTable + "." + rel.pivotFK
		builder = query.New(driver, executor).Table(rel.pivotTable).
			Join(relatedMeta.table, rel.pivotTable+"."+rel.pivotRK, "=", relatedMeta.table+"."+rel.ownerKey).
			WhereIn(groupCol, keys...)
		if rel.kind == relMorphToMany {
			builder.Where(rel.pivotTable+"."+rel.morphTypeCol(), parentMeta.table)
		}
		if relatedMeta.softDeletes {
			builder.WhereNull(relatedMeta.table + ".deleted_at")
		}
	case relHasOneThrough, relHasManyThrough:
		groupCol = rel.throughTable + "." + rel.foreignKey
		builder = query.New(driver, executor).Table(relatedMeta.table).
			Join(rel.throughTable, rel.throughTable+"."+rel.throughLocal, "=", relatedMeta.table+"."+rel.secondKey).
			WhereIn(groupCol, keys...)
		if relatedMeta.softDeletes {
			builder.WhereNull(relatedMeta.table + ".deleted_at")
		}
	default:
		return nil, nil, fmt.Errorf("unsupported relation kind for WithCount")
	}

	rows, err := builder.
		Select(groupCol).SelectRaw("COUNT(*) as aggregate").
		GroupBy(groupCol).
		Get()
	if err != nil {
		return nil, nil, err
	}

	// Drivers return a qualified select column under its bare name.
	resultCol := groupCol
	if idx := strings.LastIndex(groupCol, "."); idx >= 0 {
		resultCol = groupCol[idx+1:]
	}

	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[keyString(row[resultCol])] = toInt64Scan(row["aggregate"])
	}
	return counts, keyField, nil
}

// toInt64Scan converts a driver-returned aggregate to int64.
func toInt64Scan(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case []byte:
		var out int64
		fmt.Sscanf(string(n), "%d", &out)
		return out
	case string:
		var out int64
		fmt.Sscanf(n, "%d", &out)
		return out
	}
	return 0
}
