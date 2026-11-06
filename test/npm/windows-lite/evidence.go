//go:build linux

package main

import (
	"net/netip"
	"time"
)

type (
	caseStatus string
	npmMode    string
)

const (
	passed caseStatus = "passed"
	lite   npmMode    = "lite"
)

type corpusEvidence struct {
	Suite         string `json:"suite"`
	SchemaVersion int    `json:"schemaVersion"`
	SourceVersion string `json:"sourceVersion"`
	ConfigSHA256  string `json:"configSha256"`
	CasesSHA256   string `json:"casesSha256"`
}

type podIdentity struct {
	PodName       string    `json:"podName"`
	PodUID        string    `json:"podUID"`
	NodeName      string    `json:"nodeName"`
	NodeUID       string    `json:"nodeUID"`
	ContainerName string    `json:"containerName"`
	ContainerID   string    `json:"containerID"`
	RestartCount  int       `json:"restartCount"`
	Image         string    `json:"image"`
	ImageID       string    `json:"imageID"`
	RunningSince  time.Time `json:"runningSince"`
	Ready         bool      `json:"ready"`
	HostProcess   bool      `json:"hostProcess"`
	HostNetwork   bool      `json:"hostNetwork"`
	RunAsUserName string    `json:"runAsUserName"`
	WindowsBuild  string    `json:"windowsBuild"`
	KernelVersion string    `json:"kernelVersion"`
	OSImage       string    `json:"osImage"`
}

type npmObservation struct {
	podIdentity
	processObservation
	ConfigMapName       string  `json:"configMapName"`
	ConfigMapUID        string  `json:"configMapUID"`
	ConfigPath          string  `json:"configPath"`
	MountedConfigSHA256 string  `json:"mountedConfigSha256"`
	ObservedConfig      toggles `json:"observedConfig"`
	UsingConfigLine     string  `json:"usingConfigLine"`
	LoadedConfigLine    string  `json:"loadedConfigLine"`
}

type processObservation struct {
	ExecutablePath   string    `json:"executablePath"`
	ProcessID        uint32    `json:"processID"`
	ProcessStartedAt time.Time `json:"processStartedAt"`
}

type roleObservation struct {
	podIdentity
	IP netip.Addr `json:"ip"`
}

type probeObservation struct {
	probe
	Observed verdict `json:"observed"`
}

type trafficEvidence struct {
	ConsecutiveRounds int                  `json:"consecutiveRounds"`
	Probes            []probeObservation   `json:"probes"`
	Rounds            [][]probeObservation `json:"rounds"`
}

type rejectionEvidence struct {
	Pod       podIdentity `json:"pod"`
	Timestamp time.Time   `json:"timestamp"`
	Reason    rejection   `json:"reason"`
	Line      string      `json:"line"`
	LogSHA256 string      `json:"logSha256"`
}

type caseEvidence struct {
	PolicyName      string              `json:"policyName"`
	PolicyUID       string              `json:"policyUID"`
	PolicyCreatedAt time.Time           `json:"policyCreatedAt"`
	Baseline        trafficEvidence     `json:"baseline"`
	Applied         trafficEvidence     `json:"applied"`
	Recovery        trafficEvidence     `json:"recovery"`
	Rejections      []rejectionEvidence `json:"rejections"`
}

type runResult struct {
	SchemaVersion     int                     `json:"schemaVersion"`
	Suite             string                  `json:"suite"`
	Status            caseStatus              `json:"status"`
	RunID             int64                   `json:"runId"`
	SourceVersion     string                  `json:"sourceVersion"`
	RunnerSHA256      string                  `json:"runnerSha256"`
	Image             string                  `json:"image"`
	ImageDigest       string                  `json:"imageDigest"`
	ImageConfigDigest string                  `json:"imageConfigDigest"`
	Platform          string                  `json:"platform"`
	OSVersion         string                  `json:"osVersion"`
	Mode              npmMode                 `json:"mode"`
	LiteModeVerified  bool                    `json:"liteModeVerified"`
	ObservedConfig    toggles                 `json:"observedConfig"`
	Corpus            corpusEvidence          `json:"corpus"`
	CaseResults       map[string]caseStatus   `json:"caseResults"`
	CaseEvidence      map[string]caseEvidence `json:"caseEvidence"`
	Namespace         string                  `json:"namespace"`
	NamespaceUID      string                  `json:"namespaceUID"`
	CleanupSucceeded  bool                    `json:"cleanupSucceeded"`
	StartedAt         time.Time               `json:"startedAt"`
	FinishedAt        time.Time               `json:"finishedAt"`
	NPMPods           struct {
		Before []npmObservation `json:"before"`
		After  []npmObservation `json:"after"`
	} `json:"npmPods"`
	RolePods map[role]roleObservation `json:"rolePods"`
}
