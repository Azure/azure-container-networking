package manifests

import "embed"

//go:embed cilium/*
var CiliumManifests embed.FS

var CiliumV14Directories = []string{
	"cilium/v1.14/cns",
	"cilium/v1.14/agent",
	"cilium/v1.14/ipmasq",
	"cilium/v1.14/operator",
}
