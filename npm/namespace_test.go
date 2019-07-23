// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"testing"
)

func TestnewNs(t *testing.T) {
	if _, err := newNs("test"); err != nil {
		t.Errorf("TestnewNs failed @ newNs")
	}
}