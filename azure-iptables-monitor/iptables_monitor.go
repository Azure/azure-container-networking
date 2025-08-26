package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
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
	"k8s.io/component-base/logs"
	"k8s.io/component-base/version/verflag"
	"k8s.io/klog/v2"
)

// Version is populated by make during build.
var version string

var (
	configPath4   = flag.String("input", "/etc/config/", "Name of the directory with the ipv4 allowed regex files")
	configPath6   = flag.String("input6", "/etc/config6/", "Name of directory with the ipv6 allowed regex files")
	checkInterval = flag.Int("interval", 300, "How often to check for user iptables rules and bpf map increases (in seconds)")
	sendEvents    = flag.Bool("events", false, "Whether to send node events if unexpected iptables rules are detected")
	ipv6Enabled   = flag.Bool("ipv6", false, "Whether to check ip6tables using the ipv6 allowlists")
	checkMap      = flag.Bool("checkMap", false, "Whether to check the bpf map at mapPath for increases")
	pinPath       = flag.String("mapPath", "/azure-block-iptables/iptables_block_event_counter", "Path to pinned bpf map")
)

const label = "kubernetes.azure.com/user-iptables-rules"

type FileLineReader interface {
	Read(filename string) ([]string, error)
}

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

// patchLabel sets a specified label to a certain value on a ciliumnode resource by patching it
// Requires proper rbac
func patchLabel(clientset dynamic.Interface, labelValue bool, nodeName string) error {
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

	_, err := clientset.Resource(gvr).
		Patch(context.TODO(), nodeName, types.MergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("failed to patch %s with label %s=%v: %w", nodeName, label, labelValue, err)
	}
	return nil
}

// createNodeEvent creates a Kubernetes event for the specified node
func createNodeEvent(clientset *kubernetes.Clientset, nodeName, reason, message, eventType string) error {
	node, err := clientset.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
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
	_, err = clientset.CoreV1().Events("default").Create(
		context.TODO(),
		event,
		metav1.CreateOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to create event for node %s: %w", nodeName, err)
	}

	klog.V(2).Infof("Created event for node %s: %s - %s", nodeName, reason, message)
	return nil
}

type IPTablesClient interface {
	ListChains(table string) ([]string, error)
	List(table, chain string) ([]string, error)
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
			klog.Errorf("Error compiling regex pattern '%s': %v", pattern, err)
			continue
		}
		compiledPatterns = append(compiledPatterns, compiled)
	}

	// check each rule to see if it matches any allowed pattern
	for _, rule := range currentRules {
		ruleMatched := false
		for _, pattern := range compiledPatterns {
			if pattern.MatchString(rule) {
				klog.V(3).Infof("MATCHED: '%s' -> pattern: '%s'", rule, pattern.String())
				ruleMatched = true
				break
			}
		}
		if !ruleMatched {
			klog.Infof("Unexpected rule: %s", rule)
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
		klog.V(2).Infof("No global patterns file found, using empty patterns")
	}

	userIPTablesRules := false

	klog.V(2).Infof("Using reference patterns files in %s", path)

	for _, table := range tables {
		rules, err := GetRules(iptablesClient, table)
		if err != nil {
			klog.Errorf("failed to get rules for table %s: %v", table, err)
			continue
		}

		var referencePatterns []string
		referencePatterns, err = fileReader.Read(filepath.Join(path, table))
		if err != nil {
			referencePatterns = []string{}
			klog.V(2).Infof("No reference patterns file found for table %s", table)
		}

		referencePatterns = append(referencePatterns, globalPatterns...)

		klog.V(3).Infof("===== %s =====", table)
		if hasUnexpectedRules(rules, referencePatterns) {
			klog.Infof("Unexpected rules detected in table %s", table)
			userIPTablesRules = true
		}
	}

	return userIPTablesRules
}

