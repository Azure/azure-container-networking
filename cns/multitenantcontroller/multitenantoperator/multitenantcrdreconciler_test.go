package multitenantoperator

import (
	"fmt"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/multitenantcontroller/mockclients"
	ncapi "github.com/Azure/azure-container-networking/crds/multitenantnetworkcontainer/api/v1alpha1"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("multiTenantCrdReconciler", func() {
	var kubeClient *mockclients.MockClient
	var cnsClient *mockclients.MockAPIClient
	var mockCtl *gomock.Controller
	var reconciler *multiTenantCrdReconciler
	var mockNodeName = "mockNodeName"
	var namespacedName = types.NamespacedName{
		Namespace: "test",
		Name:      "test",
	}

	BeforeEach(func() {
		logger.InitLogger("multiTenantCrdReconciler", 0, 0, "")
		mockCtl = gomock.NewController(GinkgoT())
		kubeClient = mockclients.NewMockClient(mockCtl)
		cnsClient = mockclients.NewMockAPIClient(mockCtl)
		reconciler = &multiTenantCrdReconciler{
			KubeClient: kubeClient,
			NodeName:   mockNodeName,
			CNSClient:  cnsClient,
		}
	})

	Context("lifecycle", func() {

		It("Should succeed when the NC has already been deleted", func() {
			expectedError := &apierrors.StatusError{
				ErrStatus: metav1.Status{
					Reason: metav1.StatusReasonNotFound,
				},
			}
			kubeClient.EXPECT().Get(gomock.Any(), namespacedName, gomock.Any()).Return(expectedError)
			_, err := reconciler.Reconcile(reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(BeNil())
		})

		It("Should fail when the kube client reports failure", func() {
			expectedError := &apierrors.StatusError{
				ErrStatus: metav1.Status{
					Reason: metav1.StatusReasonInternalError,
				},
			}
			kubeClient.EXPECT().Get(gomock.Any(), namespacedName, gomock.Any()).Return(expectedError)
			_, err := reconciler.Reconcile(reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(BeNil())
			Expect(err).To(Equal(expectedError))
		})

		It("Should succeed when the NC is in Terminated state", func() {
			var nc ncapi.MultiTenantNetworkContainer = ncapi.MultiTenantNetworkContainer{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &metav1.Time{},
				},
				Status: ncapi.MultiTenantNetworkContainerStatus{
					State: "Terminated",
				},
			}
			kubeClient.EXPECT().Get(gomock.Any(), namespacedName, gomock.Any()).SetArg(2, nc)
			_, err := reconciler.Reconcile(reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(BeNil())
		})

		It("Should succeed when the NC is not in Initialized state", func() {
			var nc ncapi.MultiTenantNetworkContainer = ncapi.MultiTenantNetworkContainer{
				Status: ncapi.MultiTenantNetworkContainerStatus{
					State: "Pending",
				},
			}
			kubeClient.EXPECT().Get(gomock.Any(), namespacedName, gomock.Any()).SetArg(2, nc)
			_, err := reconciler.Reconcile(reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(BeNil())
		})

		It("Should succeed when the NC is in Initialized state and it has already been persisted in CNS", func() {
			var uuid = "uuid"
			var nc ncapi.MultiTenantNetworkContainer = ncapi.MultiTenantNetworkContainer{
				Spec: ncapi.MultiTenantNetworkContainerSpec{
					UUID: uuid,
				},
				Status: ncapi.MultiTenantNetworkContainerStatus{
					State: "Initialized",
				},
			}
			kubeClient.EXPECT().Get(gomock.Any(), namespacedName, gomock.Any()).SetArg(2, nc)
			cnsClient.EXPECT().GetNC(cns.GetNetworkContainerRequest{NetworkContainerid: uuid}).Return(cns.GetNetworkContainerResponse{}, nil)
			_, err := reconciler.Reconcile(reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(BeNil())
		})

		It("Should fail when the NC subnet isn't in correct format", func() {
			var uuid = "uuid"
			var nc ncapi.MultiTenantNetworkContainer = ncapi.MultiTenantNetworkContainer{
				Spec: ncapi.MultiTenantNetworkContainerSpec{
					UUID: uuid,
				},
				Status: ncapi.MultiTenantNetworkContainerStatus{
					State:    "Initialized",
					IPSubnet: "1.2.3.4.5",
				},
			}
			kubeClient.EXPECT().Get(gomock.Any(), namespacedName, gomock.Any()).SetArg(2, nc)
			cnsClient.EXPECT().GetNC(cns.GetNetworkContainerRequest{NetworkContainerid: uuid}).Return(cns.GetNetworkContainerResponse{}, fmt.Errorf("NotFound"))
			_, err := reconciler.Reconcile(reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring("invalid CIDR address"))
		})

		It("Should succeed when the NC subnet is in correct format", func() {
			var uuid = "uuid"
			var nc ncapi.MultiTenantNetworkContainer = ncapi.MultiTenantNetworkContainer{
				Spec: ncapi.MultiTenantNetworkContainerSpec{
					UUID: uuid,
				},
				Status: ncapi.MultiTenantNetworkContainerStatus{
					State:    "Initialized",
					IPSubnet: "1.2.3.0/24",
				},
			}
			kubeClient.EXPECT().Get(gomock.Any(), namespacedName, gomock.Any()).SetArg(2, nc)
			statusWriter := mockclients.NewMockStatusWriter(mockCtl)
			statusWriter.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)
			kubeClient.EXPECT().Status().Return(statusWriter)
			cnsClient.EXPECT().GetNC(cns.GetNetworkContainerRequest{NetworkContainerid: uuid}).Return(cns.GetNetworkContainerResponse{}, fmt.Errorf("NotFound"))
			cnsClient.EXPECT().CreateOrUpdateNC(gomock.Any()).Return(nil)
			_, err := reconciler.Reconcile(reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(BeNil())
		})
	})
})
