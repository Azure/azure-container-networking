package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/cilium/ebpf"
	goiptables "github.com/coreos/go-iptables/iptables"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Version is populated by make during build.
var version string

var (
	configPath4                      = flag.String("input", "/etc/config/", "Name of the directory with the ipv4 allowed regex files")
	configPath6                      = flag.String("input6", "/etc/config6/", "Name of directory with the ipv6 allowed regex files")
	checkInterval                    = flag.Int("interval", 300, "How often to check for user iptables rules and bpf map increases (in seconds)")
	sendEvents                       = flag.Bool("events", false, "Whether to send node events if unexpected iptables rules are detected")
	ipv6Enabled                      = flag.Bool("ipv6", false, "Whether to check ip6tables using the ipv6 allowlists")
	checkMap                         = flag.Bool("checkMap", false, "Whether to check the bpf map at mapPath for increases")
	pinPath                          = flag.String("mapPath", "/azure-block-iptables-bpf-map/iptables_block_event_counter", "Path to pinned bpf map")
	terminateOnSuccess               = flag.Bool("terminateOnSuccess", false, "Whether to terminate the program when no user iptables rules found")
	installRoutesForHealthProbeReply = flag.Bool("installRoutesForHealthProbeReply", false, "Whether to install loopback routes for replies sent to kubelet health probes")
)

const (
	label          = "kubernetes.azure.com/user-iptables-rules"
	requestTimeout = 5 * time.Second
)

var (
	healthProbeSrcIPv4 netip.Addr = netip.MustParseAddr("169.254.7.127")
	healthProbeSrcIPv6 netip.Addr = netip.MustParseAddr("fd16:9254:7127:1337:ffff:ffff:ffff:ffff")
)

type OSFileLineReader struct{}

// Read opens the file and reads each line into a new string, returning the contents as a slice of strings
// Empty lines are skipped
func (OSFileLineReader) Read(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filename, err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip empty lines
		if line != "" {
			lines = append(lines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan file %s: %w", filename, err)
	}

	return lines, nil
}

// realEBPFClient provides eBPF map operations
type realEBPFClient struct{}

func NewEBPFClient() EBPFClient {
	return &realEBPFClient{}
}

// realRouteManager manages system routes via the ip command
type realRouteManager struct{}

func NewRouteManager() RouteManager {
	return &realRouteManager{}
}

// EnsureRoute adds a route for the given IP to the loopback device, replacing any existing route
func (r *realRouteManager) EnsureRoute(ip netip.Addr) error {
	args := []string{"route", "replace", ip.String() + "/32", "dev", "lo", "proto", "static", "scope", "link"}
	if ip.Is6() {
		args = []string{"-6", "route", "replace", ip.String() + "/128", "dev", "lo", "proto", "static", "scope", "link"}
	}
	cmd := exec.Command("ip", args...) // #nosec G204 -- args are validated IPs, not user-controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to ensure route for %s: %w (output: %s)", ip, err, string(output))
	}
	return nil
}

// GetBPFMapValue queries the bpf map at pinPath and gets the value at key 0
func (e *realEBPFClient) GetBPFMapValue(pinPath string) (uint64, error) {
	bpfMap, err := ebpf.LoadPinnedMap(pinPath, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to load pinned map %s: %w", pinPath, err)
	}
	defer bpfMap.Close()

	// 0 is the key for # of blocks
	key := uint32(0)
	value := uint64(0)

	if err := bpfMap.Lookup(&key, &value); err != nil {
		return 0, fmt.Errorf("failed to lookup key %d in bpf map: %w", key, err)
	}

	return value, nil
}

// patchLabel sets a specified label to a certain value on a ciliumnode resource by patching it
// Requires proper rbac
func patchLabel(clientset DynamicClient, labelValue bool, nodeName string) error {
	gvr := schema.GroupVersionResource{
		Group:    "cilium.io",
		Version:  "v2",
		Resource: "ciliumnodes",
	}

	patch := []byte(fmt.Sprintf(`{
	"metadata": {
		"labels": {
		"%s": "%v"
		}
	}
	}`, label, labelValue))

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	err := clientset.PatchResource(ctx, gvr, nodeName, types.MergePatchType, patch)
	if err != nil {
		return fmt.Errorf("failed to patch %s with label %s=%v: %w", nodeName, label, labelValue, err)
	}
	return nil
}

// createNodeEvent creates a Kubernetes event for the specified node
func createNodeEvent(clientset KubeClient, nodeName, reason, message, eventType string) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	node, err := clientset.GetNode(ctx, nodeName)
	if err != nil {
		return fmt.Errorf("failed to get node UID for %s: %w", nodeName, err)
	}

	now := metav1.NewTime(time.Now())

	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s.%d", nodeName, now.Unix()),
			Namespace: "default",
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:       "Node",
			Name:       nodeName,
			UID:        node.UID, // required for event to show up in node describe
			APIVersion: "v1",
		},
		Reason:         reason,
		Message:        message,
		Type:           eventType,
		FirstTimestamp: now,
		LastTimestamp:  now,
		Count:          1,
		Source: corev1.EventSource{
			Component: "azure-iptables-monitor",
		},
	}
	_, err = clientset.CreateEvent(ctx, "default", event)
	if err != nil {
		return fmt.Errorf("failed to create event for node %s: %w", nodeName, err)
	}

	slog.Info("Created event for node", "node", nodeName, "reason", reason, "message", message)
	return nil
}

