package multitenantoperator

import (
	"os"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
)

var _ = Describe("multiTenantController", func() {
	BeforeEach(func() {
		logger.InitLogger("multiTenantController", 0, 0, "")
	})

	Context("lifecycle", func() {
		restService := &restserver.HTTPRestService{}
		kubeconfig := &rest.Config{}

		It("Should exist with an error when nodeName is not set", func() {
			ctl, err := New(restService, kubeconfig)
			Expect(ctl).To(BeNil())
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(Equal("Must declare NODENAME environment variable."))
		})

		It("Should report an error when apiserver is not available", func() {
			val := os.Getenv(nodeNameEnvVar)
			os.Setenv(nodeNameEnvVar, "nodeName")
			ctl, err := New(nil, nil)
			os.Setenv(nodeNameEnvVar, val)
			Expect(ctl).To(BeNil())
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(Equal("must specify Config"))
		})
	})
})
