// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package common

import (
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/log"
)

const (
	metadataURL                     = "http://169.254.169.254/metadata/instance?api-version=2017-08-01&format=json"
	azCloudUrl                      = "http://169.254.169.254/metadata/instance/compute/azEnvironment?api-version=2018-10-01&format=text"
	httpConnectionTimeout           = 7
	headerTimeout                   = 7
	RegisterNodeURLFmt              = "%s/%s/node/%s%s"
	SyncNodeNetworkContainersURLFmt = "%s/%s/node/%s%s"
	FiveSeconds                     = 5 * time.Second
	JsonContent                     = "application/json; charset=UTF-8"
	ContentType                     = "Content-Type"
)

// XmlDocument - Azure host agent XML document format.
type XmlDocument struct {
	XMLName   xml.Name `xml:"Interfaces"`
	Interface []struct {
		XMLName    xml.Name `xml:"Interface"`
		MacAddress string   `xml:"MacAddress,attr"`
		IsPrimary  bool     `xml:"IsPrimary,attr"`

		IPSubnet []struct {
			XMLName xml.Name `xml:"IPSubnet"`
			Prefix  string   `xml:"Prefix,attr"`

			IPAddress []struct {
				XMLName   xml.Name `xml:"IPAddress"`
				Address   string   `xml:"Address,attr"`
				IsPrimary bool     `xml:"IsPrimary,attr"`
			}
		}
	}
}

// Metadata retrieved from wireserver
type Metadata struct {
	Location             string `json:"location"`
	VMName               string `json:"name"`
	Offer                string `json:"offer"`
	OsType               string `json:"osType"`
	PlacementGroupID     string `json:"placementGroupId"`
	PlatformFaultDomain  string `json:"platformFaultDomain"`
	PlatformUpdateDomain string `json:"platformUpdateDomain"`
	Publisher            string `json:"publisher"`
	ResourceGroupName    string `json:"resourceGroupName"`
	Sku                  string `json:"sku"`
	SubscriptionID       string `json:"subscriptionId"`
	Tags                 string `json:"tags"`
	OSVersion            string `json:"version"`
	VMID                 string `json:"vmId"`
	VMSize               string `json:"vmSize"`
	KernelVersion        string
}

// This is how metadata server returns in response for querying metadata
type metadataWrapper struct {
	Metadata Metadata `json:"compute"`
}

// Creating http client object to be reused instead of creating one every time.
// This helps make use of the cached tcp connections.
// Clients are safe for concurrent use by multiple goroutines.
var httpClient *http.Client

// InitHttpClient initializes the httpClient object
func InitHttpClient(
	connectionTimeoutSec int,
	responseHeaderTimeoutSec int) *http.Client {
	log.Printf("[Utils] Initializing HTTP client with connection timeout: %d, response header timeout: %d",
		connectionTimeoutSec, responseHeaderTimeoutSec)
	httpClient = &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: time.Duration(connectionTimeoutSec) * time.Second,
			}).DialContext,
			ResponseHeaderTimeout: time.Duration(responseHeaderTimeoutSec) * time.Second,
		},
	}

	return httpClient
}

// GetHttpClient returns the singleton httpClient object
func GetHttpClient() *http.Client {
	return httpClient
}

// LogNetworkInterfaces logs the host's network interfaces in the default namespace.
func LogNetworkInterfaces() {
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Printf("Failed to query network interfaces, err:%v", err)
		return
	}

	for _, iface := range interfaces {
		addrs, _ := iface.Addrs()
		log.Printf("[net] Network interface: %+v with IP: %+v", iface, addrs)
	}
}

func IpToInt(ip net.IP) uint32 {
	if len(ip) == 16 {
		return binary.BigEndian.Uint32(ip[12:16])
	}

	return binary.BigEndian.Uint32(ip)
}

func GetInterfaceSubnetWithSpecificIP(ipAddr string) *net.IPNet {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Printf("InterfaceAddrs failed with %+v", err)
		return nil
	}

	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				if ipnet.IP.String() == ipAddr {
					return ipnet
				}
			}
		}
	}

	return nil
}

func StartProcess(path string, args []string) error {
	attr := os.ProcAttr{
		Env: os.Environ(),
		Files: []*os.File{
			os.Stdin,
			nil,
			nil,
		},
	}

	processArgs := append([]string{path}, args...)
	process, err := os.StartProcess(path, processArgs, &attr)
	if err == nil {
		// Release detaches the process
		return process.Release()
	}

	return err
}

// GetHostMetadata - retrieve VM metadata from wireserver
func GetHostMetadata(fileName string) (Metadata, error) {
	content, err := ioutil.ReadFile(fileName)
	if err == nil {
		var metadata Metadata
		if err = json.Unmarshal(content, &metadata); err == nil {
			return metadata, nil
		}
	}

	log.Printf("[Telemetry] Request metadata from wireserver")

	req, err := http.NewRequest("GET", metadataURL, nil)
	if err != nil {
		return Metadata{}, err
	}

	req.Header.Set("Metadata", "True")

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: time.Duration(httpConnectionTimeout) * time.Second,
			}).DialContext,
			ResponseHeaderTimeout: time.Duration(headerTimeout) * time.Second,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return Metadata{}, err
	}

	defer resp.Body.Close()

	metareport := metadataWrapper{}

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("[Telemetry] Request failed with HTTP error %d", resp.StatusCode)
	} else if resp.Body != nil {
		err = json.NewDecoder(resp.Body).Decode(&metareport)
		if err != nil {
			err = fmt.Errorf("[Telemetry] Unable to decode response body due to error: %s", err.Error())
		}
	} else {
		err = fmt.Errorf("[Telemetry] Response body is empty")
	}

	return metareport.Metadata, err
}

// SaveHostMetadata - save metadata got from wireserver to json file
func SaveHostMetadata(metadata Metadata, fileName string) error {
	dataBytes, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("[Telemetry] marshal data failed with err %+v", err)
	}

	if err = ioutil.WriteFile(fileName, dataBytes, 0o644); err != nil {
		log.Printf("[Telemetry] Writing metadata to file failed: %v", err)
	}

	return err
}

func GetAzureCloud(url string) (string, error) {
	if url == "" {
		url = azCloudUrl
	}

	log.Printf("GetAzureCloud querying url: %s", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Metadata", "True")

	client := InitHttpClient(httpConnectionTimeout, headerTimeout)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Bad http status:%v", resp.Status)
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(bodyBytes)), nil
}

func GetExecutableDirectory() (string, error) {
	var (
		dir string
		ex  string
		err error
	)
	ex, err = os.Executable()
	if err == nil {
		dir = filepath.Dir(ex)
	} else {
		var exReal string
		// If a symlink was used to start the process, depending on the operating system,
		// the result might be the symlink or the path it pointed to.
		// filepath.EvalSymlinks returns stable results
		exReal, err = filepath.EvalSymlinks(ex)
		if err == nil {
			dir = filepath.Dir(exReal)
		}
	}

	return dir, err
}
