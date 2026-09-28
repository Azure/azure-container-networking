package redact

import (
	"strings"
	"testing"
)

func TestApplyRedactsNestedStringsAndMapKeys(t *testing.T) {
	const secret = "sentinel-secret"
	value := struct {
		Name   string
		Values []string
		Items  map[string]string
		Nested *struct{ Value string }
	}{
		Name:   "name-" + secret,
		Values: []string{"value-" + secret},
		Items:  map[string]string{"key-" + secret: "item-" + secret},
		Nested: &struct{ Value string }{Value: "nested-" + secret},
	}

	New(secret).Apply(&value)

	if strings.Contains(value.Name, secret) ||
		strings.Contains(value.Values[0], secret) ||
		strings.Contains(value.Nested.Value, secret) {
		t.Fatal("configured secret remains in nested value")
	}
	for key, item := range value.Items {
		if strings.Contains(key, secret) || strings.Contains(item, secret) {
			t.Fatal("configured secret remains in map")
		}
	}
}
