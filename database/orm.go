package database

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/genesysflow/go-genesys/query"
)

// ErrNotFound is returned when a model lookup matches no rows.
var ErrNotFound = errors.New("database: model not found")

var (
	defaultManager   *Manager
	defaultManagerMu sync.RWMutex
)

// SetDefault sets the manager used by the package-level ORM helpers
// (All, Find, Create, ...). Called by the database service provider.
func SetDefault(m *Manager) {
	defaultManagerMu.Lock()
	defer defaultManagerMu.Unlock()
	defaultManager = m
}

// Default returns the manager used by the package-level ORM helpers.
func Default() *Manager {
	defaultManagerMu.RLock()
	defer defaultManagerMu.RUnlock()
	return defaultManager
}

func mustDefault() *Manager {
	m := Default()
	if m == nil {
		panic("database: no default manager - register the DatabaseServiceProvider or call database.SetDefault")
	}
	return m
}

func connectionFor[T any]() (string, driverExecutor) {
	m := mustDefault()
	var model T
	name := ""
	if namer, ok := any(&model).(ConnectionNamer); ok {
		name = namer.ConnectionName()
	}
	conn := m.Connection(name)
	return conn.Driver(), conn
}

type driverExecutor interface {
	query.Executor
}

// ModelQuery is a typed query builder for a model.
type ModelQuery[T any] struct {
	builder  *query.Builder
	driver   string
	executor query.Executor
	withs    []string
	err      error
}

// Query starts a typed query for a model:
//
//	adults, err := database.Query[User]().Where("age", ">=", 18).OrderBy("name").Get()
func Query[T any]() *ModelQuery[T] {
	meta, err := metaFor(reflect.TypeOf((*T)(nil)).Elem())
	if err != nil {
		return &ModelQuery[T]{err: err}
	}
	driver, executor := connectionFor[T]()
	return &ModelQuery[T]{
		builder:  query.New(driver, executor).Table(meta.table),
		driver:   driver,
		executor: executor,
	}
}

// QueryOn starts a typed query against an explicit executor (e.g. a transaction).
func QueryOn[T any](driver string, executor query.Executor) *ModelQuery[T] {
	meta, err := metaFor(reflect.TypeOf((*T)(nil)).Elem())
	if err != nil {
		return &ModelQuery[T]{err: err}
	}
	return &ModelQuery[T]{
		builder:  query.New(driver, executor).Table(meta.table),
		driver:   driver,
		executor: executor,
	}
}

// With eager-loads the given relation paths alongside the query results,
// one batched query per relation instead of one per row:
//
//	users, err := database.Query[User]().With("Posts", "Posts.Tags").Get()
func (q *ModelQuery[T]) With(paths ...string) *ModelQuery[T] {
	q.withs = append(q.withs, paths...)
	return q
}

// loadWiths eager-loads the requested relations onto scanned results.
func (q *ModelQuery[T]) loadWiths(results []T) error {
	if len(q.withs) == 0 || len(results) == 0 {
		return nil
	}
	slice := reflect.ValueOf(&results).Elem()
	return loadRelationsValue(q.driver, q.executor, slice, q.withs)
}

// Builder exposes the underlying untyped query builder.
func (q *ModelQuery[T]) Builder() *query.Builder {
	return q.builder
}

// Where adds a where clause. Accepts (column, value) or (column, operator, value).
func (q *ModelQuery[T]) Where(column string, args ...any) *ModelQuery[T] {
	if q.builder != nil {
		q.builder.Where(column, args...)
	}
	return q
}

// OrWhere adds an OR where clause.
func (q *ModelQuery[T]) OrWhere(column string, args ...any) *ModelQuery[T] {
	if q.builder != nil {
		q.builder.OrWhere(column, args...)
	}
	return q
}

// WhereIn adds a WHERE column IN (...) clause.
func (q *ModelQuery[T]) WhereIn(column string, values ...any) *ModelQuery[T] {
	if q.builder != nil {
		q.builder.WhereIn(column, values...)
	}
	return q
}

// WhereNull adds a WHERE column IS NULL clause.
func (q *ModelQuery[T]) WhereNull(column string) *ModelQuery[T] {
	if q.builder != nil {
		q.builder.WhereNull(column)
	}
	return q
}

