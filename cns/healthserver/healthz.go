package healthserver

import (
	"net/http"

	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	apiutil "sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(v1alpha.AddToScheme(scheme))
}

type Config struct {
	PingAPIServer bool
	Mapper        meta.RESTMapper
}

// NewHealthzHandlerWithChecks will return a [http.Handler] for CNS's /healthz endpoint.
// Depending on what we expect CNS to be able to read (based on the [configuration.CNSConfig])
// then the checks registered to the handler will test for those expectations. For example, in
// ChannelMode: CRD, the health check will ensure that CNS is able to list NNCs successfully.
func NewHealthzHandlerWithChecks(cfg *Config) (http.Handler, error) {
	checks := make(map[string]healthz.Checker)
	if cfg.PingAPIServer {
		restCfg, err := ctrl.GetConfig()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get kubeconfig")
		}
		// Use the provided (test) RESTMapper when present; otherwise fall back to a dynamic, discovery-based mapper for production.
		mapper := cfg.Mapper
		if mapper == nil {
			httpClient, httpErr := rest.HTTPClientFor(restCfg)
			if httpErr != nil {
				return nil, errors.Wrap(httpErr, "build http client for REST mapper")
			}
			mapper, err = apiutil.NewDynamicRESTMapper(restCfg, httpClient)
			if err != nil {
				return nil, errors.Wrap(err, "build rest mapper")
			}
		}
		cli, err := client.New(restCfg, client.Options{
			Scheme: scheme,
			Mapper: mapper,
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to build client")
		}
		checks["nnc"] = func(req *http.Request) error {
			ctx := req.Context()
			// we just care that we're allowed to List NNCs so set limit to 1 to minimize
			// additional load on apiserver
			if err := cli.List(ctx, &v1alpha.NodeNetworkConfigList{}, &client.ListOptions{
				Namespace: metav1.NamespaceSystem,
				Limit:     int64(1),
			}); err != nil {
				return errors.Wrap(err, "failed to list NodeNetworkConfig")
			}
			return nil
		}
	}
	return &healthz.Handler{
		Checks: checks,
	}, nil
}
