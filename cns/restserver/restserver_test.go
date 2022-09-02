package restserver

import (
	"context"

	"github.com/Azure/azure-container-networking/nmagent"
)

func setMockNMAgent(h *HTTPRestService, m *MockNMAgent) func() {
	// this is a hack that exists because the tests are too DRY, so the setup
	// logic has ossified in TestMain

	// save the previous value of the NMAgent so that it can be restored by the
	// cleanup function
	prev := h.nma

	// set the NMAgent to what was requested
	h.nma = m

	// return a cleanup function that will restore NMAgent back to what it was
	return func() {
		h.nma = prev
	}
}

type MockNMAgent struct {
	PutNetworkContainerF    func(context.Context, *nmagent.PutNetworkContainerRequest) error
	DeleteNetworkContainerF func(context.Context, nmagent.DeleteContainerRequest) error
	JoinNetworkF            func(context.Context, nmagent.JoinNetworkRequest) error
	SupportedAPIsF          func(context.Context) ([]string, error)
	GetNCVersionF           func(context.Context, nmagent.NCVersionRequest) (nmagent.NCVersion, error)
}

func (m *MockNMAgent) PutNetworkContainer(ctx context.Context, pncr *nmagent.PutNetworkContainerRequest) error {
	return m.PutNetworkContainerF(ctx, pncr)
}

func (m *MockNMAgent) DeleteNetworkContainer(ctx context.Context, dcr nmagent.DeleteContainerRequest) error {
	return m.DeleteNetworkContainerF(ctx, dcr)
}

func (m *MockNMAgent) JoinNetwork(ctx context.Context, jnr nmagent.JoinNetworkRequest) error {
	return m.JoinNetworkF(ctx, jnr)
}

func (m *MockNMAgent) SupportedAPIs(ctx context.Context) ([]string, error) {
	return m.SupportedAPIsF(ctx)
}

func (m *MockNMAgent) GetNCVersion(ctx context.Context, req nmagent.NCVersionRequest) (nmagent.NCVersion, error) {
	return m.GetNCVersionF(ctx, req)
}