// WhereNotNull adds a WHERE column IS NOT NULL clause.
func (q *ModelQuery[T]) WhereNotNull(column string) *ModelQuery[T] {
	if q.builder != nil {
		q.builder.WhereNotNull(column)
	}
	return q
}

// OrderBy adds an ORDER BY clause.
func (q *ModelQuery[T]) OrderBy(column string, direction ...string) *ModelQuery[T] {
	if q.builder != nil {
		q.builder.OrderBy(column, direction...)
	}
	return q
}

// OrderByDesc adds a descending ORDER BY clause.
func (q *ModelQuery[T]) OrderByDesc(column string) *ModelQuery[T] {
	if q.builder != nil {
		q.builder.OrderByDesc(column)
	}
	return q
}

// Latest orders by created_at descending.
func (q *ModelQuery[T]) Latest(column ...string) *ModelQuery[T] {
	if q.builder != nil {
		q.builder.Latest(column...)
	}
	return q
}

// Limit caps the number of rows returned.
func (q *ModelQuery[T]) Limit(limit int) *ModelQuery[T] {
	if q.builder != nil {
		q.builder.Limit(limit)
	}
	return q
}

// Offset skips the given number of rows.
func (q *ModelQuery[T]) Offset(offset int) *ModelQuery[T] {
	if q.builder != nil {
		q.builder.Offset(offset)
	}
	return q
}

// Get executes the query and scans results into models.
func (q *ModelQuery[T]) Get() ([]T, error) {
	if q.err != nil {
		return nil, q.err
	}
	rows, err := q.builder.Rows()
	if err != nil {
		return nil, err
	}
	results, err := scanRowsInto[T](rows)
	if err != nil {
		return nil, err
	}
	if err := q.loadWiths(results); err != nil {
		return nil, err
	}
	return results, nil
}

// First returns the first matching model or ErrNotFound.
func (q *ModelQuery[T]) First() (*T, error) {
	if q.err != nil {
		return nil, q.err
	}
	q.builder.Limit(1)
	results, err := q.Get()
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, ErrNotFound
	}
	return &results[0], nil
}

// Find returns the model with the given primary key or ErrNotFound.
func (q *ModelQuery[T]) Find(id any) (*T, error) {
	return q.Where("id", id).First()
}

// Count returns the number of matching rows.
func (q *ModelQuery[T]) Count() (int64, error) {
	if q.err != nil {
		return 0, q.err
	}
	return q.builder.Count()
}

// Exists reports whether any row matches.
func (q *ModelQuery[T]) Exists() (bool, error) {
	if q.err != nil {
		return false, q.err
	}
	return q.builder.Exists()
}

// Update applies a column map to all matching rows.
func (q *ModelQuery[T]) Update(values map[string]any) (int64, error) {
	if q.err != nil {
		return 0, q.err
	}
	return q.builder.Update(values)
}

// Delete removes all matching rows.
func (q *ModelQuery[T]) Delete() (int64, error) {
	if q.err != nil {
		return 0, q.err
	}
	return q.builder.Delete()
}

// ModelPaginator is a typed page of models with pagination metadata.
type ModelPaginator[T any] struct {
	Data        []T   `json:"data"`
	Total       int64 `json:"total"`
	PerPage     int   `json:"per_page"`
	CurrentPage int   `json:"current_page"`
	LastPage    int   `json:"last_page"`
	From        int   `json:"from"`
	To          int   `json:"to"`
}

// Paginate returns the given page of models with pagination metadata.
func (q *ModelQuery[T]) Paginate(page, perPage int) (*ModelPaginator[T], error) {
	if q.err != nil {
		return nil, q.err
	}
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 15
	}

	total, err := q.builder.Clone().Count()
	if err != nil {
		return nil, err
	}

	rows, err := q.builder.Clone().Limit(perPage).Offset((page - 1) * perPage).Rows()
	if err != nil {
		return nil, err
	}
	data, err := scanRowsInto[T](rows)
	if err != nil {
		return nil, err
	}
	if err := q.loadWiths(data); err != nil {
		return nil, err
	}

	lastPage := int((total + int64(perPage) - 1) / int64(perPage))
	if lastPage < 1 {
		lastPage = 1
	}
	from, to := 0, 0
	if len(data) > 0 {
		from = (page-1)*perPage + 1
		to = from + len(data) - 1
	}

	return &ModelPaginator[T]{
		Data: data, Total: total, PerPage: perPage,
		CurrentPage: page, LastPage: lastPage, From: from, To: to,
	}, nil
}

