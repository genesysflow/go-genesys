package validation

import (
	"reflect"
	"regexp"
	"strings"

	"github.com/genesysflow/go-genesys/query"
	"github.com/go-playground/validator/v10"
)

// DatabaseResolver hands the validator a live connection at validation
// time. It is resolved per check rather than held, so provider boot order
// does not matter and a reconnect is picked up.
type DatabaseResolver func() (driver string, executor query.Executor, err error)

// SetDatabaseResolver wires the database-backed rules (unique, exists).
// Without it those rules fail closed - see the rule docs below.
func (v *Validator) SetDatabaseResolver(fn DatabaseResolver) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.database = fn
}

func (v *Validator) databaseResolver() DatabaseResolver {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.database
}

// identifierPattern is what a table or column name in a rule may look
// like. Names come from rules, which are code, but a rule assembled from
// a variable must not be able to reach into the SQL text.
var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// registerLaravelRules adds the rules go-playground does not carry:
// database-backed unique/exists, confirmed, and prohibited.
func (v *Validator) registerLaravelRules() {
	// unique keeps go-playground's slice semantics for params that are
	// not table.column - see uniqueRule.
	_ = v.validate.RegisterValidation("unique", v.uniqueRule)
	_ = v.validate.RegisterValidation("exists", v.existsRule)
	_ = v.validate.RegisterValidation("confirmed", confirmedRule)
	_ = v.validate.RegisterValidation("prohibited", prohibitedRule)
}

// uniqueRule implements Laravel's `unique:table,column[,ignore[,idColumn]]`
// written as a go-playground param:
//
//	validate:"unique=users.email"          // no other row may hold it
//	validate:"unique=users.email.42"       // ignoring the row with id 42
//	validate:"unique=users.email.7.team_id" // ignoring team_id 7
//
// A param that is not table.column keeps go-playground's original
// meaning: every element of the slice, array, or map must be distinct.
func (v *Validator) uniqueRule(fl validator.FieldLevel) bool {
	table, column, ignore, idColumn, ok := parseDatabaseParam(fl.Param())
	if !ok {
		return distinctValues(fl)
	}

	driver, executor, err := v.connection()
	if err != nil {
		// The check cannot be answered. Passing would wave through the
		// duplicate this rule exists to stop, so it fails closed.
		return false
	}

	builder := query.New(driver, executor).Table(table).Where(column, fl.Field().Interface())
	if ignore != "" {
		builder = builder.Where(idColumn, "!=", ignore)
	}

	exists, err := builder.Exists()
	if err != nil {
		return false
	}
	return !exists
}

// existsRule implements Laravel's `exists:table,column`:
//
//	validate:"exists=users.email"
func (v *Validator) existsRule(fl validator.FieldLevel) bool {
	table, column, _, _, ok := parseDatabaseParam(fl.Param())
	if !ok {
		return false
	}

	driver, executor, err := v.connection()
	if err != nil {
		return false
	}

	exists, err := query.New(driver, executor).Table(table).Where(column, fl.Field().Interface()).Exists()
	if err != nil {
		return false
	}
	return exists
}

// connection resolves the database for a rule check.
func (v *Validator) connection() (string, query.Executor, error) {
	resolve := v.databaseResolver()
	if resolve == nil {
		return "", nil, errNoDatabase
	}
	return resolve()
}

// errNoDatabase is returned when no resolver was wired up.
var errNoDatabase = &noDatabaseError{}

type noDatabaseError struct{}

func (e *noDatabaseError) Error() string {
	return "validation: the unique and exists rules need a database - call SetDatabaseResolver"
}

// parseDatabaseParam splits "table.column[.ignore[.idColumn]]". It
// reports ok=false when the param is not a table/column pair of plain
// identifiers.
func parseDatabaseParam(param string) (table, column, ignore, idColumn string, ok bool) {
	parts := strings.Split(param, ".")
	if len(parts) < 2 || len(parts) > 4 {
		return "", "", "", "", false
	}

	table, column = parts[0], parts[1]
	idColumn = "id"
	if len(parts) >= 3 {
		ignore = parts[2]
	}
	if len(parts) == 4 {
		idColumn = parts[3]
	}

	if !identifierPattern.MatchString(table) ||
		!identifierPattern.MatchString(column) ||
		!identifierPattern.MatchString(idColumn) {
		return "", "", "", "", false
	}
	return table, column, ignore, idColumn, true
}

// distinctValues reproduces go-playground's own `unique`: every element
// of a slice, array, or map must be distinct, optionally compared by the
// named struct field.
func distinctValues(fl validator.FieldLevel) bool {
	field := fl.Field()
	param := fl.Param()

	switch field.Kind() {
	case reflect.Slice, reflect.Array:
		seen := make(map[any]struct{}, field.Len())
		for i := 0; i < field.Len(); i++ {
			element := field.Index(i)
			if param != "" {
				element = reflect.Indirect(element)
				if element.Kind() != reflect.Struct {
					return false
				}
				element = element.FieldByName(param)
				if !element.IsValid() {
					return false
				}
			}
			seen[element.Interface()] = struct{}{}
		}
		return field.Len() == len(seen)

	case reflect.Map:
		seen := make(map[any]struct{}, field.Len())
		for _, key := range field.MapKeys() {
			seen[field.MapIndex(key).Interface()] = struct{}{}
		}
		return field.Len() == len(seen)

	default:
		// A scalar is trivially distinct from nothing.
		return true
	}
}

// confirmedRule implements Laravel's `confirmed`: the field must equal
// its `<field>_confirmation` sibling, so a mistyped password is caught
// before it is hashed and stored.
func confirmedRule(fl validator.FieldLevel) bool {
	parent := reflect.Indirect(fl.Parent())
	if parent.Kind() != reflect.Struct {
		return false
	}

	confirmation := parent.FieldByName(fl.StructFieldName() + "Confirmation")
	if !confirmation.IsValid() {
		return false
	}

	return reflect.DeepEqual(fl.Field().Interface(), confirmation.Interface())
}

// prohibitedRule implements Laravel's `prohibited`: the field must be
// absent or empty. It guards fields a client must never set, such as a
// role or an ownership column.
func prohibitedRule(fl validator.FieldLevel) bool {
	field := fl.Field()
	if !field.IsValid() {
		return true
	}
	return field.IsZero()
}
