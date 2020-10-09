package main

import (
	"log"
	"os"
)

func main() {
	log.Println("Getting installer config from env...")
	envs, err := getInstallerConfigFromEnv()
	if err != nil {
		log.Printf("Failed to get environmental variables with err: %v", err)
		os.Exit(1)
	}

	log.Printf("Installing Azure CNI to %s...\n", envs.dstBinDir)
	err = install(envs)
	if err != nil {
		log.Printf("Failed to install CNI and conflists with err: %v", err)
		os.Exit(1)
	}

	log.Println("Azure CNI and Conflists installed")

	// this loop exists for when the logfile gets rotated, and tail loses the original file
	for {
		err := follow(envs.logFile)
		if err != nil {
			log.Print(err)
		}
	}
}
