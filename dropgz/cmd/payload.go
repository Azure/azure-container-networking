package cmd

import (
	"fmt"

	"github.com/Azure/azure-container-networking/dropgz/pkg/embed"
	"github.com/Azure/azure-container-networking/dropgz/pkg/hash"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// list subcommand
var list = &cobra.Command{
	Use: "list",
	RunE: func(*cobra.Command, []string) error {
		if err := setLogLevel(); err != nil {
			return err
		}
		contents, err := embed.Contents()
		if err != nil {
			return err
		}
		for _, c := range contents {
			fmt.Printf("\t%s\n", c)
		}
		return nil
	},
}

func checksum(srcs, dests []string) error {
	if len(srcs) != len(dests) {
		return errors.Wrapf(embed.ErrArgsMismatched, "%d and %d", len(srcs), len(dests))
	}
	rc, err := embed.Extract("sum.txt")
	if err != nil {
		return errors.Wrap(err, "failed to extract checksum file")
	}
	defer rc.Close()

	checksums, err := hash.Parse(rc)
	if err != nil {
		return errors.Wrap(err, "failed to parse checksums")
	}
	for i := range srcs {
		valid, err := checksums.Check(srcs[i], dests[i])
		if err != nil {
			return errors.Wrapf(err, "failed to validate file at %s", dests[i])
		}
		if !valid {
			return errors.Errorf("%s checksum validation failed", dests[i])
		}
	}
	return nil
}

var (
	skipVerify bool
	outs       []string
)

// deploy subcommand
var deploy = &cobra.Command{
	Use: "deploy",
	RunE: func(_ *cobra.Command, srcs []string) error {
		if err := setLogLevel(); err != nil {
			return err
		}
		if len(outs) == 0 {
			outs = srcs
		}
		if len(srcs) != len(outs) {
			return errors.Wrapf(embed.ErrArgsMismatched, "%d files, %d outputs", len(srcs), len(outs))
		}
		log := z.With(zap.Strings("sources", srcs), zap.Strings("outputs", outs), zap.String("cmd", "deploy"))
		if err := embed.Deploy(log, srcs, outs); err != nil {
			return errors.Wrapf(err, "failed to deploy %s", srcs)
		}
		log.Info("successfully wrote files")
		if skipVerify {
			return nil
		}
		if err := checksum(srcs, outs); err != nil {
			return err
		}
		log.Info("verified file integrity")
		return nil
	},
	Args: cobra.OnlyValidArgs,
}

// verify subcommand
var verify = &cobra.Command{
	Use: "verify",
	RunE: func(_ *cobra.Command, srcs []string) error {
		if err := setLogLevel(); err != nil {
			return err
		}
		if len(outs) == 0 {
			outs = srcs
		}
		if len(srcs) != len(outs) {
			return errors.Wrapf(embed.ErrArgsMismatched, "%d sources, %d destinations", len(srcs), len(outs))
		}
		log := z.With(zap.Strings("sources", srcs), zap.Strings("outputs", outs), zap.String("cmd", "verify"))
		if err := checksum(srcs, outs); err != nil {
			return err
		}
		log.Info("verified files")
		return nil
	},
	Args: cobra.OnlyValidArgs,
}

func init() {
	root.AddCommand(list)

	verify.ValidArgs, _ = embed.Contents()
	verify.Flags().StringSliceVarP(&outs, "output", "o", []string{}, "output file path")
	root.AddCommand(verify)

	deploy.ValidArgs, _ = embed.Contents() // setting this after the command is initialized is required
	deploy.Flags().BoolVar(&skipVerify, "skip-verify", false, "set to disable checksum validation")
	deploy.Flags().StringSliceVarP(&outs, "output", "o", []string{}, "output file path")
	root.AddCommand(deploy)
}
