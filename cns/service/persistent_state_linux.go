// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package main

import "github.com/Azure/azure-container-networking/platform"

const (
	defaultEndpointStorePath = "/var/run/azure-cns/"
	defaultStateStorePath    = "/var/lib/azure-network/"
)

func resolvePersistentStatePaths(stateDirectory, endpointDirectory string) persistentStatePaths {
	if stateDirectory == "" {
		stateDirectory = defaultStateStorePath
	}
	if endpointDirectory == "" {
		endpointDirectory = defaultEndpointStorePath
	}
	return persistentStatePaths{
		stateDirectory:    stateDirectory,
		stateFile:         stateDirectory + name + ".json",
		databaseFile:      stateDirectory + name + ".db",
		stateLockFile:     platform.CNILockPath + name + persistentStateLockExtension,
		endpointDirectory: endpointDirectory,
		endpointFile:      endpointDirectory + endpointStoreName + ".json",
		endpointLockFile:  platform.CNILockPath + endpointStoreName + persistentStateLockExtension,
	}
}
