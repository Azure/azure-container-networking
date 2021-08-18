module github.com/Azure/azure-container-networking

go 1.16

require (
	code.cloudfoundry.org/clock v1.0.0 // indirect
	github.com/Masterminds/semver v1.5.0
	github.com/Microsoft/go-winio v0.4.15-0.20190919025122-fc70bd9a86b5
	github.com/Microsoft/hcsshim v0.8.10-0.20200506181021-222e9efadbe0
	github.com/billgraziano/dpapi v0.3.0
	github.com/containernetworking/cni v0.7.0-rc2
	github.com/daixiang0/gci v0.2.9 // indirect
	github.com/docker/docker v0.7.3-0.20190327010347-be7ac8be2ae0 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/libnetwork v0.8.0-dev.2.0.20210525090646-64b7a4574d14
	github.com/go-critic/go-critic v0.5.7 // indirect
	github.com/go-toolsmith/pkgload v1.0.1 // indirect
	github.com/golang/mock v1.5.0
	github.com/golang/protobuf v1.5.2
	github.com/google/go-cmp v0.5.5
	github.com/google/uuid v1.1.2
	github.com/gorilla/handlers v1.5.1 // indirect
	github.com/gorilla/mux v1.8.0
	github.com/hashicorp/go-version v1.3.0
	github.com/ishidawataru/sctp v0.0.0-20210226210310-f2269e66cdee // indirect
	github.com/logrusorgru/aurora v2.0.3+incompatible // indirect
	github.com/microsoft/ApplicationInsights-Go v0.4.3
	github.com/nxadm/tail v1.4.8
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.14.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/client_model v0.2.0
	github.com/quasilyte/regex/syntax v0.0.0-20200805063351-8f842688393c // indirect
	github.com/rs/cors v1.8.0
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/vishvananda/netlink v1.1.0 // indirect
	github.com/vishvananda/netns v0.0.0-20210104183010-2eb08e3e575f // indirect
	golang.org/x/net v0.0.0-20210510120150-4163338589ed // indirect
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c
	golang.org/x/term v0.0.0-20210503060354-a79de5458b56 // indirect
	golang.org/x/tools v0.1.5 // indirect
	google.golang.org/grpc v1.38.0
	google.golang.org/protobuf v1.26.0
	k8s.io/api v0.21.3
	k8s.io/apiextensions-apiserver v0.21.3
	k8s.io/apimachinery v0.21.3
	k8s.io/client-go v0.21.3
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20210722164352-7f3ee0f31471
	mvdan.cc/gofumpt v0.1.1 // indirect
	sigs.k8s.io/controller-runtime v0.9.5
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/onsi/ginkgo => github.com/onsi/ginkgo v1.12.0
	github.com/onsi/gomega => github.com/onsi/gomega v1.10.0
)