// All returns every row of the model's table.
func All[T any]() ([]T, error) {
	return Query[T]().Get()
}

// Find returns the model with the given primary key or ErrNotFound.
func Find[T any](id any) (*T, error) {
	return Query[T]().Find(id)
}

// FirstWhere returns the first model matching a simple where clause.
func FirstWhere[T any](column string, args ...any) (*T, error) {
	return Query[T]().Where(column, args...).First()
}

// Create inserts the model and sets its ID and timestamps in place.
func Create[T any](model *T) error {
	meta, err := metaFor(reflect.TypeOf(model).Elem())
	if err != nil {
		return err
	}

	touchTimestamps(meta, model, true)

	driver, executor := connectionFor[T]()
	values := meta.values(reflect.ValueOf(model), true)

	id, err := query.New(driver, executor).Table(meta.table).InsertGetID(values)
	if err != nil {
		return err
	}
	setPK(meta, model, id)
	return nil
}

// Save inserts the model when its ID is zero, otherwise updates it.
func Save[T any](model *T) error {
	meta, err := metaFor(reflect.TypeOf(model).Elem())
	if err != nil {
		return err
	}
	if pkValue(meta, model) == 0 {
		return Create(model)
	}
	return Update(model)
}

// Update persists all of the model's columns by primary key.
func Update[T any](model *T) error {
	meta, err := metaFor(reflect.TypeOf(model).Elem())
	if err != nil {
		return err
	}
	id := pkValue(meta, model)
	if id == 0 {
		return fmt.Errorf("database: cannot update %s without an id", meta.table)
	}

	touchTimestamps(meta, model, false)

	driver, executor := connectionFor[T]()
	values := meta.values(reflect.ValueOf(model), true)

	affected, err := query.New(driver, executor).Table(meta.table).Where("id", id).Update(values)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes the row with the given primary key.
func Delete[T any](id any) error {
	meta, err := metaFor(reflect.TypeOf((*T)(nil)).Elem())
	if err != nil {
		return err
	}
	driver, executor := connectionFor[T]()
	affected, err := query.New(driver, executor).Table(meta.table).Where("id", id).Delete()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteModel removes the given model's row by primary key.
func DeleteModel[T any](model *T) error {
	meta, err := metaFor(reflect.TypeOf(model).Elem())
	if err != nil {
		return err
	}
	return Delete[T](pkValue(meta, model))
}

func pkValue[T any](meta *modelMeta, model *T) int64 {
	if meta.pkIndex < 0 {
		return 0
	}
	v := reflect.ValueOf(model).Elem().FieldByIndex(meta.fields[meta.pkIndex].index)
	if v.CanInt() {
		return v.Int()
	}
	return 0
}

func setPK[T any](meta *modelMeta, model *T, id int64) {
	if meta.pkIndex < 0 || id == 0 {
		return
	}
	v := reflect.ValueOf(model).Elem().FieldByIndex(meta.fields[meta.pkIndex].index)
	if v.CanInt() && v.CanSet() {
		v.SetInt(id)
	}
}

// touchTimestamps fills created_at (on insert) and updated_at.
func touchTimestamps[T any](meta *modelMeta, model *T, creating bool) {
	now := time.Now().UTC().Truncate(time.Second)
	v := reflect.ValueOf(model).Elem()
	for _, f := range meta.fields {
		if f.isCreated && creating {
			field := v.FieldByIndex(f.index)
			if t, ok := field.Interface().(time.Time); ok && t.IsZero() {
				field.Set(reflect.ValueOf(now))
			}
		}
		if f.isUpdated {
			field := v.FieldByIndex(f.index)
			if _, ok := field.Interface().(time.Time); ok {
				field.Set(reflect.ValueOf(now))
			}
		}
	}
}