// GetRules returns all rules as a slice of strings for the specified tableName
func GetRules(client IPTablesClient, tableName string) ([]string, error) {
	var allRules []string
	chains, err := client.ListChains(tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to list chains for table %s: %w", tableName, err)
	}

	for _, chain := range chains {
		rules, err := client.List(tableName, chain)
		if err != nil {
			return nil, fmt.Errorf("failed to list rules for table %s chain %s: %w", tableName, chain, err)
		}
		allRules = append(allRules, rules...)
	}

	return allRules, nil
}

// hasUnexpectedRules checks if any rules in currentRules don't match any of the allowedPatterns
// Returns true if there are unexpected rules, false if all rules match expected patterns
func hasUnexpectedRules(currentRules, allowedPatterns []string) bool {
	foundUnexpectedRules := false

	// compile regex patterns
	compiledPatterns := make([]*regexp.Regexp, 0, len(allowedPatterns))
	for _, pattern := range allowedPatterns {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			slog.Error("Error compiling regex pattern", "pattern", pattern, "error", err)
			continue
		}
		compiledPatterns = append(compiledPatterns, compiled)
	}

	// check each rule to see if it matches any allowed pattern
	for _, rule := range currentRules {
		ruleMatched := false
		for _, pattern := range compiledPatterns {
			if pattern.MatchString(rule) {
				slog.Debug("MATCHED", "rule", rule, "pattern", pattern.String())
				ruleMatched = true
				break
			}
		}
		if !ruleMatched {
			slog.Info("Unexpected rule", "rule", rule)
			foundUnexpectedRules = true
			// continue to iterate over remaining rules to identify all unexpected rules
		}
	}

	return foundUnexpectedRules
}

// nodeHasUserIPTablesRules returns true if the node has iptables rules that do not match the regex
// specified in the rule's respective table: nat, mangle, filter, raw, or security
// The global file's regexes can match to a rule in any table
func nodeHasUserIPTablesRules(fileReader FileLineReader, path string, iptablesClient IPTablesClient) bool {
	tables := []string{"nat", "mangle", "filter", "raw", "security"}

	globalPatterns, err := fileReader.Read(filepath.Join(path, "global"))
	if err != nil {
		globalPatterns = []string{}
		slog.Info("No global patterns file found, using empty patterns")
	}

	userIPTablesRules := false

	slog.Info("Using reference patterns files", "path", path)

	for _, table := range tables {
		rules, err := GetRules(iptablesClient, table)
		if err != nil {
			slog.Error("Failed to get rules for table", "table", table, "error", err)
			continue
		}

		var referencePatterns []string
		referencePatterns, err = fileReader.Read(filepath.Join(path, table))
		if err != nil {
			referencePatterns = []string{}
			slog.Info("No reference patterns file found for table", "table", table)
		}

		referencePatterns = append(referencePatterns, globalPatterns...)

		slog.Debug("Checking table", "table", table)
		if hasUnexpectedRules(rules, referencePatterns) {
			slog.Info("Unexpected rules detected in table", "table", table)
			userIPTablesRules = true
		}
	}

	return userIPTablesRules
}

// Check returns true if the node has user iptables rules (ipv4 or ipv6, based on the config), false otherwise
func Check(cfg Config, deps Dependencies, previousBlocks *uint64) bool {
	userIPTablesRulesFound := nodeHasUserIPTablesRules(deps.FileReader, cfg.ConfigPath4, deps.IPTablesV4)
	if userIPTablesRulesFound {
		slog.Info("User iptables rules detected in IPv4 iptables")
	}

	// check ip6tables rules if enabled
	if cfg.IPv6Enabled {
		userIP6TablesRulesFound := nodeHasUserIPTablesRules(deps.FileReader, cfg.ConfigPath6, deps.IPTablesV6)
		if userIP6TablesRulesFound {
			slog.Info("User iptables rules detected in IPv6 iptables")
		}
		userIPTablesRulesFound = userIPTablesRulesFound || userIP6TablesRulesFound
	}

	// update label based on whether user iptables rules were found
	err := patchLabel(deps.DynamicClient, userIPTablesRulesFound, cfg.NodeName)
	if err != nil {
		slog.Error("Failed to patch label", "error", err)
	} else {
		slog.Info("Successfully updated label", "node", cfg.NodeName, "label", label, "value", userIPTablesRulesFound)
	}

	if cfg.SendEvents && userIPTablesRulesFound {
		err = createNodeEvent(deps.KubeClient, cfg.NodeName, "UnexpectedIPTablesRules", "Node has unexpected iptables rules", corev1.EventTypeWarning)
		if err != nil {
			slog.Error("Failed to create event", "error", err)
		}
	}

	// if disabled the number of blocks never increases from zero
	currentBlocks := uint64(0)
	if cfg.CheckMap {
		// read bpf map to check for number of blocked iptables rules
		currentBlocks, err = deps.EBPFClient.GetBPFMapValue(cfg.PinPath)
		if err != nil {
			slog.Error("Failed to get bpf map value", "error", err)
		}
		slog.Info("IPTables rules blocks", "previous", *previousBlocks, "current", currentBlocks)
	}

	// if number of blocked rules increased since last time
	blockedRulesIncreased := currentBlocks > *previousBlocks
	if cfg.SendEvents && blockedRulesIncreased {
		msg := "A process attempted to add iptables rules to the node but was blocked since last check. " +
			"iptables rules blocked because EBPF Host Routing is enabled: aka.ms/acnsperformance"
		err = createNodeEvent(deps.KubeClient, cfg.NodeName, "BlockedIPTablesRule", msg, corev1.EventTypeWarning)
		if err != nil {
			slog.Error("Failed to create iptables block event", "error", err)
		}
	}
	// persist between runs
	*previousBlocks = currentBlocks
	return userIPTablesRulesFound
}

