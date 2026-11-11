# Microsoft Azure Container Networking

## Azure Network Policy Manager

`azure-npm` Network Policy plugin implements the [Kubernetes Network Policy](https://kubernetes.io/docs/concepts/services-networking/network-policies/)

The plugin is available on Linux and (preview) Windows Server 2022.

Azure-NPM serves as a distributed firewall for the Kubernetes cluster, and it can be easily controlled by `kubectl`.

## Documentation
1. [Secure traffic between pods using network policies in Azure Kubernetes Service (AKS)](https://learn.microsoft.com/en-us/azure/aks/use-network-policies)
2. [Monitor and Visualize Network Configurations with Azure NPM](https://learn.microsoft.com/en-us/azure/virtual-network/kubernetes-network-policies#monitor-and-visualize-network-configurations-with-azure-npm)

## Install
Specify `--network-policy=azure` when creating an AKS cluster. For more information, see the [Microsoft Docs](https://learn.microsoft.com/en-us/azure/aks/use-network-policies#create-an-aks-cluster-and-enable-network-policy).

### Manual Installation
Running the command below will bring up one azure-npm instance on each Kubernetes node.
```
# linux
kubectl apply -f https://raw.githubusercontent.com/Azure/azure-container-networking/master/npm/azure-npm.yaml
# windows
kubectl apply -f https://raw.githubusercontent.com/Azure/azure-container-networking/master/npm/examples/windows/azure-npm.yaml
```
Now you can secure your Kubernetes cluster with Azure-NPM by applying Kubernetes network policies.

## Build
### Linux
`azure-npm` can be built directly from the source code in this repository.
```
make azure-npm
make npm-image
make azure-npm-archive
```
The first command builds the `azure-npm` executable. 
The second command builds the `azure-npm` docker image.
The third command builds the `azure-npm` binary and place it in a tar archive. 
The binaries are placed in the `output` directory.

### Windows
```
$env:ACN_PACKAGE_PATH = "github.com/Azure/azure-container-networking"
$env:NPM_AI_PATH = "$env:ACN_PACKAGE_PATH/npm.aiMetadata"
$env:NPM_AI_ID = "1234abcd-1234-abcd-1234-12345678abcd"
$env:VERSION = "0.0.0"
$env:REPO = "mcr.microsoft.com/azure-npm:" # include colon at end
$env:IMAGE = "$env:REPO$env:VERSION"
docker build `
	-f npm/windows.Dockerfile `
	-t $env:IMAGE `
	--build-arg VERSION=$env:VERSION `
	--build-arg NPM_AI_PATH=$env:NPM_AI_PATH `
	--build-arg NPM_AI_ID=$env:NPM_AI_ID `
	.
docker push $env:IMAGE
echo $env:IMAGE
```

## Conformance validation

The [NPM conformance pipeline](../.pipelines/npm/npm-conformance-tests.yaml) accepts
an optional `npmImage` parameter for validating an existing image on Linux/amd64
and Linux/arm64, including a pre-release Dalec image. The image must be pullable by the test
cluster. Supply a complete reference in any of these forms:

```text
registry.example.com/team/azure-npm:tag
registry.example.com/team/azure-npm@sha256:<digest>
registry.example.com/team/azure-npm:tag@sha256:<digest>
```

The pipeline uses the supplied reference unchanged in every test leg and skips
the NPM image build and push. It does not add a platform prefix, rebuild, retag,
or republish the supplied image. Omitting `npmImage`, setting it to an empty
string, or using `__use-default__` retains the source build and its
`$IMAGE_REGISTRY/azure-npm:linux-amd64-<source-version>` image.

All four existing legs run: foreground, background, Linux stress, and place-first
service tests. Supplying an override also runs background conformance on arm64,
so the supplied image must support both architectures. Source-built validation
remains amd64-only. The stress leg uses the background configuration while
retaining its own cluster and public-IP identity. Each run has a separate resource group,
and each job uses its own kubeconfig. Deployment checks the requested image and
DaemonSet rollout before testing, and logs pod image IDs for runtime evidence.
Image, configuration, or rollout failures fail the job without falling back to a
source-built image.

Clearing `npmImage` restores Linux source-built validation; it is not a substitute
for validating a failed candidate. Windows validation is independently opt-in.
These overrides do not change manual-install manifests or production defaults.

### Windows NPM Lite

The same conformance pipeline accepts an optional `windowsNpmImage`, a complete
immutable Windows/amd64 child reference. Supplying it alone enables Windows Lite
conformance; omitting it, setting it to an empty string, or using
`__use-default__` preserves the existing Linux-only path. Tag-plus-digest
references are preserved unchanged. Preflight
derives the exact Windows2022 `os.version` (`10.0.20348.<revision>`) from the
digest-verified image config before resource creation. There is no separate OS
version parameter.

The independent stage and job are both named `WindowsNPMLite`. It reuses the
legacy Windows pipeline's service connection, disposable test-only admin
fixture, and one-Linux/two-Windows2022-node topology. It does not require a new
secret, variable group, or caller credential. It uses the build-scoped
`e2e-npm-lite-<BuildId>` resource group and fixed `conformance-win-lite`
cluster name, separate from Linux resources. An existing group is never reused;
cleanup requires the matching build/job/attempt ownership tag. Each job uses a
private kubeconfig without writing shared SSH keys or kubeconfig files.

The candidate must be anonymously retrievable by the build agent and pullable by
the new Windows nodes. Preparation verifies the raw child/config digest chain,
Windows/amd64 platform, and LTSC2022 OS version, builds only the shared test
executor, and deploys the supplied NPM image without rebuilding, retagging, or
publishing it. The complete
[Windows configuration](../.pipelines/npm/windows-npm-config.json) is merged with
the versioned Lite patch before deployment; both debug APIs remain disabled.
The derived OS version is logged during preflight, not passed to the executor.
Actual node pulls and observed runtime identity are still required.

For combined Dalec validation, supply the full mirrored immutable index as
`npmImage` and its verified mirrored Windows child as `windowsNpmImage` in one
public run. This retains all existing supplied-image Linux legs, including
arm64. The caller must establish mirror and descriptor continuity. The runner
compares runtime image IDs with the child, never substitutes the index digest,
and does not assume access to the original staging registry.

The acceptance signal is the queued run's ordinary terminal
`completed` / `succeeded` result. There is no Windows result JSON, proof ZIP,
or custom status-property protocol. When Windows is requested, a native
completion guard fails the pipeline unless the Linux conformance, Linux cleanup,
and Windows Lite stages all succeed. The Windows job includes namespace and
owned resource-group cleanup; its private kubeconfig is never published.

Only this mixed route requires a positive stress-worker count and checks each
Linux invocation's native Ginkgo JSON or place-first JUnit report for actual
passing tests. Missing, empty, all-skipped, or failed test reports fail the
invocation even if the engine exits successfully. Native nonzero exit codes
are preserved. Linux-only behavior and scenario selection remain unchanged.
This opt-in Windows leg covers Lite only, not full-NPM stress or Windows servicing;
Hypercube remains with the RP caller.

The reusable [Lite configuration](../test/npm/windows-lite/config.json) and
[version 1 case corpus](../test/npm/windows-lite/cases.v1.json) can also be consumed
by the shared executor on an existing test cluster without the provisioning
stage. The existing Windows unit-test jobs load these same files and check
their translation expectations; those checks are not live image conformance.

Use the configuration and cases embedded in the same compiled source archive,
with ordinary immutable archive integrity and license packaging. Keep that
test-source revision separate from the candidate image's product-source provenance.
No independent corpus/config/source hash inputs are needed. Merge the required
configuration into the Windows NPM configuration only, without changing Linux
configuration or other Windows settings. After rollout, the runner must verify
that every tested Windows NPM pod actually uses the expected `NPM_CONFIG` file
with `EnableV2NPM=true`, `EnableNPMLite=true`, `EnableHTTPDebugAPI=false`, and
`EnablePprof=false`. Missing or false Lite configuration fails validation; the
ordinary `npm.exe start` command alone is not Lite evidence.

Before and after the cases, verify the supplied immutable Windows/amd64 child
image reference, running container image IDs, Windows2022 node platform, ready
DaemonSet pods, and HostProcess configuration. Use the original candidate
reference unchanged, without rebuilding or substituting another repository or
digest. A queue parameter, a ConfigMap alone, or results from non-Lite NPM do not
prove that the candidate ran in Lite mode.

The corpus has `schemaVersion: 1`, `suite: windows-npm-lite`, and standard
Kubernetes NetworkPolicy objects. A consumer must reject unknown versions,
rejection codes, roles, or probe expectations rather than skipping cases. It
creates an isolated test namespace and three Windows workload roles: `subject`,
`allowed-peer`, and `denied-peer`. Label each with `acn-npm-lite-role=<role>`,
provide a TCP listener on port 8080 named `http`, and place the subject on one
Windows node and both peers on the other. Set each policy's namespace to the test namespace.
Replace only the declared `peerCIDRs` sentinels in `ipBlock.cidr` and
`ipBlock.except` with the corresponding workload's actual IPv4 `/32`.

Run all ten cases serially. Before each policy, both probes must connect. After
applying it, wait for the declared `allow` or `deny` outcomes, then delete it and
require both connections to recover before the next case. The four positive
cases cover ingress/egress CIDR allow rules and CIDR exceptions. The six negative
cases cover ingress/egress peer pod selectors, peer namespace selectors, and
named ports. The policy's target `spec.podSelector` is still supported in Lite.

For a rejection case, both connections must remain allowed and each tested
Windows NPM pod must emit the specified `rejectionMessages` reason, correlated
with that policy's name and unique namespace, after its creation. Missing
rejection evidence, stale logs, or unexpected traffic changes fail the case.
Failed traffic alone never counts as proof of unsupported-policy rejection.
Clean up only this run's policies and namespace, including on failure.

#### Existing-cluster executor

The [shared executor](../test/npm/windows-lite) implements these checks as a
standard-library-only Go CLI running on Linux/amd64. It requires an existing
`kubectl`, a private kubeconfig, at least two ready Windows2022 nodes, and an
already deployed Lite NPM DaemonSet. It does not provision infrastructure,
authenticate to registries, or redeploy NPM. Select the Windows DaemonSet
explicitly; its selector and single container are discovered and verified, with
no fallback to the Linux DaemonSet.

Build and run from the chosen immutable source checkout:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=readonly -trimpath \
  -o windows-npm-lite ./test/npm/windows-lite
./windows-npm-lite \
  --kubeconfig "$KUBECONFIG" \
  --daemonset azure-npm-win \
  --npm-image "$WINDOWS_NPM_IMAGE"
```

These are the three required inputs. Optional `--npm-namespace` defaults to
`kube-system`. The deployment adapter, using its own registry credentials,
must verify the raw child/config digest and size chain, Windows/amd64 platform,
and LTSC2022 OS revision before provisioning or deployment. The executor does
not fetch or parse registry metadata again. It requires running container image
IDs to identify the supplied child, never an index or config digest.
Mirror/index continuity remains the caller's responsibility.

ConfigMap mounts remain read-only even when the optional `volumeMount.readOnly`
field is omitted. Sandbox-relative paths accept repeated separators but reject
rooted paths, traversal, and ambiguous subpath mounts. Launchers must set
`NPM_CONFIG` to the sandbox-absolute mounted config path. The current container's
startup log must name that exact absolute path and report all four expected
loaded toggles. The runner reads the actual mounted bytes and compares them
with the ConfigMap. It does not inspect process memory or infer a working
directory.

Initial NPM observation waits up to ninety seconds for complete startup logs.
Configuration fallback, ambiguous or incorrect loaded configuration, and
observation-command failures are fatal. A missing final startup observation or
changed mounted config also fails the run.

The executor uses the immutable Windows2022 agnhost 2.39 child
`registry.k8s.io/e2e-test-images/agnhost@sha256:66482b9d14a4e4d412c18533e738255dddabe6d05da19b2df0476f17bf2324ef`.
Initial role readiness allows ten minutes for cold pulls. Each traffic phase
requires three consecutive matching rounds, five seconds apart, within ninety
seconds. Only the verified TCP client's exact `TIMEOUT` or `REFUSED` result
counts as denial; exec/authentication/transport failures fail validation.

Execution is bounded to thirty-five minutes, reserving the final ninety seconds
for UID-bound namespace cleanup. SIGINT/SIGTERM terminate in-flight kubectl
process groups, including credential-helper descendants, and allow independent
cleanup within that original deadline. Completed kubectl commands also terminate
their process group before reaping the leader, including helpers that retain or
close the output pipes. Cancellation cannot signal a reused process-group ID.
Failure, cancellation, changed runtime identity, or incomplete cleanup returns
a nonzero exit status. Success requires all ten cases, each case's baseline,
applied and recovery traffic, and owned namespace cleanup. The caller propagates
that exit status through its existing test and pipeline machinery.
These checks do not claim full-NPM conformance or Windows servicing coverage.

## Usage
[Microsoft Docs](https://learn.microsoft.com/en-us/azure/aks/use-network-policies#verify-network-policy-setup) has a detailed step by step example on how to use Kubernetes network policy.

## Troubleshooting
When `azure-npm` isn't working as expected, try to **delete all networkpolicies and apply them again**.
Also, a good practice is to merge all network policies targeting the same set of pods/labels into one yaml file.
This way, operators can keep the minimum number of network policies and makes it easier for operators to troubleshoot.

### Linux
NPM adds firewall rules via `iptables` and `ipset`. You can examine the configuration on a given node with:
- `kubectl exec -it -n kube-system $npmPod -- iptables -vnL`
- `kubectl exec -it -n kube-system $npmPod -- ipset -L`

### Windows
NPM adds firewall rules via HNS. You can examine the configuration on a given node with:
- ACLs applied on Pod Endpoints: `kubectl exec -n kube-system $npmWinPod -- Get-HNSEndpoint`
- SetPolicies are like ipsets: `(Get-HNSNetwork | ? Name -Like Azure).Policies`
