package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"os"
	"time"

	"github.com/Azure/azure-container-networking/ipam/ciliumpool"
	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	cniversion "github.com/containernetworking/cni/pkg/version"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

var version = "unknown"

func main() {
	if os.Getenv("CNI_COMMAND") != "" {
		wrap := func(command string) func(*skel.CmdArgs) error {
			return func(args *skel.CmdArgs) error { return cniCommand(context.Background(), command, args) }
		}
		skel.PluginMainFuncs(skel.CNIFuncs{Add: wrap("ADD"), Del: wrap("DEL"), Check: wrap("CHECK")},
			cniversion.PluginSupports("0.3.0", "0.3.1", "0.4.0", "1.0.0"), "Azure Cilium IPAM "+version)
		return
	}
	if err := adminCommand(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func environment(ctx context.Context, cfg ciliumpool.Config, supply bool) (ciliumpool.Guest, dynamic.Interface, error) {
	api, err := clientcmd.BuildConfigFromFlags("", cfg.Kubeconfig)
	if err != nil {
		return ciliumpool.Guest{}, nil, fmt.Errorf("loading node API credentials: %w", err)
	}
	api.Timeout = 10 * time.Second
	api.UserAgent = ciliumpool.PluginName + "/" + version
	client, err := dynamic.NewForConfig(api)
	if err != nil {
		return ciliumpool.Guest{}, nil, fmt.Errorf("creating Kubernetes client: %w", err)
	}
	guest, err := ciliumpool.Discover(ctx, cfg, supply)
	if err != nil {
		return ciliumpool.Guest{}, nil, fmt.Errorf("discovering guest: %w", err)
	}
	return guest, client, nil
}

func cniCommand(ctx context.Context, command string, args *skel.CmdArgs) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	nw, cfg, err := ciliumpool.ParseConfig(args.StdinData)
	if err != nil {
		return fmt.Errorf("loading IPAM configuration: %w", err)
	}
	owner := ciliumpool.Owner{Network: nw.Name, Sandbox: args.ContainerID, Interface: args.IfName}
	if command == "ADD" && nw.IPAM.Address != "" {
		return fmt.Errorf("%w: explicit ADD address is unsupported", ciliumpool.ErrConfig)
	}
	guest, client, err := environment(ctx, cfg, command != "DEL")
	if err != nil {
		return err
	}
	allocator, err := ciliumpool.NewAllocator(ctx, cfg, guest, client)
	if err != nil {
		return fmt.Errorf("binding IPAM allocator: %w", err)
	}
	switch command {
	case "ADD":
		ip, callErr := allocator.Add(ctx, owner)
		if callErr != nil {
			return fmt.Errorf("allocating address: %w", callErr)
		}
		result, callErr := allocationResult(ip, cfg.Subnet, cfg.Interface, nw.CNIVersion)
		if callErr != nil {
			return callErr
		}
		if printErr := cnitypes.PrintResult(result, nw.CNIVersion); printErr != nil {
			return fmt.Errorf("printing allocation result: %w", printErr)
		}
		return nil
	case "DEL":
		if err = allocator.Delete(ctx, owner, nw.IPAM.Address); err != nil {
			return fmt.Errorf("releasing address: %w", err)
		}
		return nil
	case "CHECK":
		if err = allocator.Check(ctx, owner); err != nil {
			return fmt.Errorf("checking reservation: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported CNI command %q", ciliumpool.ErrConfig, command)
	}
}

func allocationResult(address, subnet, hostInterface, version string) (*current.Result, error) {
	ip, err := netip.ParseAddr(address)
	if err != nil || !ip.Is4() {
		return nil, fmt.Errorf("%w: invalid allocated IPv4 address %q", ciliumpool.ErrOwnership, address)
	}
	prefix, err := netip.ParsePrefix(subnet)
	if err != nil || !prefix.Contains(ip) {
		return nil, fmt.Errorf("%w: allocated address is outside subnet %q", ciliumpool.ErrOwnership, subnet)
	}
	return &current.Result{CNIVersion: version, Interfaces: []*current.Interface{{Name: hostInterface}}, IPs: []*current.IPConfig{{
		Address: net.IPNet{IP: net.IP(ip.AsSlice()), Mask: net.CIDRMask(prefix.Bits(), 32)},
		Gateway: net.IP(prefix.Addr().Next().AsSlice()),
	}}}, nil
}

func adminCommand(args []string) error {
	flags := flag.NewFlagSet(ciliumpool.PluginName, flag.ContinueOnError)
	path := flags.String("config", "", "path to the deployed CNI configuration or conflist")
	register := flags.Bool("register", false, "register a clean or drained node explicitly")
	reconcile := flags.Bool("reconcile", false, "perform one bounded reconciliation")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parsing arguments: %w", err)
	}
	if *path == "" || *register == *reconcile || flags.NArg() != 0 {
		return fmt.Errorf("%w: provide --config and exactly one of --register or --reconcile", ciliumpool.ErrConfig)
	}
	data, err := os.ReadFile(*path)
	if err != nil {
		return fmt.Errorf("reading CNI configuration: %w", err)
	}
	_, cfg, err := ciliumpool.ParseConfig(data)
	if err != nil {
		return fmt.Errorf("loading IPAM configuration: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	guest, client, err := environment(ctx, cfg, true)
	if err != nil {
		return err
	}
	if *register {
		if err = ciliumpool.Provision(ctx, cfg, guest, client); err != nil {
			return fmt.Errorf("provisioning IPAM: %w", err)
		}
		return nil
	}
	allocator, err := ciliumpool.NewAllocator(ctx, cfg, guest, client)
	if err != nil {
		return fmt.Errorf("binding IPAM allocator: %w", err)
	}
	if err = allocator.Reconcile(ctx); err != nil {
		return fmt.Errorf("reconciling IPAM: %w", err)
	}
	return nil
}
