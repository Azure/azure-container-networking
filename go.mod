module github.com/Azure/azure-container-networking

go 1.17

require (
	code.cloudfoundry.org/clock v1.0.0 // indirect
	github.com/Masterminds/semver v1.5.0
	github.com/Microsoft/go-winio v0.4.17
	github.com/Microsoft/hcsshim v0.8.23
	github.com/billgraziano/dpapi v0.3.0
	github.com/containernetworking/cni v0.8.1
	github.com/docker/docker v20.10.8+incompatible // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/libnetwork v0.8.0-dev.2.0.20210525090646-64b7a4574d14
	github.com/golang/mock v1.6.0
	github.com/golang/protobuf v1.5.2
	github.com/google/go-cmp v0.5.6
	github.com/google/uuid v1.3.0
	github.com/gorilla/mux v1.8.0
	github.com/hashicorp/go-version v1.3.0
	github.com/ishidawataru/sctp v0.0.0-20210226210310-f2269e66cdee // indirect
	github.com/microsoft/ApplicationInsights-Go v0.4.4
	github.com/nxadm/tail v1.4.8
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.14.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/client_model v0.2.0
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.9.0
	github.com/stretchr/testify v1.7.0
	github.com/vishvananda/netns v0.0.0-20210104183010-2eb08e3e575f // indirect
	golang.org/x/sys v0.0.0-20210823070655-63515b42dcdf
	golang.org/x/term v0.0.0-20210503060354-a79de5458b56 // indirect
	google.golang.org/grpc v1.40.0
	google.golang.org/protobuf v1.27.1
	k8s.io/api v0.22.4
	k8s.io/apiextensions-apiserver v0.22.1
	k8s.io/apimachinery v0.22.4
	k8s.io/client-go v0.22.4
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20210819203725-bdf08cb9a70a
	sigs.k8s.io/controller-runtime v0.9.5
	sigs.k8s.io/yaml v1.2.0
)

require (
	github.com/avast/retry-go/v3 v3.1.1
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.1.1 // indirect
	github.com/containerd/cgroups v1.0.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/evanphx/json-patch v4.11.0+incompatible // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/go-logr/logr v0.4.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/googleapis/gnostic v0.5.5 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hpcloud/tail v1.0.0 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/json-iterator/go v1.1.11 // indirect
	github.com/magiconair/properties v1.8.5 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/mitchellh/mapstructure v1.4.2 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/pelletier/go-toml v1.9.4 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/common v0.26.0 // indirect
	github.com/prometheus/procfs v0.6.0 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/spf13/afero v1.6.0 // indirect
	github.com/spf13/cast v1.4.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/subosito/gotenv v1.2.0 // indirect
	go.opencensus.io v0.23.0 // indirect
	golang.org/x/net v0.0.0-20210825183410-e898025ed96a // indirect
	golang.org/x/oauth2 v0.0.0-20210819190943-2bc19b11175f // indirect
	golang.org/x/text v0.3.6 // indirect
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	gomodules.xyz/jsonpatch/v2 v2.2.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20210828152312-66f60bf46e71 // indirect
	gopkg.in/fsnotify.v1 v1.4.7 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.63.2 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
	k8s.io/component-base v0.22.1 // indirect
	k8s.io/klog/v2 v2.9.0
	k8s.io/kube-openapi v0.0.0-20211109043538-20434351676c // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.1.2 // indirect
)

require (
	github.com/fatih/structs v1.1.0
	github.com/gofrs/uuid v3.3.0+incompatible // indirect
	github.com/jsternberg/zap-logfmt v1.2.0
	go.uber.org/zap v1.18.1
)

require (
	github.com/gofrs/uuid v3.3.0+incompatible // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
)

replace (
	github.com/Microsoft/go-winio => github.com/microsoft/go-winio v0.4.17
	github.com/Microsoft/hcsshim => github.com/vakalapa/hcsshim v0.9.1-0.20211203205307-837d4d06df77
	github.com/onsi/ginkgo => github.com/onsi/ginkgo v1.12.0
	github.com/onsi/gomega => github.com/onsi/gomega v1.10.0
)
