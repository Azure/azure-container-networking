package ciliumpool

import (
	"context"
	"testing"
)

func TestAPIInvalidVersionTestRetriesButReplacementDoesNot(t *testing.T) {
	owner := Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}
	t.Run("invalid version", func(t *testing.T) {
		a, api, factoryErr := testAllocator()
		if factoryErr != nil {
			t.Fatal(factoryErr)
		}
		t.Cleanup(api.server.Close)
		api.conflicts = 1
		api.invalidConflict = true
		if _, err := a.Add(context.Background(), owner); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("object replacement", func(t *testing.T) {
		a, api, factoryErr := testAllocator()
		if factoryErr != nil {
			t.Fatal(factoryErr)
		}
		t.Cleanup(api.server.Close)
		api.conflicts = 1
		api.replaceOnConflict = true
		if _, err := a.Add(context.Background(), owner); err == nil {
			t.Fatal("retried allocation into replacement CiliumNode")
		}
	})
	t.Run("binding drift", func(t *testing.T) {
		a, api, factoryErr := testAllocator()
		if factoryErr != nil {
			t.Fatal(factoryErr)
		}
		t.Cleanup(api.server.Close)
		annotations := api.node.GetAnnotations()
		annotations[vmAnnotation] = testForeign
		api.node.SetAnnotations(annotations)
		if _, err := a.Add(context.Background(), owner); err == nil {
			t.Fatal("foreign VM binding admitted")
		}
	})
}
