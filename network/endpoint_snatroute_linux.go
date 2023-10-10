package network

import (
	"fmt"

	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/network/snat"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

func GetSnatHostIfName(epInfo *EndpointInfo) string {
	return fmt.Sprintf("%s%s", snatVethInterfacePrefix, epInfo.Id[:7])
}

func GetSnatContIfName(epInfo *EndpointInfo) string {
	return fmt.Sprintf("%s%s-2", snatVethInterfacePrefix, epInfo.Id[:7])
}

func AddSnatEndpoint(snatClient *snat.Client) error {
	if err := snatClient.CreateSnatEndpoint(); err != nil {
		return errors.Wrap(err, "failed to add snat endpoint")
	}
	return nil
}

func AddSnatEndpointRules(snatClient *snat.Client, hostToNC, ncToHost bool, nl netlink.NetlinkInterface, plc platform.ExecClient) error {
	// Allow specific Private IPs via Snat Bridge
	if err := snatClient.AllowIPAddressesOnSnatBridge(); err != nil {
		return errors.Wrap(err, "failed to allow ip addresses on snat bridge")
	}

	// Block Private IPs via Snat Bridge
	if err := snatClient.BlockIPAddressesOnSnatBridge(); err != nil {
		return errors.Wrap(err, "failed to block ip addresses on snat bridge")
	}
	nuc := networkutils.NewNetworkUtils(nl, plc)
	if err := nuc.EnableIPForwarding(snat.SnatBridgeName); err != nil {
		return errors.Wrap(err, "failed to enable ip forwarding")
	}

	if hostToNC {
		if err := snatClient.AllowInboundFromHostToNC(); err != nil {
			return errors.Wrap(err, "failed to allow inbound from host to nc")
		}
	}

	if ncToHost {
		if err := snatClient.AllowInboundFromNCToHost(); err != nil {
			return errors.Wrap(err, "failed to allow inbound from nc to host")
		}
	}
	return nil
}

func MoveSnatEndpointToContainerNS(snatClient *snat.Client, netnsPath string, nsID uintptr) error {
	if err := snatClient.MoveSnatEndpointToContainerNS(netnsPath, nsID); err != nil {
		if delErr := snatClient.DeleteSnatEndpoint(); delErr != nil {
			logger.Error("failed to delete snat endpoint on error(moving to container ns)", zap.Error(delErr))
		}
		return errors.Wrap(err, "failed to move snat endpoint to container ns. deleted snat endpoint")
	}
	return nil
}

func SetupSnatContainerInterface(snatClient *snat.Client) error {
	if err := snatClient.SetupSnatContainerInterface(); err != nil {
		return errors.Wrap(err, "failed to setup snat container interface")
	}
	return nil
}

func ConfigureSnatContainerInterface(snatClient *snat.Client) error {
	if err := snatClient.ConfigureSnatContainerInterface(); err != nil {
		return errors.Wrap(err, "failed to configure snat container interface")
	}
	return nil
}

func DeleteSnatEndpoint(snatClient *snat.Client) error {
	if err := snatClient.DeleteSnatEndpoint(); err != nil {
		return errors.Wrap(err, "failed to delete snat endpoint")
	}
	return nil
}

func DeleteSnatEndpointRules(snatClient *snat.Client, hostToNC, ncToHost bool) {
	if hostToNC {
		err := snatClient.DeleteInboundFromHostToNC()
		if err != nil {
			logger.Error("failed to delete inbound from host to nc rules", zap.Error(err))
		}
	}

	if ncToHost {
		err := snatClient.DeleteInboundFromNCToHost()
		if err != nil {
			logger.Error("failed to delete inbound from nc to host rules", zap.Error(err))
		}
	}
}