// installHealthProbeReplyRoutes installs loopback routes for replies sent to kubelet health probes.
// IPv6 route is installed only when ipv6Enabled is true.
func installHealthProbeReplyRoutes(deps Dependencies, ipv6Enabled bool) {
	if err := deps.RouteManager.EnsureRoute(healthProbeSrcIPv4); err != nil {
		slog.Error("Failed to install IPv4 route for health probe reply", "error", err)
	} else {
		slog.Info("Installed loopback route for health probe IPv4", "ip", healthProbeSrcIPv4)
	}

	if ipv6Enabled {
		if err := deps.RouteManager.EnsureRoute(healthProbeSrcIPv6); err != nil {
			slog.Error("Failed to install IPv6 route for health probe reply", "error", err)
		} else {
			slog.Info("Installed loopback route for health probe IPv6", "ip", healthProbeSrcIPv6)
		}
	}
}

// Run runs Check in a loop and handles the number of blocks
func Run(cfg Config, deps Dependencies) {
	if cfg.InstallRoutesForHealthProbeReply {
		installHealthProbeReplyRoutes(deps, cfg.IPv6Enabled)
	}

	blockCount := uint64(0)

	for {
		userIPTablesRulesFound := Check(cfg, deps, &blockCount)

		if !userIPTablesRulesFound && cfg.TerminateOnSuccess {
			slog.Info("No user iptables rules found, terminating the iptables monitor")
			break
		}
		time.Sleep(time.Duration(cfg.CheckInterval) * time.Second)
	}
}

func main() {
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	slog.Info("Starting", "version", version)

	// get current node name from environment variable
	currentNodeName := os.Getenv("NODE_NAME")
	if currentNodeName == "" {
		slog.Error("NODE_NAME environment variable not set")
		os.Exit(1)
	}

	cfg := Config{
		ConfigPath4:                      *configPath4,
		ConfigPath6:                      *configPath6,
		CheckInterval:                    *checkInterval,
		SendEvents:                       *sendEvents,
		IPv6Enabled:                      *ipv6Enabled,
		CheckMap:                         *checkMap,
		PinPath:                          *pinPath,
		TerminateOnSuccess:               *terminateOnSuccess,
		InstallRoutesForHealthProbeReply: *installRoutesForHealthProbeReply,
		NodeName:                         currentNodeName,
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		slog.Error("Failed to create in-cluster config", "error", err)
		os.Exit(1)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		slog.Error("Failed to create kubernetes clientset", "error", err)
		os.Exit(1)
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		slog.Error("Failed to create dynamic client", "error", err)
		os.Exit(1)
	}

	var iptablesClient IPTablesClient
	iptablesClient, err = goiptables.New()
	if err != nil {
		slog.Error("Failed to create iptables client", "error", err)
		os.Exit(1)
	}

	var ip6tablesClient IPTablesClient
	if *ipv6Enabled {
		ip6tablesClient, err = goiptables.New(goiptables.IPFamily(goiptables.ProtocolIPv6))
		if err != nil {
			slog.Error("Failed to create ip6tables client", "error", err)
			os.Exit(1)
		}
	}
	slog.Info("IPv6 enabled", "enabled", *ipv6Enabled)

	deps := Dependencies{
		KubeClient:    NewKubeClient(clientset),
		DynamicClient: NewDynamicClient(dynamicClient),
		IPTablesV4:    iptablesClient,
		IPTablesV6:    ip6tablesClient,
		EBPFClient:    NewEBPFClient(),
		FileReader:    OSFileLineReader{},
	}

	if *installRoutesForHealthProbeReply {
		deps.RouteManager = NewRouteManager()
		slog.Info("Route installation for health probe reply enabled")
	}

	slog.Info("Starting iptables monitor", "node", cfg.NodeName)

	Run(cfg, deps)
}
