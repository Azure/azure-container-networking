package dataplane

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/util"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"

	"github.com/zcalusic/sysinfo"
	"k8s.io/klog"
)

const detectingErrMsg = "failed to detect iptables version. failed to find KUBE chains in iptables-legacy-save and iptables-nft-save and failed to get kernel version. NPM will crash to retry"

var errDetectingIptablesVersion = errors.New(detectingErrMsg)

func (dp *DataPlane) getEndpointsToApplyPolicies(_ []*policies.NPMNetworkPolicy) (map[string]string, error) {
	// NOOP in Linux
	return nil, nil
}

func (dp *DataPlane) shouldUpdatePod() bool {
	return false
}

func (dp *DataPlane) updatePod(pod *updateNPMPod) error {
	// NOOP in Linux
	return nil
}

func (dp *DataPlane) bootupDataPlane() error {
	if err := detectIptablesVersion(dp.ioShim); err != nil {
		return npmerrors.ErrorWrapper(npmerrors.BootupDataplane, false, "failed to detect iptables version", err)
	}

	// It is important to keep order to clean-up ACLs before ipsets. Otherwise we won't be able to delete ipsets referenced by ACLs
	if err := dp.policyMgr.Bootup(nil); err != nil {
		return npmerrors.ErrorWrapper(npmerrors.BootupDataplane, false, "failed to reset policy dataplane", err)
	}
	if err := dp.ipsetMgr.ResetIPSets(); err != nil {
		return npmerrors.ErrorWrapper(npmerrors.BootupDataplane, false, "failed to reset ipsets dataplane", err)
	}
	return nil
}

func (dp *DataPlane) refreshPodEndpoints() error {
	// NOOP in Linux
	return nil
}

// detectIptablesVersion sets the global iptables variable to nft if detected or legacy if detected.
// NPM will crash if it fails to detect either.
// This global variable is referenced in all iptables related functions.
func detectIptablesVersion(ioShim *common.IOShim) error {
	klog.Info("first attempt detecting iptables version. running: iptables-nft-save -t mangle")
	cmd := ioShim.Exec.Command(util.IptablesSaveNft, "-t", "mangle")
	output, err := cmd.CombinedOutput()
	if err == nil && strings.Contains(string(output), "KUBE-IPTABLES-HINT") || strings.Contains(string(output), "KUBE-KUBELET-CANARY") {
		msg := "detected iptables version on first attempt. found KUBE chains in nft tables. NPM will use iptables-nft"
		klog.Info(msg)
		metrics.SendLog(util.DaemonDataplaneID, msg, metrics.DonotPrint)
		util.Iptables = util.IptablesNft
		util.IptablesSave = util.IptablesSaveNft
		util.IptablesRestore = util.IptablesRestoreNft
		return nil
	}

	if err != nil {
		msg := fmt.Sprintf("failed to detect iptables version on first attempt. error running iptables-nft-save. will try detecting using iptables-legacy-save. err: %w", err)
		klog.Info(msg)
		metrics.SendErrorLogAndMetric(util.DaemonDataplaneID, msg)
	}

	klog.Info("second attempt detecting iptables version. running: iptables-legacy-save -t mangle")
	lCmd := ioShim.Exec.Command(util.IptablesSaveLegacy, "-t", "mangle")
	loutput, err := lCmd.CombinedOutput()
	if err == nil && strings.Contains(string(loutput), "KUBE-IPTABLES-HINT") || strings.Contains(string(loutput), "KUBE-KUBELET-CANARY") {
		msg := "detected iptables version on second attempt. found KUBE chains in legacy tables. NPM will use iptables-legacy"
		klog.Info(msg)
		metrics.SendLog(util.DaemonDataplaneID, msg, metrics.DonotPrint)
		util.Iptables = util.IptablesLegacy
		util.IptablesSave = util.IptablesSaveLegacy
		util.IptablesRestore = util.IptablesRestoreLegacy
		return nil
	}

	if err != nil {
		msg := fmt.Sprintf("failed to detect iptables version on second attempt. error running iptables-legacy-save. will try detecting using kernel version. err: %w", err)
		klog.Info(msg)
		metrics.SendErrorLogAndMetric(util.DaemonDataplaneID, msg)
	}

	klog.Info("third attempt detecting iptables version. getting kernel version")
	var si sysinfo.SysInfo
	si.GetSysInfo()
	kernelVersion := strings.Split(si.Kernel.Release, ".")
	if kernelVersion[0] == "" {
		msg := fmt.Sprintf("failed to detect iptables version on third attempt. error getting kernel version. err: %w", err)
		klog.Info(msg)
		metrics.SendErrorLogAndMetric(util.DaemonDataplaneID, msg)
		return errDetectingIptablesVersion
	}

	majorVersion, err := strconv.Atoi(kernelVersion[0])
	if err != nil {
		msg := fmt.Sprintf("failed to detect iptables version on third attempt. error converting kernel version to int. err: %w", err)
		klog.Info(msg)
		metrics.SendErrorLogAndMetric(util.DaemonDataplaneID, msg)
		return errDetectingIptablesVersion
	}

	if majorVersion >= 5 {
		msg := "detected iptables version on third attempt. found kernel version >= 5. NPM will use iptables-nft"
		klog.Info(msg)
		metrics.SendLog(util.DaemonDataplaneID, msg, metrics.DonotPrint)
		util.Iptables = util.IptablesNft
		util.IptablesSave = util.IptablesSaveNft
		util.IptablesRestore = util.IptablesRestoreNft
		return nil
	}

	msg := "detected iptables version on third attempt. found kernel version < 5. NPM will use iptables-legacy"
	klog.Info(msg)
	metrics.SendLog(util.DaemonDataplaneID, msg, metrics.DonotPrint)

	return nil
}
