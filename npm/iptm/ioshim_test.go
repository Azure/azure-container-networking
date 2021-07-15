package iptm

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFakeIOShim(t *testing.T) {
	fake := NewFakeIptOperationShim()
	f, err := fake.openConfigFile(testFileName)
	require.NoError(t, err)

	s := bufio.NewScanner(f)
	res := ""
	for s.Scan() {
		res += s.Text()
	}

	require.Equal(t, strings.Replace(testIPTablesData, "\n", "", -1), res)
}
