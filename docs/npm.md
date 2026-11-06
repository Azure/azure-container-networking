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

This pipeline does not validate Windows. Clearing the override restores
source-built validation; it is not a substitute for validating a failed candidate.
The override does not change the manual-install manifest or production defaults.

### Windows NPM Lite

The reusable [Lite configuration](../test/npm/windows-lite/config.json) and
[version 1 case corpus](../test/npm/windows-lite/cases.v1.json) are inputs to a
Windows NPM Lite test runner on an existing test cluster. They do not provision a
cluster, build an image, or add Windows legs to the Linux pipeline. The existing
Windows unit-test jobs load these same files and check their translation
expectations; those checks are not live image conformance.

Pin both files to the same published ACN commit. Keep that test-source revision
separate from the candidate image's product-source provenance. Merge the required
configuration into the Windows NPM configuration only, without changing Linux
configuration or other Windows settings. After rollout, the runner must verify
that every tested Windows NPM pod actually uses the expected `NPM_CONFIG` file
with `EnableV2NPM=true`, `EnableNPMLite=true`, `EnableHTTPDebugAPI=false`, and
`EnablePprof=false`. Missing or false Lite configuration fails validation; the
ordinary `npm.exe start` command alone is not Lite evidence.

Before and after the cases, verify the supplied immutable Windows/amd64 child
image reference, running container image IDs, Windows image OS version, ready
DaemonSet pods, and HostProcess configuration. Use the original candidate
reference unchanged, without rebuilding or substituting another repository or
digest. Record these observations together with the corpus commit and per-case
results. A queue parameter, a ConfigMap alone, or results from non-Lite NPM do not
prove that the candidate ran in Lite mode.

The corpus has `schemaVersion: 1`, `suite: windows-npm-lite`, and standard
Kubernetes NetworkPolicy objects. A consumer must reject unknown versions,
rejection codes, roles, or probe expectations rather than skipping cases. It
creates an isolated test namespace and three Windows workload roles: `subject`,
`allowed-peer`, and `denied-peer`. Label each with `acn-npm-lite-role=<role>`,
provide a TCP listener on port 8080 named `http`, and place the subject and peers
on different Windows nodes. Set each policy's namespace to the test namespace.
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

Build and run from the exact executable-source checkout:

```bash
SOURCE=$(git rev-parse HEAD)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=readonly -trimpath \
  -ldflags "-X main.sourceVersion=$SOURCE" \
  -o windows-npm-lite ./test/npm/windows-lite
./windows-npm-lite \
  --kubeconfig "$KUBECONFIG" \
  --daemonset azure-npm-win --npm-namespace kube-system \
  --npm-image "$WINDOWS_NPM_IMAGE" --windows-os-version "$WINDOWS_OS_VERSION" \
  --image-manifest "$RAW_CHILD_MANIFEST" --image-config "$RAW_IMAGE_CONFIG" \
  --source-version "$SOURCE" --run-id "$BUILD_ID" --output "$RESULT_FILE"
```

The caller supplies the raw child manifest and image-config bytes in regular
files after its own authenticated retrieval. The executor verifies their digest/size chain and exact
Windows/amd64 OS version, then requires running container image IDs to identify
that child. An index digest or config digest cannot replace the observed child
digest. Mirror/index descriptor continuity and actual pipeline source/run
identity remain the caller's responsibility. The linked source SHA and command
arguments alone are not independent build provenance.

Corpus provenance remains pinned to
`9d3089f42f7e06c30041eb0388f10a9c071cbd3d`, independently of the executable commit.
The result's `runnerSha256` hashes the sorted non-test `.go` basenames in the
executor directory: UTF-8 basename, NUL, exact file bytes, NUL, concatenated for
each file. This is a source-content digest, not a binary or archive digest.

The executor uses the immutable Windows2022 agnhost 2.39 child
`registry.k8s.io/e2e-test-images/agnhost@sha256:66482b9d14a4e4d412c18533e738255dddabe6d05da19b2df0476f17bf2324ef`.
Initial role readiness allows ten minutes for cold pulls. Each traffic phase
requires three consecutive matching rounds, five seconds apart, within ninety
seconds. Only the verified TCP client's exact `TIMEOUT` or `REFUSED` result
counts as denial; exec/authentication/transport failures fail validation.

Execution is bounded to thirty-five minutes, reserving the final ninety seconds
for UID-bound namespace cleanup. SIGINT/SIGTERM terminate in-flight kubectl
process groups, including credential-helper descendants, and allow independent
cleanup within that original deadline. Failure,
cancellation, changed runtime identity, or incomplete cleanup never produces a
success result. The output path must not already exist. Successful JSON records
`suite`, `schemaVersion`, `status`, `runId`, `sourceVersion`, `runnerSha256`,
`image`, `imageDigest`, `imageConfigDigest`, `platform`, `osVersion`, `mode`,
`liteModeVerified`, `observedConfig`, `corpus`, all ten `caseResults`, and
`cleanupSucceeded`, plus per-pod, process, policy, probe, and rejection evidence.
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
