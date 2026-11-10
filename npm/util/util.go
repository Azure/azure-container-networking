// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package util

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"hash/fnv"
	"math/big"
	"net"
	"net/netip"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Masterminds/semver"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/tools/cache"
)

// DeleteOption is used to decide if a delete is force delete or soft delete
type DeleteOption bool

const (
	// For DeleteIPSet
	ForceDelete DeleteOption = true
	SoftDelete  DeleteOption = false
)

var ErrEmptyNodeIP = errors.New("error: node IP is empty")

// regex to get minor version
var re = regexp.MustCompile("[0-9]+")

func IsWindowsDP() bool {
	return runtime.GOOS == "windows"
}

// Exists reports whether the named file or directory exists.
func Exists(filePath string) bool {
	if _, err := os.Stat(filePath); err == nil {
		return true
	} else if !os.IsNotExist(err) {
		return true
	}

	return false
}

// GetClusterID retrieves cluster ID through node name. (Azure-specific)
func GetClusterID(nodeName string) string {
	s := strings.Split(nodeName, "-")
	if len(s) < 3 {
		return ""
	}

	return s[2]
}

// Hash hashes a string to another string with length <= 32.
func Hash(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	return fmt.Sprint(h.Sum32())
}

// hashedChainDigestLen is the number of base36 digest characters used as the variable part of
// an iptables policy chain name. iptables limits a chain name to 28 characters, and the
// longest chain prefix (IptablesAzureIngressPolicyChainPrefix, 17 chars) plus a dash already
// uses 18, leaving 10 characters for the digest. A base36 digest across those 10 characters
// spans ~51 bits, far wider than an fmt-formatted 32-bit Hash, so distinct policies are far
// less likely to map to the same chain name.
const hashedChainDigestLen = 10

// GetHashedChainName returns a fixed-width base36 digest of name for use as the variable part
// of an iptables policy chain name. It reduces a wide (SHA-256) digest modulo
// 36^hashedChainDigestLen so every base36 digit is uniform (truncating the leading digits of
// the full integer would be slightly biased), and left-pads so the result is always exactly
// hashedChainDigestLen characters.
func GetHashedChainName(name string) string {
	sum := sha256.Sum256([]byte(name))
	mod := new(big.Int).Exp(big.NewInt(36), big.NewInt(hashedChainDigestLen), nil)
	digest := new(big.Int).Mod(new(big.Int).SetBytes(sum[:]), mod).Text(36)
	if len(digest) < hashedChainDigestLen {
		digest = strings.Repeat("0", hashedChainDigestLen-len(digest)) + digest
	}
	return digest[:hashedChainDigestLen]
}

// hashedNameDigestLen is the number of base36 digest characters appended after
// AzureNpmPrefix to form a kernel ipset name. 20 base36 chars is ~103 bits, which keeps
// distinct ipset names from resolving to the same kernel name, and fits the 31-char limit.
const hashedNameDigestLen = 20

