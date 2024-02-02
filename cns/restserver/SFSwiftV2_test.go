package restserver

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	cnsclient "github.com/Azure/azure-container-networking/cns/client"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/types"
	"gotest.tools/v3/assert"
)

func TestValidateSFIPConfigsRequestSuccess(t *testing.T) {
	cnsClient, err := cnsclient.New("", 15*time.Second)
	if err != nil {
		logger.Errorf("Failed to init cnsclient, err:%v.\n", err)
		return
	}
	middleware := SFSWIFTv2Middleware{CnsClient: cnsClient}

	happyReq := &cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	b, _ := testPod1Info.OrchestratorContext()
	happyReq.OrchestratorContext = b
	happyReq.SecondaryInterfacesExist = false

	_, respCode, err1 := middleware.validateIPConfigsRequest(context.TODO(), happyReq)
	assert.Equal(t, err1, "")
	assert.Equal(t, respCode, types.Success)
	assert.Equal(t, happyReq.SecondaryInterfacesExist, true)
	assert.Equal(t, happyReq.AddInterfacesDataToResponse, true)
}

func TestValidateSFIPConfigsRequestFailure(t *testing.T) {
	cnsClient, err := cnsclient.New("", 15*time.Second)
	if err != nil {
		logger.Errorf("Failed to init cnsclient, err:%v.\n", err)
		return
	}
	middleware := SFSWIFTv2Middleware{CnsClient: cnsClient}
	// Fail to unmarshal pod info test
	failReq := &cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	failReq.OrchestratorContext = []byte("invalid")
	_, respCode, _ := middleware.validateIPConfigsRequest(context.TODO(), failReq)
	assert.Equal(t, respCode, types.UnexpectedError)
}
