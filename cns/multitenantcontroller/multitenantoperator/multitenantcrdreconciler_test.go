package multitenantoperator

import (
	"context"
	"encoding/json"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/multitenantcontroller/mockclients"
	cnstypes "github.com/Azure/azure-container-networking/cns/types"
	ncapi "github.com/Azure/azure-container-networking/crd/multitenantnetworkcontainer/api/v1alpha1"
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
	var cnsRestService *mockclients.MockcnsRESTservice
	var mockCtl *gomock.Controller
	var reconciler *multiTenantCrdReconciler
	const uuidValue = "uuid"
	mockNodeName := "mockNodeName"
	namespacedName := types.NamespacedName{
		Namespace: "test",
		Name:      "test",
	}
	podInfo := cns.KubernetesPodInfo{
		PodName:      namespacedName.Name,
		PodNamespace: namespacedName.Namespace,
	}

	BeforeEach(func() {
		logger.InitLogger("multiTenantCrdReconciler", 0, 0, "")
		mockCtl = gomock.NewController(GinkgoT())
		kubeClient = mockclients.NewMockClient(mockCtl)
		cnsRestService = mockclients.NewMockcnsRESTservice(mockCtl)
		reconciler = &multiTenantCrdReconciler{
			KubeClient:     kubeClient,
			NodeName:       mockNodeName,
			CNSRestService: cnsRestService,
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
			_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{
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
			_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{
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
			_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{
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
			_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(BeNil())
		})

		It("Should succeed when the NC is in Initialized state and it has already been persisted in CNS", func() {
			uuid := uuidValue
			var nc ncapi.MultiTenantNetworkContainer = ncapi.MultiTenantNetworkContainer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedName.Name,
					Namespace: namespacedName.Namespace,
				},
				Spec: ncapi.MultiTenantNetworkContainerSpec{
					UUID: uuid,
				},
				Status: ncapi.MultiTenantNetworkContainerStatus{
					State: "Initialized",
					MultiTenantInfo: ncapi.MultiTenantInfo{
						EncapType: "Vlan",
						ID:        1,
					},
				},
			}

			orchestratorContext, err := json.Marshal(podInfo)
			Expect(err).To(BeNil())

			kubeClient.EXPECT().Get(gomock.Any(), namespacedName, gomock.Any()).SetArg(2, nc)
			cnsRestService.EXPECT().GetNetworkContainerInternal(cns.GetNetworkContainerRequest{
				NetworkContainerid:  uuid,
				OrchestratorContext: orchestratorContext,
			}).Return(cns.GetNetworkContainerResponse{}, cnstypes.Success)
			_, err = reconciler.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(BeNil())
		})

		It("Should fail when the NC subnet isn't in correct format", func() {
			uuid := uuidValue
			var nc ncapi.MultiTenantNetworkContainer = ncapi.MultiTenantNetworkContainer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedName.Name,
					Namespace: namespacedName.Namespace,
				},
				Spec: ncapi.MultiTenantNetworkContainerSpec{
					UUID: uuid,
				},
				Status: ncapi.MultiTenantNetworkContainerStatus{
					State:    "Initialized",
					IPSubnet: "1.2.3.4.5",
					MultiTenantInfo: ncapi.MultiTenantInfo{
						EncapType: "Vlan",
						ID:        1,
					},
				},
			}

			orchestratorContext, err := json.Marshal(podInfo)
			Expect(err).To(BeNil())

			kubeClient.EXPECT().Get(gomock.Any(), namespacedName, gomock.Any()).SetArg(2, nc)
			cnsRestService.EXPECT().GetNetworkContainerInternal(cns.GetNetworkContainerRequest{
				NetworkContainerid:  uuid,
				OrchestratorContext: orchestratorContext,
			}).Return(cns.GetNetworkContainerResponse{}, cnstypes.UnknownContainerID)
			_, err = reconciler.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring("UnknownContainerID"))
		})

		It("Should succeed when the NC subnet is in correct format", func() {
			uuid := uuidValue
			var nc ncapi.MultiTenantNetworkContainer = ncapi.MultiTenantNetworkContainer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedName.Name,
					Namespace: namespacedName.Namespace,
				},
				Spec: ncapi.MultiTenantNetworkContainerSpec{
					UUID: uuid,
				},
				Status: ncapi.MultiTenantNetworkContainerStatus{
					State:    "Initialized",
					IPSubnet: "1.2.3.0/24",
					MultiTenantInfo: ncapi.MultiTenantInfo{
						EncapType: "Vlan",
						ID:        1,
					},
				},
			}

			orchestratorContext, err := json.Marshal(cns.KubernetesPodInfo{
				PodName:      namespacedName.Name,
				PodNamespace: namespacedName.Namespace,
			})
			Expect(err).To(BeNil())
			networkContainerRequest := cns.CreateNetworkContainerRequest{
				NetworkContainerid:   nc.Spec.UUID,
				NetworkContainerType: cns.Kubernetes,
				OrchestratorContext:  orchestratorContext,
				Version:              "0",
				IPConfiguration: cns.IPConfiguration{
					IPSubnet: cns.IPSubnet{
						IPAddress:    nc.Status.IP,
						PrefixLength: uint8(24),
					},
					GatewayIPAddress: nc.Status.Gateway,
				},
				MultiTenancyInfo: cns.MultiTenancyInfo{
					EncapType: "Vlan",
					ID:        1,
				},
			}

			kubeClient.EXPECT().Get(gomock.Any(), namespacedName, gomock.Any()).SetArg(2, nc)
			statusWriter := mockclients.NewMockStatusWriter(mockCtl)
			statusWriter.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)
			kubeClient.EXPECT().Status().Return(statusWriter)
			cnsRestService.EXPECT().GetNetworkContainerInternal(cns.GetNetworkContainerRequest{
				NetworkContainerid:  uuid,
				OrchestratorContext: orchestratorContext,
			}).Return(cns.GetNetworkContainerResponse{}, cnstypes.Success)
			cnsRestService.EXPECT().CreateOrUpdateNetworkContainerInternal(networkContainerRequest).Return(cnstypes.Success)
			_, err = reconciler.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(BeNil())
		})
	})
})
