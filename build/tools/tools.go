//+build tools

package tools

import (
	_ "github.com/AlekSi/gocov-xml"
	_ "github.com/axw/gocov/gocov"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/jstemmer/go-junit-report"
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
)
