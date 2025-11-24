package longRunningCluster

import (
	"os"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestDatapathSuite(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Datapath Lifecycle Suite")
}

// Shared test configuration
type TestConfig struct {
	ResourceGroup   string
	PodImage        string
	StorageAccount1 string
	StorageAccount2 string
	Scenarios       []PodScenario
	VnetSubnetCache map[string]VnetSubnetInfo
	NodeNICUsage    map[string]*NodeNICState // Tracks NIC usage per node
}

// Suite-level variable accessible to all tests
var sharedConfig *TestConfig

var _ = ginkgo.SynchronizedBeforeSuite(func() []byte {
	// Runs once - initialize shared configuration from topology
	sharedConfig = &TestConfig{
		ResourceGroup:   os.Getenv("RG"),
		PodImage:        "nicolaka/netshoot:latest",
		StorageAccount1: os.Getenv("STORAGE_ACCOUNT_1"),
		StorageAccount2: os.Getenv("STORAGE_ACCOUNT_2"),
		Scenarios:       GetScenariosFromTopology(), // Generated from topology!
		VnetSubnetCache: make(map[string]VnetSubnetInfo),
		NodeNICUsage:    make(map[string]*NodeNICState),
	}
	return []byte{} // No data to share across processes
}, func(data []byte) {
	// Runs on all test processes
	if sharedConfig == nil {
		sharedConfig = &TestConfig{
			ResourceGroup:   os.Getenv("RG"),
			PodImage:        "nicolaka/netshoot:latest",
			StorageAccount1: os.Getenv("STORAGE_ACCOUNT_1"),
			StorageAccount2: os.Getenv("STORAGE_ACCOUNT_2"),
			Scenarios:       GetScenariosFromTopology(), // Generated from topology!
			VnetSubnetCache: make(map[string]VnetSubnetInfo),
			NodeNICUsage:    make(map[string]*NodeNICState),
		}
	}
})
