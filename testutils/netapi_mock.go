//go:build !ignore_uncovered
// +build !ignore_uncovered

package testutils

type NetApiMock struct {
	err error
}

func (netApi *NetApiMock) AddExternalInterface(ifName string, subnet string) error {
	return netApi.err
}
