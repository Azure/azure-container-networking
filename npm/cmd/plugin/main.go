package main

import (
	"fmt"
	"log"
	"os"
	osexec "os/exec"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var KubernetesConfigFlags *genericclioptions.ConfigFlags

func main() {
	rootCmd := &cobra.Command{
		Use:   "azure-npm",
		Short: "A kubectl plugin for inspecting Azure NPM",
	}

	// Respect some basic kubectl flags like --namespace
	flags := genericclioptions.NewConfigFlags(true)
	flags.AddFlags(rootCmd.PersistentFlags())

	rootCmd.AddCommand(TuplesCmd(flags))

	cobra.CheckErr(rootCmd.Execute())
}

func TuplesCmd(flags *genericclioptions.ConfigFlags) *cobra.Command {
	opts := execFlags{}
	var pod, deployment, selector *string

	cmd := &cobra.Command{
		Use:           "gettuples",
		Short:         "",
		Long:          `.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			err := viper.BindPFlags(cmd.Flags())
			if err != nil {
				return fmt.Errorf("failed to bind pflags with err %w", err)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {

			src, err := cmd.Flags().GetString("src")
			if src == "" {
				return fmt.Errorf("cannot gettuples with empty source")
			}
			if err != nil {
				return fmt.Errorf("failed to get source with err %w", err)
			}

			dst, err := cmd.Flags().GetString("dst")
			if dst == "" {
				return fmt.Errorf("cannot gettuples with empty destination")
			}
			if err != nil {
				return fmt.Errorf("failed to get destination with err %w", err)
			}

			args = append(args, "/usr/bin/azure-npm", "debug", "gettuples", "-s", src, "-d", dst)
			log.Printf("args %+v", args)
			err = kubectlExec(flags, *pod, *deployment, *selector, args, opts)
			if err != nil {
				log.Printf("exec failed with error %+v", err)
			}
			return nil
		},
	}

	KubernetesConfigFlags = genericclioptions.NewConfigFlags(false)

	pod = addPodFlag(cmd)
	deployment = addDeploymentFlag(cmd)
	selector = addSelectorFlag(cmd)
	cmd.Flags().BoolVarP(&opts.TTY, "tty", "t", false, "Stdin is a TTY")
	cmd.Flags().BoolVarP(&opts.Stdin, "stdin", "i", false, "Pass stdin to the container")
	cmd.Flags().StringP("src", "", "", "set the source")
	cmd.Flags().StringP("dst", "", "", "set the destination")
	cmd.Flags().StringP("cache-file", "c", "", "Set the NPM cache file path (optional, but required when using an iptables save file)")

	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	return cmd
}

type execFlags struct {
	TTY   bool
	Stdin bool
}

func kubectlExec(flags *genericclioptions.ConfigFlags, podName, deployment, selector string, cmd []string, opts execFlags) error {
	pod, err := choosePod(flags, podName, deployment, selector)
	if err != nil {
		return err
	}

	args := []string{"exec"}
	if opts.TTY {
		args = append(args, "-t")
	}
	if opts.Stdin {
		args = append(args, "-i")
	}

	args = append(args, []string{"-n", pod.Namespace, pod.Name, "--"}...)
	args = append(args, cmd...)
	kubectlArgs := getKubectlConfigFlags(flags)
	kubectlArgs = append(kubectlArgs, args...)
	return execCommand(append([]string{"kubectl"}, kubectlArgs...))
}

// Replaces the currently running process with the given command
func execCommand(args []string) error {
	path, err := osexec.LookPath(args[0])
	if err != nil {
		return fmt.Errorf("failed to exec with err: %w", err)
	}
	args[0] = path

	env := os.Environ()
	if err := syscall.Exec(path, args, env); err != nil {
		return fmt.Errorf("failed to exec with err %w", err)
	}
	return nil
}

// addPodFlag adds a --pod flag to a cobra command
func addPodFlag(cmd *cobra.Command) *string {
	v := ""
	cmd.Flags().StringVar(&v, "pod", "", "Query a particular azure-npm pod")
	return &v
}

// addDeploymentFlag adds a --deployment flag to a cobra command
func addDeploymentFlag(cmd *cobra.Command) *string {
	v := ""
	cmd.Flags().StringVar(&v, "deployment", "azure-npm", "The name of the azure-npm deployment")
	return &v
}

// addSelectorFlag adds a --selector flag to a cobra command
func addSelectorFlag(cmd *cobra.Command) *string {
	v := ""
	cmd.Flags().StringVarP(&v, "selector", "l", "", "Selector (label query) of the azure-npm pod")
	return &v
}

// getKubectlConfigFlags serializes the parsed flag struct back into a series of command line args
// that can then be passed to kubectl. The mirror image of
// https://github.com/kubernetes/cli-runtime/blob/master/pkg/genericclioptions/config_flags.go#L251
func getKubectlConfigFlags(flags *genericclioptions.ConfigFlags) []string {
	return []string{}
}
