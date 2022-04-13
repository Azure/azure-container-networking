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
		Use:   "azurenpm",
		Short: "A kubectl plugin for inspecting your ingress-nginx deployments",
	}

	// Respect some basic kubectl flags like --namespace
	flags := genericclioptions.NewConfigFlags(true)
	flags.AddFlags(rootCmd.PersistentFlags())

	rootCmd.AddCommand(TuplesCmd(flags))

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func TuplesCmd(flags *genericclioptions.ConfigFlags) *cobra.Command {
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
			pod, err := GetNamedPod(flags, "azure-npm-k9vdz")
			if err != nil {
				return fmt.Errorf("error getting pod %w", err)
			}
			log.Printf("pod: %+v", pod)
			return nil
		},
	}

	KubernetesConfigFlags = genericclioptions.NewConfigFlags(false)
	KubernetesConfigFlags.AddFlags(cmd.Flags())

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

// getKubectlConfigFlags serializes the parsed flag struct back into a series of command line args
// that can then be passed to kubectl. The mirror image of
// https://github.com/kubernetes/cli-runtime/blob/master/pkg/genericclioptions/config_flags.go#L251
func getKubectlConfigFlags(flags *genericclioptions.ConfigFlags) []string {
	out := []string{}
	o := &out

	appendStringFlag(o, flags.KubeConfig, "kubeconfig")
	appendStringFlag(o, flags.CacheDir, "cache-dir")
	appendStringFlag(o, flags.CertFile, "client-certificate")
	appendStringFlag(o, flags.KeyFile, "client-key")
	appendStringFlag(o, flags.BearerToken, "token")
	appendStringFlag(o, flags.Impersonate, "as")
	appendStringArrayFlag(o, flags.ImpersonateGroup, "as-group")
	appendStringFlag(o, flags.Username, "username")
	appendStringFlag(o, flags.Password, "password")
	appendStringFlag(o, flags.ClusterName, "cluster")
	appendStringFlag(o, flags.AuthInfoName, "user")
	//appendStringFlag(o, flags.Namespace, "namespace")
	appendStringFlag(o, flags.Context, "context")
	appendStringFlag(o, flags.APIServer, "server")
	appendBoolFlag(o, flags.Insecure, "insecure-skip-tls-verify")
	appendStringFlag(o, flags.CAFile, "certificate-authority")
	appendStringFlag(o, flags.Timeout, "request-timeout")

	return out
}
