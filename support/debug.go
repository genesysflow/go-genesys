package support

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// dumpWriter is swapped in tests.
var dumpWriter io.Writer = os.Stderr

// exit is swapped in tests so Dd is testable.
var exit = os.Exit

// Dump pretty-prints values for debugging, Laravel's dump().
func Dump(values ...any) {
	for _, value := range values {
		if encoded, err := json.MarshalIndent(value, "", "  "); err == nil {
			fmt.Fprintln(dumpWriter, string(encoded))
			continue
		}
		fmt.Fprintf(dumpWriter, "%#v\n", value)
	}
}

// Dd dumps the values and stops the program, Laravel's dd().
func Dd(values ...any) {
	Dump(values...)
	exit(1)
}

// DataGet retrieves a value from nested maps/slices/structs using dot
// notation, with "*" collecting across a slice - Laravel's data_get:
//
//	support.DataGet(payload, "user.address.city")
//	support.DataGet(payload, "orders.*.total")   // []any of every total
//	support.DataGet(payload, "missing.key", 0)   // fallback
func DataGet(data any, path string, fallback ...any) any {
	value, ok := dataGet(data, strings.Split(path, "."))
	if !ok {
		if len(fallback) > 0 {
			return fallback[0]
		}
		return nil
	}
	return value
}

func dataGet(data any, segments []string) (any, bool) {
	if len(segments) == 0 {
		return data, true
	}
	segment, rest := segments[0], segments[1:]

	if segment == "*" {
		items, ok := toSlice(data)
		if !ok {
			return nil, false
		}
		results := make([]any, 0, len(items))
		for _, item := range items {
			if value, ok := dataGet(item, rest); ok {
				results = append(results, value)
			}
		}
		return results, true
	}

	switch node := data.(type) {
	case map[string]any:
		value, ok := node[segment]
		if !ok {
			return nil, false
		}
		return dataGet(value, rest)
	}

	v := reflect.ValueOf(data)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, false
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Map:
		value := v.MapIndex(reflect.ValueOf(segment))
		if !value.IsValid() {
			return nil, false
		}
		return dataGet(value.Interface(), rest)
	case reflect.Slice, reflect.Array:
		index, err := strconv.Atoi(segment)
		if err != nil || index < 0 || index >= v.Len() {
			return nil, false
		}
		return dataGet(v.Index(index).Interface(), rest)
	case reflect.Struct:
		field := v.FieldByName(segment)
		if !field.IsValid() {
			return nil, false
		}
		return dataGet(field.Interface(), rest)
	}
	return nil, false
}

func toSlice(data any) ([]any, bool) {
	if items, ok := data.([]any); ok {
		return items, true
	}
	v := reflect.ValueOf(data)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return nil, false
	}
	items := make([]any, v.Len())
	for i := range items {
		items[i] = v.Index(i).Interface()
	}
	return items, true
}
