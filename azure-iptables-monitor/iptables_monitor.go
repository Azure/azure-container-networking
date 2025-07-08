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

	goiptables "github.com/coreos/go-iptables/iptables"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/component-base/logs"
	"k8s.io/component-base/version/verflag"
	"k8s.io/klog/v2"
)

// Version is populated by make during build.
var version string

var (
	configPath    = flag.String("input", "/etc/config/", "Name of the directory with the allowed regex files")
	checkInterval = flag.Int("interval", 600, "How often to check iptables rules (in seconds)")
)

const nodeLabel = "user-iptables-rules"

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

// patchNodeLabel sets a specified node label to a certain value by patching it
// Requires proper rbac (node patch)
func patchNodeLabel(clientset *kubernetes.Clientset, labelValue bool, nodeName string) error {
	patch := []byte(fmt.Sprintf(`{
	"metadata": {
		"labels": {
		"%s": "%v"
		}
	}
	}`, nodeLabel, labelValue))

	_, err := clientset.CoreV1().Nodes().Patch(
		context.TODO(),
		nodeName,
		types.StrategicMergePatchType,
		patch,
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to patch node %s with label %s=%v: %w", nodeName, nodeLabel, labelValue, err)
	}
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
		}
	}

	return foundUnexpectedRules
}

// nodeHasUserIPTablesRules returns true if the node has iptables rules that do not match the regex
// specified in the rule's respective table: nat, mangle, filter, raw, or security
// The global file's regexes can match to a rule in any table
func nodeHasUserIPTablesRules(fileReader FileLineReader, iptablesClient IPTablesClient) bool {
	tables := []string{"nat", "mangle", "filter", "raw", "security"}

	globalPatterns, err := fileReader.Read(filepath.Join(*configPath, "global"))
	if err != nil {
		globalPatterns = []string{}
		klog.V(2).Infof("No global patterns file found, using empty patterns")
	}

	userIPTablesRules := false

	for _, table := range tables {
		rules, err := GetRules(iptablesClient, table)
		if err != nil {
			klog.Errorf("failed to get rules for table %s: %v", table, err)
			continue
		}

		var referencePatterns []string
		referencePatterns, err = fileReader.Read(filepath.Join(*configPath, table))
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

	var iptablesClient IPTablesClient
	iptablesClient, err = goiptables.New()
	if err != nil {
		klog.Fatalf("failed to create iptables client: %v", err)
	}

	// get current node name from environment variable
	currentNodeName := os.Getenv("NODE_NAME")
	if currentNodeName == "" {
		klog.Fatalf("NODE_NAME environment variable not set")
	}

	klog.Infof("Starting iptables monitor for node: %s", currentNodeName)

	var fileReader FileLineReader = OSFileLineReader{}

	for {
		nodeHasUserIPTablesRules := nodeHasUserIPTablesRules(fileReader, iptablesClient)

		// update node label based on whether user iptables rules were found
		err = patchNodeLabel(clientset, nodeHasUserIPTablesRules, currentNodeName)
		if err != nil {
			klog.Errorf("failed to patch node label: %v", err)
		} else {
			klog.V(2).Infof("Successfully updated node label for %s: %s=%v", currentNodeName, nodeLabel, nodeHasUserIPTablesRules)
		}

		time.Sleep(time.Duration(*checkInterval) * time.Second)
	}
}
