package restserver

import (
	"github.com/Azure/azure-container-networking/cns/fakes"
)

func setMockNMAgent(h *HTTPRestService, m *fakes.NMAgentClientFake) func() {
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
