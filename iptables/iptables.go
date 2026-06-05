package iptables

// This package contains wrapper functions to program iptables rules

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/cni/log"
	"github.com/Azure/azure-container-networking/platform"
	"go.uber.org/zap"
)

// iptables exit code 1 emits this exact stderr substring (across iptables-legacy
// and iptables-nft, IPv4 and IPv6) when the rule being matched/deleted is not
// present. Any OTHER iptables error (lock contention, permission denied, syntax
// error) produces different stderr, so this substring is the safest "rule
// absent" signal we can detect from ExecuteRawCommand output.
const iptablesRuleNotFoundSubstr = "does a matching rule exist"

var (
	logger                        = log.CNILogger.With(zap.String("component", "cni-iptables"))
	errCouldNotValidateRuleExists = errors.New("could not validate iptable rule exists after insertion")
)

// cni iptable chains
const (
	CNIInputChain  = "AZURECNIINPUT"
	CNIOutputChain = "AZURECNIOUTPUT"
)

// standard iptable chains
const (
	Input       = "INPUT"
	Output      = "OUTPUT"
	Forward     = "FORWARD"
	Prerouting  = "PREROUTING"
	Postrouting = "POSTROUTING"
	Swift       = "SWIFT"
	Snat        = "SNAT"
	Return      = "RETURN"
)

// Standard Table names
const (
	Filter = "filter"
	Nat    = "nat"
	Mangle = "mangle"
	Raw    = "raw"
)

// target
const (
	Accept     = "ACCEPT"
	Drop       = "DROP"
	Masquerade = "MASQUERADE"
	Notrack    = "NOTRACK"
)

// actions
const (
	Insert = "I"
	Append = "A"
	Delete = "D"
)

// states
const (
	Established = "ESTABLISHED"
	Related     = "RELATED"
)

const (
	iptables    = "iptables"
	ip6tables   = "ip6tables"
	lockTimeout = 60
)

const (
	V4 = "4"
	V6 = "6"
)

// known ports
const (
	DNSPort  = 53
	HTTPPort = 80
)

// known protocols
const (
	UDP = "udp"
	TCP = "tcp"
)

var DisableIPTableLock bool

type IPTableEntry struct {
	Version string
	Params  string
}

type Client struct {
	pl platform.ExecClient
}

func NewClient() *Client {
	return &Client{
		pl: platform.NewExecClient(logger),
	}
}

// Run iptables command
func (c *Client) RunCmd(version, params string) error {
	var cmd string

	iptCmd := iptables
	if version == V6 {
		iptCmd = ip6tables
	}

	if DisableIPTableLock {
		cmd = fmt.Sprintf("%s %s", iptCmd, params)
	} else {
		cmd = fmt.Sprintf("%s -w %d %s", iptCmd, lockTimeout, params)
	}

	if _, err := c.pl.ExecuteRawCommand(cmd); err != nil {
		return err
	}

	return nil
}

// check if iptable chain alreay exists
func (c *Client) ChainExists(version, tableName, chainName string) bool {
	params := fmt.Sprintf("-t %s -nL %s", tableName, chainName)
	if err := c.RunCmd(version, params); err != nil {
		return false
	}

	return true
}

func (c *Client) GetCreateChainCmd(version, tableName, chainName string) IPTableEntry {
	return IPTableEntry{
		Version: version,
		Params:  fmt.Sprintf("-t %s -N %s", tableName, chainName),
	}
}

// create new iptable chain under specified table name
func (c *Client) CreateChain(version, tableName, chainName string) error {
	var err error

	if !c.ChainExists(version, tableName, chainName) {
		cmd := c.GetCreateChainCmd(version, tableName, chainName)
		err = c.RunCmd(version, cmd.Params)
	} else {
		logger.Info("Chain exists in table", zap.String("chainName", chainName), zap.String("tableName", tableName))
	}

	return err
}

// check if iptable rule alreay exists
func (c *Client) RuleExists(version, tableName, chainName, match, target string) bool {
	params := fmt.Sprintf("-t %s -C %s %s -j %s", tableName, chainName, match, target)
	if err := c.RunCmd(version, params); err != nil {
		return false
	}
	return true
}

func (c *Client) GetInsertIptableRuleCmd(version, tableName, chainName, match, target string) IPTableEntry {
	return IPTableEntry{
		Version: version,
		Params:  fmt.Sprintf("-t %s -I %s 1 %s -j %s", tableName, chainName, match, target),
	}
}

// Insert iptable rule at beginning of iptable chain
func (c *Client) InsertIptableRule(version, tableName, chainName, match, target string) error {
	if c.RuleExists(version, tableName, chainName, match, target) {
		logger.Info("Rule already exists")
		return nil
	}

	cmd := c.GetInsertIptableRuleCmd(version, tableName, chainName, match, target)
	err := c.RunCmd(version, cmd.Params)
	if err != nil {
		return err
	}
	if !c.RuleExists(version, tableName, chainName, match, target) {
		return errCouldNotValidateRuleExists
	}
	return nil
}

func (c *Client) GetAppendIptableRuleCmd(version, tableName, chainName, match, target string) IPTableEntry {
	return IPTableEntry{
		Version: version,
		Params:  fmt.Sprintf("-t %s -A %s %s -j %s", tableName, chainName, match, target),
	}
}

// Append iptable rule at end of iptable chain
func (c *Client) AppendIptableRule(version, tableName, chainName, match, target string) error {
	if c.RuleExists(version, tableName, chainName, match, target) {
		logger.Info("Rule already exists")
		return nil
	}

	cmd := c.GetAppendIptableRuleCmd(version, tableName, chainName, match, target)
	err := c.RunCmd(version, cmd.Params)
	if err != nil {
		return err
	}
	if !c.RuleExists(version, tableName, chainName, match, target) {
		return errCouldNotValidateRuleExists
	}
	return nil
}

// Delete matched iptable rule
func (c *Client) DeleteIptableRule(version, tableName, chainName, match, target string) error {
	params := fmt.Sprintf("-t %s -D %s %s -j %s", tableName, chainName, match, target)
	return c.RunCmd(version, params)
}

// DeleteIptableRuleIfExists deletes the rule unconditionally and swallows ONLY
// the "rule does not exist" error. All other failures (xtables lock contention,
// permission denied, syntax errors, etc.) are surfaced to the caller.
//
// This is the safe alternative to the `RuleExists(...) { DeleteIptableRule(...) }`
// pattern: RuleExists returns false for ANY check error (including transient
// lock failures), causing cleanup to silently skip the delete and leave stale
// host state behind. By attempting the delete first and only ignoring the
// kernel's explicit "no such rule" signal, we get crash-safe idempotency
// without losing visibility into real failures.
//
// Detection note: iptables returns exit code 1 for both "rule not found" AND
// some other error classes, so we must inspect stderr (not just the exit code)
// to distinguish them. The substring "does a matching rule exist" is the
// long-standing iptables message for a missing rule and is stable across
// iptables-legacy and iptables-nft on both IPv4 and IPv6.
func (c *Client) DeleteIptableRuleIfExists(version, tableName, chainName, match, target string) error {
	err := c.DeleteIptableRule(version, tableName, chainName, match, target)
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), iptablesRuleNotFoundSubstr) {
		logger.Info("iptables rule already absent, treating as success",
			zap.String("table", tableName), zap.String("chain", chainName),
			zap.String("match", match), zap.String("target", target))
		return nil
	}
	return err
}
