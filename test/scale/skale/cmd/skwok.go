// embed kwok
package cmd

import (
	"context"

	"sigs.k8s.io/kwok/pkg/kwok/cmd"
)

func init() {
	kwok := cmd.NewCommand(context.Background())
	Skale.AddCommand(kwok)
}
