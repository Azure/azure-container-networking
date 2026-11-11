package restserver

import (
	"net/http"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/common"
)

func (service *HTTPRestService) supportedAPIsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	_ = common.Encode(w, cns.SupportedAPIsResponse{SupportedAPIs: []string{}})
}