// SortMap sorts the map by key in alphabetical order.
// Note: even though the map is sorted, accessing it through range will still result in random order.
func SortMap(m *map[string]string) ([]string, []string) {
	var sortedKeys, sortedVals []string
	for k := range *m {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	sortedMap := &map[string]string{}
	for _, k := range sortedKeys {
		v := (*m)[k]
		(*sortedMap)[k] = v
		sortedVals = append(sortedVals, v)
	}

	m = sortedMap

	return sortedKeys, sortedVals
}

// GetIPSetListFromLabels combine Labels into a single slice
func GetIPSetListFromLabels(labels map[string]string) []string {
	ipsetList := []string{}
	for labelKey, labelVal := range labels {
		ipsetList = append(ipsetList, labelKey, labelKey+IpsetLabelDelimter+labelVal)
	}
	return ipsetList
}

// GetIPSetListCompareLabels compares Labels and
// returns a delete ipset list and add ipset list
func GetIPSetListCompareLabels(orig map[string]string, new map[string]string) ([]string, []string) {
	notInOrig := []string{}
	notInNew := []string{}

	for keyOrig, valOrig := range orig {
		if valNew, ok := new[keyOrig]; ok {
			if valNew != valOrig {
				notInNew = append(notInNew, keyOrig+IpsetLabelDelimter+valOrig)
				notInOrig = append(notInOrig, keyOrig+IpsetLabelDelimter+valNew)
			}
		} else {
			// {IMPORTANT} this order is important, key should be before and key+val later
			notInNew = append(notInNew, keyOrig, keyOrig+IpsetLabelDelimter+valOrig)
		}
	}

	for keyNew, valNew := range new {
		if _, ok := orig[keyNew]; !ok {
			// {IMPORTANT} this order is important, key should be before and key+val later
			notInOrig = append(notInOrig, keyNew, keyNew+IpsetLabelDelimter+valNew)
		}
	}

	return notInOrig, notInNew
}

// UniqueStrSlice removes duplicate elements from the input string.
func UniqueStrSlice(s []string) []string {
	m, unique := map[string]bool{}, []string{}
	for _, elem := range s {
		if m[elem] {
			continue
		}

		m[elem] = true
		unique = append(unique, elem)
	}

	return unique
}

// ClearAndAppendMap clears base and appends new to base.
func ClearAndAppendMap(base, new map[string]string) map[string]string {
	base = make(map[string]string)
	for k, v := range new {
		base[k] = v
	}

	return base
}

// AppendMap appends new to base.
func AppendMap(base, new map[string]string) map[string]string {
	for k, v := range new {
		base[k] = v
	}

	return base
}

// GetHashedName returns the kernel ipset name for the given prefixed name. It uses a wide
// digest (wider than GetHashedChainName's, since iptables chain names are more
// length-constrained) so distinct ipset names map to distinct kernel names. The result is
// AzureNpmPrefix (10) + hashedNameDigestLen (20) = 30 chars, within the 31-char kernel ipset name limit.
func GetHashedName(name string) string {
	sum := sha256.Sum256([]byte(name))
	// Text(36) omits leading zeros, so a small digest could be shorter than
	// hashedNameDigestLen; left-pad to a fixed width before slicing so the result is always
	// exactly hashedNameDigestLen characters and the slice can never panic.
	digest := new(big.Int).SetBytes(sum[:]).Text(36)
	if len(digest) < hashedNameDigestLen {
		digest = strings.Repeat("0", hashedNameDigestLen-len(digest)) + digest
	}
	return AzureNpmPrefix + digest[:hashedNameDigestLen]
}

// CompareK8sVer compares two k8s versions.
// returns -1, 0, 1 if firstVer smaller, equals, bigger than secondVer respectively.
// returns -2 for error.
func CompareK8sVer(firstVer *version.Info, secondVer *version.Info) int {
	v1Minor := re.FindAllString(firstVer.Minor, -1)
	if len(v1Minor) < 1 {
		return -2
	}
	v1, err := semver.NewVersion(firstVer.Major + "." + v1Minor[0])
	if err != nil {
		return -2
	}
	v2Minor := re.FindAllString(secondVer.Minor, -1)
	if len(v2Minor) < 1 {
		return -2
	}
	v2, err := semver.NewVersion(secondVer.Major + "." + v2Minor[0])
	if err != nil {
		return -2
	}

	return v1.Compare(v2)
}

// GetOperatorAndLabel returns the operator associated with the label and the label without operator.
func GetOperatorAndLabel(label string) (string, string) {
	if len(label) == 0 {
		return "", ""
	}

	if string(label[0]) == IptablesNotFlag {
		return IptablesNotFlag, label[1:]
	}

	return "", label
}

// GetLabelsWithoutOperators returns labels without operators.
func GetLabelsWithoutOperators(labels []string) []string {
	var res []string
	for _, label := range labels {
		if len(label) > 0 {
			if string(label[0]) == IptablesNotFlag {
				res = append(res, label[1:])
			} else {
				res = append(res, label)
			}
		}
	}

	return res
}

// DropEmptyFields deletes empty entries from a slice.
func DropEmptyFields(s []string) []string {
	i := 0
	for {
		if i == len(s) {
			break
		}

		if s[i] == "" {
			s = append(s[:i], s[i+1:]...)
			continue
		}

		i++
	}

	return s
}

// GetNSNameWithPrefix returns Namespace name with ipset prefix
func GetNSNameWithPrefix(nsName string) string {
	return NamespacePrefix + nsName
}

// CompareResourceVersions take in two resource versions and returns true if new is greater than old
func CompareResourceVersions(rvOld string, rvNew string) bool {
	// Ignore oldRV error as we care about new RV
	tempRvOld := ParseResourceVersion(rvOld)
	tempRvnew := ParseResourceVersion(rvNew)
	return tempRvnew > tempRvOld
}

// CompareUintResourceVersions take in two resource versions as uint and returns true if new is greater than old
func CompareUintResourceVersions(rvOld uint64, rvNew uint64) bool {
	return rvNew > rvOld
}

// ParseResourceVersion get uint64 version of ResourceVersion
func ParseResourceVersion(rv string) uint64 {
	if rv == "" {
		return 0
	}
	rvInt, err := strconv.ParseUint(rv, 10, 64)
	if err != nil {
		log.Logf("Error: while parsing resource version to uint64 %s", rv)
	}

	return rvInt
}

// GetObjKeyFunc will return obj's key
func GetObjKeyFunc(obj interface{}) (string, error) {
	return cache.MetaNamespaceKeyFunc(obj)
}

// GetSetsFromLabels for a given map of labels will return ipset names
func GetSetsFromLabels(labels map[string]string) []string {
	l := []string{}

	for k, v := range labels {
		l = append(l, k, fmt.Sprintf("%s%s%s", k, IpsetLabelDelimter, v))
	}

	return l
}

func GetIpSetFromLabelKV(k, v string) string {
	return fmt.Sprintf("%s%s%s", k, IpsetLabelDelimter, v)
}

func IsKeyValueLabelSetName(k string) bool {
	return strings.Contains(k, IpsetLabelDelimter)
}

func GetLabelKVFromSet(ipsetName string) (string, string) {
	strSplit := strings.Split(ipsetName, IpsetLabelDelimter)
	if len(strSplit) > 1 {
		return strSplit[0], strSplit[1]
	}
	return strSplit[0], ""
}

// StrExistsInSlice check if a string already exists in a given slice
func StrExistsInSlice(items []string, val string) bool {
	for _, item := range items {
		if item == val {
			return true
		}
	}
	return false
}

func CompareSlices(list1, list2 []string) bool {
	for _, item := range list1 {
		if !StrExistsInSlice(list2, item) {
			return false
		}
	}
	return true
}

func SliceToString(list []string) string {
	return strings.Join(list, SetPolicyDelimiter)
}

func IsIPV4(ip string) bool {
	isIPBlock := strings.Contains(ip, "/")
	ipOnly := strings.Split(ip, "/")
	if strings.Contains(ip, "/0") && ipOnly[0] != "0.0.0.0" {
		return false
	}

	address, err := netip.ParseAddr(ipOnly[0])
	if err != nil {
		return false
	}

	if address.Is4() && isIPBlock {
		_, _, err := net.ParseCIDR(ip)
		return err == nil
	}

	return address.Is4()
}

// Get preferred outbound ip of this machine
// source: https://stackoverflow.com/questions/23558425/how-do-i-get-the-local-ip-address-in-go
func NodeIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", fmt.Errorf("failed to get node IP: %w", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	nodeIP := localAddr.IP.String()
	if nodeIP == "" {
		return "", ErrEmptyNodeIP
	}

	return nodeIP, nil
}
