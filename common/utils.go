// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package common

import (
	"bufio"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/Azure/azure-container-networking/log"
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

func CheckIfFileExists(filepath string) (bool, error) {
	_, err := os.Stat(filepath)
	if err == nil {
		return true, nil
	}

	if os.IsNotExist(err) {
		return false, nil
	}

	return true, err
}

func CreateDirectory(dirPath string) error {
	var err error

	if dirPath == "" {
		log.Printf("dirPath is empty, nothing to create.")
		return nil
	}

	isExist, _ := CheckIfFileExists(dirPath)
	if !isExist {
		err = os.Mkdir(dirPath, os.ModePerm)
	}

	return err
}

func IpToInt(ip net.IP) uint32 {
	if len(ip) == 16 {
		return binary.BigEndian.Uint32(ip[12:16])
	}

	return binary.BigEndian.Uint32(ip)
}

func GetInterfaceSubnetWithSpecificIp(ipAddr string) *net.IPNet {
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
	var attr = os.ProcAttr{
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

// ReadFileByLines reads file line by line and return array of lines.
func ReadFileByLines(filename string) ([]string, error) {
	var (
		lineStrArr []string
	)

	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("Error opening %s file error %v", filename, err)
	}

	defer f.Close()

	r := bufio.NewReader(f)

	for {
		lineStr, err := r.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				return nil, fmt.Errorf("Error reading %s file error %v", filename, err)
			}

			lineStrArr = append(lineStrArr, lineStr)
			break
		}

		lineStrArr = append(lineStrArr, lineStr)
	}

	return lineStrArr, nil
}
