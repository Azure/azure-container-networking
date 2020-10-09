package main

import (
	"fmt"
	"log"
	"os"

	"github.com/nxadm/tail"
)

func main() {
	fmt.Println("Getting installer config from env...")
	envs, err := getInstallerConfigFromEnv()
	if err != nil {
		fmt.Printf("Failed to get environmental variables with err: %v", err)
		os.Exit(1)
	}

	log.Printf("Installing Azure CNI to %s...\n", envs.dstBinDir)
	install(envs)
	log.Println("Installed")

	t, err := tail.TailFile(envs.logFile, tail.Config{Follow: true})

	if err != nil {
		log.Print(err)
		os.Exit(1)
	}

	fmt.Println("Watching logfile:")
	for line := range t.Lines {
		fmt.Println(line.Text)
	}

}
