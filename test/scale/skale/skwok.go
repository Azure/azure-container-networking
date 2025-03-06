// embed kwok
package main

import (
	"context"

	"sigs.k8s.io/kwok/pkg/kwok/cmd"
)

func init() {
	kwok := cmd.NewCommand(context.Background())
	rootcmd.AddCommand(kwok)
}
