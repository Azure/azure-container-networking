package v1alpha1

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	testEnv   *envtest.Environment
	k8sClient client.Client
)

func TestMain(m *testing.M) {
	// Check if KUBEBUILDER_ASSETS is set
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		fmt.Println("SKIP: KUBEBUILDER_ASSETS not set. Run with:")
		fmt.Println(`  KUBEBUILDER_ASSETS="$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@latest use -p path)" go test -v ./...`)
		os.Exit(0)
	}

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "manifests")},
	}

	cfg, err := testEnv.Start()
	if err != nil {
		panic(err)
	}

	scheme := runtime.NewScheme()
	_ = AddToScheme(scheme)

	k8sClient, _ = client.New(cfg, client.Options{Scheme: scheme})

	code := m.Run()
	_ = testEnv.Stop()
	os.Exit(code)
}

func TestPodNetworkInstance_SpecValidation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		pni         PodNetworkInstance
		expectError bool
	}{
		// No IPConstraint tests
		{
			name: "no IPConstraint with podIPReservationSize 0 (dynamic) - create allowed",
			pni: PodNetworkInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-no-ip-size0", Namespace: "default"},
				Spec: PodNetworkInstanceSpec{
					PodNetworkConfigs: []PodNetworkConfig{
						{PodNetwork: "net1", PodIPReservationSize: 0},
					},
				},
			},
			expectError: false,
		},
		{
			name: "no IPConstraint with podIPReservationSize 1 (static) - create allowed",
			pni: PodNetworkInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-no-ip-size1", Namespace: "default"},
				Spec: PodNetworkInstanceSpec{
					PodNetworkConfigs: []PodNetworkConfig{
						{PodNetwork: "net1", PodIPReservationSize: 1},
					},
				},
			},
			expectError: false,
		},
		{
			name: "no IPConstraint with podIPReservationSize 2 (static) - create allowed",
			pni: PodNetworkInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-no-ip-size2", Namespace: "default"},
				Spec: PodNetworkInstanceSpec{
					PodNetworkConfigs: []PodNetworkConfig{
						{PodNetwork: "net1", PodIPReservationSize: 2},
					},
				},
			},
			expectError: false,
		},
		// IPConstraint blank (empty string) tests
		{
			name: "IPConstraint blank with podIPReservationSize 0 (dynamic) - create allowed",
			pni: PodNetworkInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-blank-ip-size0", Namespace: "default"},
				Spec: PodNetworkInstanceSpec{
					PodNetworkConfigs: []PodNetworkConfig{
						{PodNetwork: "net1", PodIPReservationSize: 0, IPConstraint: ""},
					},
				},
			},
			expectError: false,
		},
		{
			name: "IPConstraint blank with podIPReservationSize 1 (static) - create allowed",
			pni: PodNetworkInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-blank-ip-size1", Namespace: "default"},
				Spec: PodNetworkInstanceSpec{
					PodNetworkConfigs: []PodNetworkConfig{
						{PodNetwork: "net1", PodIPReservationSize: 1, IPConstraint: ""},
					},
				},
			},
			expectError: false,
		},
		{
			name: "IPConstraint blank with podIPReservationSize 2 (static) - create allowed",
			pni: PodNetworkInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-blank-ip-size2", Namespace: "default"},
				Spec: PodNetworkInstanceSpec{
					PodNetworkConfigs: []PodNetworkConfig{
						{PodNetwork: "net1", PodIPReservationSize: 2, IPConstraint: ""},
					},
				},
			},
			expectError: false,
		},
		// IPConstraint with valid IP and various reservation sizes
		{
			name: "IPConstraint with podIPReservationSize 0 (dynamic ipconstraint) - not allowed",
			pni: PodNetworkInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ip-size0", Namespace: "default"},
				Spec: PodNetworkInstanceSpec{
					PodNetworkConfigs: []PodNetworkConfig{
						{PodNetwork: "net1", PodIPReservationSize: 0, IPConstraint: "198.176.10.1"},
					},
				},
			},
			expectError: true, // ipConstraint only allowed when podIPReservationSize is 1
		},
		{
			name: "IPConstraint with podIPReservationSize 1 (static ipconstraint) - create allowed",
			pni: PodNetworkInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ip-size1", Namespace: "default"},
				Spec: PodNetworkInstanceSpec{
					PodNetworkConfigs: []PodNetworkConfig{
						{PodNetwork: "net1", PodIPReservationSize: 1, IPConstraint: "198.176.10.1"},
					},
				},
			},
			expectError: false,
		},
		{
			name: "IPConstraint with podIPReservationSize 2 (static ipconstraint > 1 reservation) - not allowed",
			pni: PodNetworkInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ip-size2", Namespace: "default"},
				Spec: PodNetworkInstanceSpec{
					PodNetworkConfigs: []PodNetworkConfig{
						{PodNetwork: "net1", PodIPReservationSize: 2, IPConstraint: "198.176.10.1"},
					},
				},
			},
			expectError: true, // ipConstraint only allowed when podIPReservationSize is 1
		},
		// IPConstraint with CIDR prefix tests
		{
			name: "IPConstraint with /32 prefix - create allowed",
			pni: PodNetworkInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ip-prefix32", Namespace: "default"},
				Spec: PodNetworkInstanceSpec{
					PodNetworkConfigs: []PodNetworkConfig{
						{PodNetwork: "net1", PodIPReservationSize: 1, IPConstraint: "198.176.110.112/32"},
					},
				},
			},
			expectError: false,
		},
		{
			name: "IPConstraint with /24 prefix - not allowed",
			pni: PodNetworkInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ip-prefix24", Namespace: "default"},
				Spec: PodNetworkInstanceSpec{
					PodNetworkConfigs: []PodNetworkConfig{
						{PodNetwork: "net1", PodIPReservationSize: 1, IPConstraint: "10.10.10.0/24"},
					},
				},
			},
			expectError: true, // Pattern only allows /32
		},
		// IPv6 tests - not supported
		{
			name: "IPConstraint with IPv6 address - not allowed",
			pni: PodNetworkInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ipv6", Namespace: "default"},
				Spec: PodNetworkInstanceSpec{
					PodNetworkConfigs: []PodNetworkConfig{
						{PodNetwork: "net1", PodIPReservationSize: 1, IPConstraint: "2001:db8:1234::/32"},
					},
				},
			},
			expectError: true, // Pattern only matches IPv4
		},
		// Invalid IP format tests
		{
			name: "IPConstraint with invalid IP format - not allowed",
			pni: PodNetworkInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-invalid-ip", Namespace: "default"},
				Spec: PodNetworkInstanceSpec{
					PodNetworkConfigs: []PodNetworkConfig{
						{PodNetwork: "net1", PodIPReservationSize: 1, IPConstraint: "invalid-ip"},
					},
				},
			},
			expectError: true, // Pattern validation fails
		},
		{
			name: "IPConstraint with IP octet > 255 - not allowed",
			pni: PodNetworkInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ip-octet-overflow", Namespace: "default"},
				Spec: PodNetworkInstanceSpec{
					PodNetworkConfigs: []PodNetworkConfig{
						{PodNetwork: "net1", PodIPReservationSize: 1, IPConstraint: "256.176.10.1"},
					},
				},
			},
			expectError: true, // Pattern validation fails
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := k8sClient.Create(ctx, &tt.pni)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				deleteErr := k8sClient.Delete(ctx, &tt.pni) // cleanup
				require.NoError(t, deleteErr, "failed to delete PNI")
			}
		})
	}
}
