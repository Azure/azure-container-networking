package ciliumpool

import "testing"

func TestBootIdentityMustBeKernelUUID(t *testing.T) {
	want := "9fb7798b-a983-480c-9118-d51720d029cd"
	if got, err := parseBootID([]byte(want + "\n")); err != nil || got != want {
		t.Fatalf("boot ID = %q, %v", got, err)
	}
	for _, bad := range []string{"", "unknown", "00000000-0000-0000-0000-000000000000"} {
		if _, err := parseBootID([]byte(bad)); err == nil {
			t.Fatalf("invalid boot identity %q could reset ownership", bad)
		}
	}
}
