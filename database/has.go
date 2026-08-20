package database

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/genesysflow/go-genesys/query"
)

// Has constrains the query to models that have at least one related row,
// Laravel's has():
//
//	authors, _ := database.Query[Author]().Has("Posts").Get()
//
// Dotted paths check nested relations: Has("Posts.Tags").
func (q *ModelQuery[T]) Has(relation string) *ModelQuery[T] {
	return q.WhereHas(relation, nil)
}

// DoesntHave constrains the query to models with no related rows.
func (q *ModelQuery[T]) DoesntHave(relation string) *ModelQuery[T] {
	return q.whereHas(relation, nil, "AND", true)
}

// WhereHas is Has with extra constraints on the related rows. The
// callback receives the related table's query builder:
//
//	authors, _ := database.Query[Author]().
//	    WhereHas("Posts", func(posts *query.Builder) {
//	        posts.Where("published", true)
//	    }).Get()
func (q *ModelQuery[T]) WhereHas(relation string, fn func(*query.Builder)) *ModelQuery[T] {
	return q.whereHas(relation, fn, "AND", false)
}

// OrWhereHas is WhereHas joined with OR.
func (q *ModelQuery[T]) OrWhereHas(relation string, fn func(*query.Builder)) *ModelQuery[T] {
	return q.whereHas(relation, fn, "OR", false)
}

func (q *ModelQuery[T]) whereHas(relation string, fn func(*query.Builder), boolean string, not bool) *ModelQuery[T] {
	if q.err != nil || q.builder == nil {
		return q
	}
	parentType := reflect.TypeOf((*T)(nil)).Elem()
	sub, err := q.existsSubquery(parentType, q.meta.table, relation, fn)
	if err != nil {
		q.err = err
		return q
	}
	switch {
	case not:
		q.builder.WhereNotExists(sub)
	case boolean == "OR":
		q.builder.OrWhereExists(sub)
	default:
		q.builder.WhereExists(sub)
	}
	return q
}

// existsSubquery builds the correlated subquery for one relation path
// segment, recursing into nested segments ("Posts.Tags").
func (q *ModelQuery[T]) existsSubquery(parentType reflect.Type, parentTable, path string, fn func(*query.Builder)) (*query.Builder, error) {
	head, rest, _ := strings.Cut(path, ".")

	relations, err := relationsFor(parentType)
	if err != nil {
		return nil, err
	}
	rel, ok := relations[head]
	if !ok {
		return nil, fmt.Errorf("database: %s has no relation %q (declare it with a rel tag)", parentType.Name(), head)
	}
	relatedMeta, err := metaFor(rel.related)
	if err != nil {
		return nil, err
	}

	var sub *query.Builder
	relatedTable := relatedMeta.table

	// Self-referential relations must alias the inner table, or the
	// subquery's columns would shadow the outer row's and the EXISTS
	// would be uncorrelated (Laravel's laravel_reserved_N alias).
	innerRef := relatedTable
	if relatedTable == parentTable {
		innerRef = "genesys_self_" + relatedTable
	}
	tableExpr := func(name, ref string) string {
		if name == ref {
			return name
		}
		return name + " as " + ref
	}

	switch rel.kind {
	case relHasOne, relHasMany:
		sub = query.New(q.driver, q.executor).Table(tableExpr(relatedTable, innerRef)).
			WhereColumn(innerRef+"."+rel.foreignKey, "=", parentTable+"."+rel.ownerKey)
	case relBelongsTo:
		sub = query.New(q.driver, q.executor).Table(tableExpr(relatedTable, innerRef)).
			WhereColumn(innerRef+"."+rel.ownerKey, "=", parentTable+"."+rel.foreignKey)
	case relBelongsToMany:
		pivotRef := rel.pivotTable
		if rel.pivotTable == parentTable {
			pivotRef = "genesys_self_" + rel.pivotTable
		}
		sub = query.New(q.driver, q.executor).Table(tableExpr(rel.pivotTable, pivotRef)).
			Join(tableExpr(relatedTable, innerRef), pivotRef+"."+rel.pivotRK, "=", innerRef+"."+rel.ownerKey).
			WhereColumn(pivotRef+"."+rel.pivotFK, "=", parentTable+"."+rel.ownerKey)
	default:
		return nil, fmt.Errorf("database: unsupported relation kind for %q", head)
	}

	if relatedMeta.softDeletes {
		sub.WhereNull(innerRef + ".deleted_at")
	}

	if rest != "" {
		nested, err := q.existsSubquery(rel.related, innerRef, rest, fn)
		if err != nil {
			return nil, err
		}
		sub.WhereExists(nested)
	} else if fn != nil {
		fn(sub)
	}

	return sub, nil
}
