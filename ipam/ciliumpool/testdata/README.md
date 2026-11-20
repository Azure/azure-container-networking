# CiliumNode API contract fixture

`ciliumnodes-v1.20.1.yaml` is the unmodified Cilium v1.20.1 CRD from
[commit 7d68cfb394f2960e10aa72e76d0d51e66c1b2ebc](https://github.com/cilium/cilium/blob/7d68cfb394f2960e10aa72e76d0d51e66c1b2ebc/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumnodes.yaml).
Its Git blob SHA is `5ffb64fc5df4e83f5727f89e2b04b82741202d4f`.

Run the local API-server test with:

```sh
ACN_CILIUM_ENVTEST=1 go test ./ipam/ciliumpool -run '^TestCiliumNodeAPIContract$' -count=1
```

The test starts an isolated etcd/API server on loopback with ephemeral ports,
installs the exact CRD, and exercises allocation, deletion, interleaved status
writers and stale UID/resourceVersion rejection. It does not use a kubeconfig
or an existing cluster. Kubernetes 1.33.0 test binaries are downloaded into a
temporary directory unless `KUBEBUILDER_ASSETS` supplies installed binaries.
All test processes and temporary data are cleaned up.
