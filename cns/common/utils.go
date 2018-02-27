package common

import (
	"fmt"
	"os/exec"

	"github.com/Azure/azure-container-networking/log"
)

func ExecuteShellCommand(command string) error {
	log.Printf("[Azure-CNS] %s", command)
	cmd := exec.Command("sh", "-c", command)
	err := cmd.Start()
	if err != nil {
		return err
	}
	return cmd.Wait()
}

func SetOutboundSNAT(subnet string) error {
	cmd := fmt.Sprintf("iptables -t nat -A POSTROUTING -m iprange ! --dst-range 168.63.129.16 -m addrtype ! --dst-type local ! -d %v -j MASQUERADE",
		subnet)
	err := ExecuteShellCommand(cmd)
	if err != nil {
		log.Printf("SNAT Iptable rule was not set")
		return err
	}
	return nil
}
