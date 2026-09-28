// Package redact removes configured secret values from structured data before
// it is logged, sent to a model, or persisted.
package redact

import (
	"reflect"
	"strings"
)

// Redactor replaces exact configured secret values in strings.
type Redactor struct {
	secrets []string
}

// New returns a Redactor containing the unique, non-empty secret values.
func New(values ...string) Redactor {
	secrets := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		secrets = append(secrets, value)
	}
	return Redactor{secrets: secrets}
}

// String redacts configured secret values from value.
func (r Redactor) String(value string) string {
	for _, secret := range r.secrets {
		value = strings.ReplaceAll(value, secret, "[REDACTED]")
	}
	return value
}

// Apply redacts every string in the value referenced by target, including map
// keys and nested structs, slices, maps, pointers, and interfaces.
func (r Redactor) Apply(target any) {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		panic("redact: target must be a non-nil pointer")
	}
	value.Elem().Set(r.redactValue(value.Elem()))
}

func (r Redactor) redactValue(value reflect.Value) reflect.Value {
	switch value.Kind() {
	case reflect.String:
		redacted := reflect.ValueOf(r.String(value.String()))
		return redacted.Convert(value.Type())
	case reflect.Pointer:
		if value.IsNil() {
			return value
		}
		redacted := reflect.New(value.Type().Elem())
		redacted.Elem().Set(r.redactValue(value.Elem()))
		return redacted
	case reflect.Interface:
		if value.IsNil() {
			return value
		}
		redacted := reflect.New(value.Type()).Elem()
		redacted.Set(r.redactValue(value.Elem()))
		return redacted
	case reflect.Struct:
		redacted := reflect.New(value.Type()).Elem()
		redacted.Set(value)
		for i := 0; i < value.NumField(); i++ {
			field := value.Type().Field(i)
			if field.PkgPath != "" {
				continue
			}
			redacted.Field(i).Set(r.redactValue(value.Field(i)))
		}
		return redacted
	case reflect.Slice:
		if value.IsNil() {
			return value
		}
		redacted := reflect.MakeSlice(value.Type(), value.Len(), value.Cap())
		for i := 0; i < value.Len(); i++ {
			redacted.Index(i).Set(r.redactValue(value.Index(i)))
		}
		return redacted
	case reflect.Array:
		redacted := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			redacted.Index(i).Set(r.redactValue(value.Index(i)))
		}
		return redacted
	case reflect.Map:
		if value.IsNil() {
			return value
		}
		redacted := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			key := r.redactValue(iter.Key())
			item := r.redactValue(iter.Value())
			redacted.SetMapIndex(key, item)
		}
		return redacted
	default:
		return value
	}
}
