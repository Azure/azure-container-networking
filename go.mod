module github.com/Azure/azure-container-networking

go 1.25.0

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.21.1
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.13.1
	github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets v0.12.0
	github.com/Masterminds/semver v1.5.0
	github.com/Microsoft/go-winio v0.6.2
	github.com/Microsoft/hcsshim v0.14.1
	github.com/avast/retry-go/v3 v3.1.1
	github.com/avast/retry-go/v4 v4.7.0
	github.com/billgraziano/dpapi v0.5.0
	github.com/containernetworking/cni v1.3.0
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/go-logr/zapr v1.3.0
	github.com/golang/mock v1.6.0
	github.com/golang/protobuf v1.5.4
	github.com/google/gnostic-models v0.7.0 // indirect
	github.com/google/go-cmp v0.7.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/hashicorp/go-version v1.9.0
	github.com/microsoft/ApplicationInsights-Go v0.4.4
	github.com/nxadm/tail v1.4.11
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.38.2
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/client_model v0.6.2
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	github.com/spf13/viper v1.21.0
	github.com/stretchr/testify v1.11.1
	go.uber.org/zap v1.27.1
	golang.org/x/exp v0.0.0-20251219203646-944ab1f22d93
	golang.org/x/sys v0.43.0
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260120221211-b8f7ae30c516 // indirect
	google.golang.org/grpc v1.80.0
	google.golang.org/protobuf v1.36.11
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
	k8s.io/api v0.35.4
	k8s.io/apiextensions-apiserver v0.35.3
	k8s.io/apimachinery v0.35.4
	k8s.io/client-go v0.35.4
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.140.0
	k8s.io/utils v0.0.0-20260319190234-28399d86e0b5
	sigs.k8s.io/controller-runtime v0.23.3
)

require (
	code.cloudfoundry.org/clock v1.41.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.12.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/keyvault/internal v0.7.1 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.6.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coreos/go-iptables v0.8.0
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fsnotify/fsnotify v1.9.0
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-openapi/jsonpointer v0.22.4 // indirect
	github.com/go-openapi/jsonreference v0.21.4 // indirect
	github.com/go-openapi/swag v0.25.4 // indirect
	github.com/gofrs/uuid v4.4.0+incompatible // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/hpcloud/tail v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/labstack/echo/v4 v4.15.1
	github.com/labstack/gommon v0.4.2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/spdystream v0.5.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/common v0.67.5
	github.com/prometheus/procfs v0.19.2 // indirect
	github.com/sirupsen/logrus v1.9.4
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	github.com/vishvananda/netlink v1.3.2-0.20260109214200-c6faf428e8f8
	github.com/vishvananda/netns v0.0.5
	go.opencensus.io v0.24.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.50.0
	golang.org/x/net v0.53.0
	golang.org/x/oauth2 v0.34.0 // indirect
	golang.org/x/term v0.42.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	golang.org/x/time v0.15.0
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	gomodules.xyz/jsonpatch/v2 v2.4.0 // indirect
	gopkg.in/fsnotify.v1 v1.4.7 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/kube-openapi v0.0.0-20250910181357-589584f1c912 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
)

require (
	github.com/Azure/azure-container-networking/zapai v0.0.3
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4 v4.9.0-beta.1
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dashboard/armdashboard v1.2.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/monitor/armmonitor v0.11.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v5 v5.2.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources v1.2.0
	github.com/cilium/cilium v1.19.3
	github.com/cilium/ebpf v0.20.1-0.20260218191617-ee67e7f43dd9
	github.com/jsternberg/zap-logfmt v1.3.0
	golang.org/x/sync v0.20.0
	gotest.tools/v3 v3.5.2
	k8s.io/kubectl v0.35.3
	sigs.k8s.io/yaml v1.6.0
)

require (
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cilium/hive v0.0.0-20260108104938-97756f6ff54c // indirect
	github.com/cilium/proxy v0.0.0-20250623105955-2136f59a4ea1 // indirect
	github.com/cilium/statedb v0.5.6 // indirect
	github.com/cilium/stream v0.0.1 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/typeurl/v2 v2.2.3 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/analysis v0.24.1 // indirect
	github.com/go-openapi/errors v0.22.6 // indirect
	github.com/go-openapi/loads v0.23.2 // indirect
	github.com/go-openapi/runtime v0.29.2 // indirect
	github.com/go-openapi/spec v0.22.3 // indirect
	github.com/go-openapi/strfmt v0.25.0 // indirect
	github.com/go-openapi/swag/cmdutils v0.25.4 // indirect
	github.com/go-openapi/swag/conv v0.25.4 // indirect
	github.com/go-openapi/swag/fileutils v0.25.4 // indirect
	github.com/go-openapi/swag/jsonname v0.25.4 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.4 // indirect
	github.com/go-openapi/swag/loading v0.25.4 // indirect
	github.com/go-openapi/swag/mangling v0.25.4 // indirect
	github.com/go-openapi/swag/netutils v0.25.4 // indirect
	github.com/go-openapi/swag/stringutils v0.25.4 // indirect
	github.com/go-openapi/swag/typeutils v0.25.4 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.4 // indirect
	github.com/go-openapi/validate v0.25.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/mackerelio/go-osstat v0.2.6 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/petermattis/goid v0.0.0-20250813065127-a731cc31b4fe // indirect
	github.com/sasha-s/go-deadlock v0.3.6 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.mongodb.org/mongo-driver v1.17.6 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.41.0 // indirect
	go.opentelemetry.io/otel/metric v1.41.0 // indirect
	go.opentelemetry.io/otel/sdk v1.40.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.40.0 // indirect
	go.opentelemetry.io/otel/trace v1.41.0 // indirect
	go.uber.org/dig v1.17.1 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	go4.org/netipx v0.0.0-20231129151722-fdeea329fbba // indirect
	golang.org/x/tools v0.43.0 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.13.0 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.2-0.20260122202528-d9cc6641c482 // indirect
)

require (
	github.com/containerd/cgroups/v3 v3.0.5 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/emicklei/go-restful/v3 v3.13.0 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	k8s.io/kubelet v0.35.4
)

replace (
	github.com/onsi/ginkgo => github.com/onsi/ginkgo v1.12.0
	github.com/onsi/gomega => github.com/onsi/gomega v1.10.0
)

retract (
	v1.16.17 // contains only retractions, new version to retract 1.15.22.
	v1.16.16 // contains only retractions, has to be newer than 1.16.15.
	v1.16.15 // typo in the version number.
	v1.15.22 // typo in the version number.
)
