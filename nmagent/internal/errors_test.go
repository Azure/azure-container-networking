package internal

import (
	"net/http"
	"testing"
)

func TestErrorSource(t *testing.T) {
	esTests := []struct {
		sub string
		exp string
	}{
		{"wireserver", "wireserver"},
		{"nmagent", "nmagent"},
		{"garbage", ""},
		{"", ""},
	}

	for _, test := range esTests {
		test := test
		t.Run(test.sub, func(t *testing.T) {
			t.Parallel()

			// since this is intended for use with headers, this tests end-to-end
			es := NewErrorSource(test.sub)

			head := http.Header{}
			SetErrorSource(&head, es)
			gotEs := GetErrorSource(head)

			got := gotEs.String()

			if test.exp != got {
				t.Fatal("received value differs from expectation: exp:", test, "got:", got)
			}
		})
	}
}
