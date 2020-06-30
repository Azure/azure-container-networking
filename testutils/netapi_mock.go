package testutils

type NetApiMock struct {
	err error
}

func (netApi *NetApiMock) AddExternalInterface(ifName string, subnet string) error {
	return netApi.err
}
