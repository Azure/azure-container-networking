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
	"k8s.io/client-go/tools/clientcmd/api"
)

// NamespaceOptions provides information required to update
// the current context on a user's KUBECONFIG
type NPMOptions struct {
	configFlags *genericclioptions.ConfigFlags

	resultingContext     *api.Context
	resultingContextName string

	userSpecifiedCluster   string
	userSpecifiedContext   string
	userSpecifiedAuthInfo  string
	userSpecifiedNamespace string

	rawConfig      api.Config
	listNamespaces bool
	args           []string

	genericclioptions.IOStreams
}

var (
	KubernetesConfigFlags *genericclioptions.ConfigFlags
)

// NewNamespaceOptions provides an instance of NamespaceOptions with default values
func NewNPMOptions(streams genericclioptions.IOStreams) *NPMOptions {
	return &NPMOptions{
		configFlags: genericclioptions.NewConfigFlags(true),

		IOStreams: streams,
	}
}

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
		PreRun: func(cmd *cobra.Command, args []string) {
			viper.BindPFlags(cmd.Flags())
		},
		RunE: func(cmd *cobra.Command, args []string) error {

			src, err := cmd.Flags().GetString("src")
			if src == "" {
				return fmt.Errorf("failed to get source with err %w", err)
			}
			dst, err := cmd.Flags().GetString("dst")
			if dst == "" {
				return fmt.Errorf("failed to get destination with err %w", err)
			}

			args = append(args, "/usr/bin/azure-npm", "debug", "gettuples", "-s", src, "-d", dst)
			log.Printf("args %+v", args)
			err = exec(flags, *pod, *deployment, *selector, args, opts)
			if err != nil {
				log.Printf("exec failed with error %+v", err)
			}
			return nil
		},
	}

	KubernetesConfigFlags = genericclioptions.NewConfigFlags(false)

	pod = AddPodFlag(cmd)
	deployment = AddDeploymentFlag(cmd)
	selector = AddSelectorFlag(cmd)
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

func exec(flags *genericclioptions.ConfigFlags, podName string, deployment string, selector string, cmd []string, opts execFlags) error {
	pod, err := ChoosePod(flags, podName, deployment, selector)
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
	return Exec(flags, args)
}

// Exec replaces the current process with a kubectl invocation
func Exec(flags *genericclioptions.ConfigFlags, args []string) error {
	kArgs := getKubectlConfigFlags(flags)
	kArgs = append(kArgs, args...)
	return execCommand(append([]string{"kubectl"}, kArgs...))
}

// Replaces the currently running process with the given command
func execCommand(args []string) error {
	path, err := osexec.LookPath(args[0])
	if err != nil {
		return err
	}
	args[0] = path

	env := os.Environ()
	return syscall.Exec(path, args, env)
}

func appendStringFlag(out *[]string, in *string, flag string) {
	if in != nil && *in != "" {
		*out = append(*out, fmt.Sprintf("--%v=%v", flag, *in))
	}
}

func appendBoolFlag(out *[]string, in *bool, flag string) {
	if in != nil {
		*out = append(*out, fmt.Sprintf("--%v=%v", flag, *in))
	}
}

func appendStringArrayFlag(out *[]string, in *[]string, flag string) {
	if in != nil && len(*in) > 0 {
		*out = append(*out, fmt.Sprintf("--%v=%v'", flag, strings.Join(*in, ",")))
	}
}

// AddPodFlag adds a --pod flag to a cobra command
func AddPodFlag(cmd *cobra.Command) *string {
	v := ""
	cmd.Flags().StringVar(&v, "pod", "", "Query a particular ingress-nginx pod")
	return &v
}

// AddDeploymentFlag adds a --deployment flag to a cobra command
func AddDeploymentFlag(cmd *cobra.Command) *string {
	v := ""
	cmd.Flags().StringVar(&v, "deployment", "azure-npm", "The name of the ingress-nginx deployment")
	return &v
}

// AddSelectorFlag adds a --selector flag to a cobra command
func AddSelectorFlag(cmd *cobra.Command) *string {
	v := ""
	cmd.Flags().StringVarP(&v, "selector", "l", "", "Selector (label query) of the ingress-nginx pod")
	return &v
}

// getKubectlConfigFlags serializes the parsed flag struct back into a series of command line args
// that can then be passed to kubectl. The mirror image of
// https://github.com/kubernetes/cli-runtime/blob/master/pkg/genericclioptions/config_flags.go#L251
func getKubectlConfigFlags(flags *genericclioptions.ConfigFlags) []string {

	return []string{}
}
