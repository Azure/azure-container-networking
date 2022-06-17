package main

import (
	"context"

	"github.com/Azure/azure-container-networking/azure-ipam/internal/buildinfo"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/network"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	types100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// IPAMPlugin is the struct for the delegated azure-ipam plugin
type IPAMPlugin struct {
	Name      string
	Version   string
	Options   map[string]interface{}
	logger    *zap.Logger
	cnsClient cnsClient
}

type cnsClient interface {
	RequestIPAddress(context.Context, cns.IPConfigRequest) (*cns.IPConfigResponse, error)
	ReleaseIPAddress(context.Context, cns.IPConfigRequest) error
}

// NewPlugin constructs a new IPAM plugin
func NewPlugin(logger *zap.Logger, c cnsClient) (*IPAMPlugin, error) {
	plugin := &IPAMPlugin{
		Name:      pluginName,
		Version:   buildinfo.Version,
		logger:    logger,
		cnsClient: c,
	}
	return plugin, nil
}

//
// CNI implementation
// https://github.com/containernetworking/cni/blob/master/SPEC.md
//

// CmdAdd handles CNI add commands.
func (p *IPAMPlugin) CmdAdd(args *cniSkel.CmdArgs) error {
	p.logger.Info("ADD called")
	// Create CNS request from args
	req, err := createCNSRequest(args)
	if err != nil {
		p.logger.Error("Failed to create CNS IP config request",
			zap.Error(err),
		)
		return errors.Wrapf(err, "failed to create CNS IP config request")
	}
	p.logger.Info("Created CNS IP config request",
		zap.Any("request", req),
	)

	// cnsClient sets a request timeout.
	ctx := context.TODO()
	p.logger.Info("Making request to CNS")
	resp, err := p.cnsClient.RequestIPAddress(ctx, req)
	if err != nil {
		p.logger.Error("Failed to request IP address from CNS",
			zap.Error(err),
			zap.Any("request", req),
		)
		return errors.Wrapf(err, "failed to get IP address from CNS")
	}
	p.logger.Info("Received CNS IP config response",
		zap.Any("response", resp),
	)

	// Get Pod IP and gateway IP from CNS response
	podIPNet, gwIP, err := processCNSResponse(resp)
	if err != nil {
		p.logger.Error("Failed to interpret CNS IPConfigResponse",
			zap.Error(err),
			zap.Any("response", resp),
		)
		return errors.Wrapf(err, "failed to interpret CNS IPConfigResponse")
	}
	p.logger.Info("Parsed pod IP and gateway IP",
		zap.String("podIPNet", podIPNet.String()),
		zap.String("gwIP", gwIP.String()),
	)

	// Parsing network conf
	nwCfg, err := parseNetConf(args.StdinData)
	if err != nil {
		p.logger.Error("Failed to parse CNI network config",
			zap.Error(err),
			zap.Any("argStdinData", args.StdinData),
		)
		return errors.Wrapf(err, "failed to parse CNI network config")
	}
	p.logger.Info("Parsed network config",
		zap.Any("nwCfg", nwCfg),
	)

	cniResult := &types100.Result{
		IPs: []*types100.IPConfig{
			{
				Address: *podIPNet,
				Gateway: gwIP,
			},
		},
		Routes: []*cniTypes.Route{
			{
				Dst: network.Ipv4DefaultRouteDstPrefix,
				GW:  gwIP,
			},
		},
	}

	versionedCniResult, err := cniResult.GetAsVersion(nwCfg.CNIVersion)
	if err != nil {
		p.logger.Error("Failed to interpret CNI result with netconf CNI version",
			zap.Error(err),
			zap.Any("cniVersion", nwCfg.CNIVersion),
		)
		return errors.Wrapf(err, "failed to interpret CNI result as version %s", nwCfg.CNIVersion)
	}

	versionedCniResult.Print()

	return nil
}

// CmdDel handles CNI delete commands.
func (p *IPAMPlugin) CmdDel(args *cniSkel.CmdArgs) error {
	p.logger.Info("DEL called")
	// Create CNS request from args
	req, err := createCNSRequest(args)
	if err != nil {
		p.logger.Error("Failed to create CNS IP config request",
			zap.Error(err),
		)
		return errors.Wrapf(err, "failed to create CNS IP config request")
	}
	p.logger.Info("Created CNS IP config request",
		zap.Any("request", req),
	)
	ctx := context.TODO()
	p.logger.Info("Making request to CNS")
	if err := p.cnsClient.ReleaseIPAddress(ctx, req); err != nil {
		p.logger.Error("Failed to release IP address from CNS",
			zap.Error(err),
			zap.Any("request", req),
		)
		return cniTypes.NewError(cniTypes.ErrTryAgainLater, err.Error(), "")
	}

	return nil
}

// CmdCheck handles CNI check command - not implemented
func (p *IPAMPlugin) CmdCheck(args *cniSkel.CmdArgs) error {
	p.logger.Info("CHECK called")
	return nil
}