func getBPFMapValue() (uint64, error) {
	m, err := ebpf.LoadPinnedMap(*pinPath, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to load pinned map %s: %w", *pinPath, err)
	}
	defer m.Close()
	// 0 is the key for # of blocks
	key := uint32(0)
	value := uint64(0)

	if err := m.Lookup(&key, &value); err != nil {
		return 0, fmt.Errorf("failed to lookup key %d in bpf map: %w", key, err)
	}

	return value, nil
}

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	logs.InitLogs()
	defer logs.FlushLogs()

	klog.Infof("Version: %s", version)
	verflag.PrintAndExitIfRequested()

	config, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatalf("failed to create in-cluster config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("failed to create kubernetes clientset: %v", err)
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		klog.Fatalf("failed to create dynamic client: %v", err)
	}

	var iptablesClient IPTablesClient
	iptablesClient, err = goiptables.New()
	if err != nil {
		klog.Fatalf("failed to create iptables client: %v", err)
	}

	var ip6tablesClient IPTablesClient
	if *ipv6Enabled {
		ip6tablesClient, err = goiptables.New(goiptables.IPFamily(goiptables.ProtocolIPv6))
		if err != nil {
			klog.Fatalf("failed to create ip6tables client: %v", err)
		}
	}
	klog.Infof("IPv6: %v", *ipv6Enabled)

	// get current node name from environment variable
	currentNodeName := os.Getenv("NODE_NAME")
	if currentNodeName == "" {
		klog.Fatalf("NODE_NAME environment variable not set")
	}

	klog.Infof("Starting iptables monitor for node: %s", currentNodeName)

	var fileReader FileLineReader = OSFileLineReader{}

	previousBlocks := uint64(0)

	for {
		userIPTablesRulesFound := nodeHasUserIPTablesRules(fileReader, *configPath4, iptablesClient)
		if userIPTablesRulesFound {
			klog.Info("Above user iptables rules detected in IPv4 iptables")
		}

		// check ip6tables rules if enabled
		if *ipv6Enabled {
			userIP6TablesRulesFound := nodeHasUserIPTablesRules(fileReader, *configPath6, ip6tablesClient)
			if userIP6TablesRulesFound {
				klog.Info("Above user iptables rules detected in IPv6 iptables")
			}
			userIPTablesRulesFound = userIPTablesRulesFound || userIP6TablesRulesFound
		}

		// update label based on whether user iptables rules were found
		err = patchLabel(dynamicClient, userIPTablesRulesFound, currentNodeName)
		if err != nil {
			klog.Errorf("failed to patch label: %v", err)
		} else {
			klog.V(2).Infof("Successfully updated label for %s: %s=%v", currentNodeName, label, userIPTablesRulesFound)
		}

		if *sendEvents && userIPTablesRulesFound {
			err = createNodeEvent(clientset, currentNodeName, "UnexpectedIPTablesRules", "Node has unexpected iptables rules", corev1.EventTypeWarning)
			if err != nil {
				klog.Errorf("failed to create event: %v", err)
			}
		}

		// if disabled the number of blocks never increases from zero
		currentBlocks := uint64(0)
		if *checkMap {
			// read bpf map to check for number of blocked iptables rules
			currentBlocks, err = getBPFMapValue()
			if err != nil {
				klog.Errorf("failed to get bpf map value: %v", err)
			}
			klog.V(2).Infof("IPTables rules blocks: Previous: %d Current: %d", previousBlocks, currentBlocks)
		}
		// if number of blocked rules increased since last time
		blockedRulesIncreased := currentBlocks > previousBlocks
		if *sendEvents && blockedRulesIncreased {
			msg := "A process attempted to add iptables rules to the node but was blocked since last check. " +
				"iptables rules blocked because EBPF Host Routing is enabled: aka.ms/acnsperformance"
			err = createNodeEvent(clientset, currentNodeName, "BlockedIPTablesRule", msg, corev1.EventTypeWarning)
			if err != nil {
				klog.Errorf("failed to create iptables block event: %v", err)
			}
		}
		previousBlocks = currentBlocks

		time.Sleep(time.Duration(*checkInterval) * time.Second)
	}
}
