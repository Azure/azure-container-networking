// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package main

const (
	defaultEndpointStorePath = "/k/azurecns/"
	defaultStateStorePath    = ""
)

func resolvePersistentStatePaths(stateDirectory, endpointDirectory string) persistentStatePaths {
	statePrefix := stateDirectory
	if stateDirectory == "" {
		stateDirectory = "."
	}
	if endpointDirectory == "" {
		endpointDirectory = defaultEndpointStorePath
	}
	return persistentStatePaths{
		stateDirectory:    stateDirectory,
		stateFile:         statePrefix + name + ".json",
		databaseFile:      statePrefix + name + ".db",
		stateLockFile:     name + persistentStateLockExtension,
		endpointDirectory: endpointDirectory,
		endpointFile:      endpointDirectory + endpointStoreName + ".json",
		endpointLockFile:  endpointStoreName + persistentStateLockExtension,
	}
}
