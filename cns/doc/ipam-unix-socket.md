# CNS IPAM Unix socket

On Linux, `EnableIPAMUnixSocket` moves the CNS IPAM routes from TCP to a Unix socket listener.
The setting defaults to `false`.
The socket path is `/var/run/azure-cns/cns.sock`.
TCP continues to serve all non-IPAM routes.
If `EnableIPAMUnixSocket` is true, CNI configurations must set `cnsSocketPath` to `/var/run/azure-cns/cns.sock`.

Before enabling the listener, provision `/var/run/azure-cns` as a root-owned directory without group or other write permission.
CNS accepts mode `0755`, which Kubernetes uses for `DirectoryOrCreate` host volumes.
CNS rejects a missing directory, a directory symlink, a different owner, or group or other write permission.
CNS does not create the directory or change its permissions.
`/var/run` is a tmpfs, so the directory must be provisioned again after every node boot, before CNS starts.
The `dropgz secure-dir /var/run/azure-cns` command creates the directory if needed and sets it to root-owned with mode `0700`.
It refuses symlinks and non-directories, and it is idempotent, so it can run as an init container on every pod start.
CNS rejects existing files, symlinks, directories, and active sockets at the socket path.
CNS replaces a stale socket only when the connection attempt returns `ECONNREFUSED`.

The socket serves only `RequestIPConfig`, `RequestIPConfigs`, `ReleaseIPConfig`, and `ReleaseIPConfigs`.
CNS sets the socket mode to `0600` before accepting connections.
At shutdown, CNS closes the listener and deletes the socket.
Other platforms reject the enabled setting.

## Pod activation

The `CNS_IPAM_UNIX_SOCKET_ENABLED` environment variable overrides `EnableIPAMUnixSocket` for one CNS pod.
If the variable is absent, CNS uses the file setting.
CNS rejects an invalid or empty value.

Enable UDS through this variable in the pod specification, not through a shared ConfigMap.
Kubernetes updates a mounted ConfigMap in running pods.
If a CNS container in an old pod restarts, it reads the new file.
That pod can still have a CNI binary on the host that does not read `cnsSocketPath`.
The CNI then sends IPAM requests to TCP, and CNS rejects them.
A pod-specific variable activates UDS only in pods whose init container installed a compatible CNI binary.

The CNI binary must read `cnsSocketPath` before CNS enables UDS.
`azure-vnet` supports it from `v1.9.5`.
`azure-ipam` supports it from `v0.4.6` and `v0.5.2`.
`azure-ipam` versions `v0.5.0` and `v0.5.1` do not support it.
CNS must also generate the CNI conflist, because the conflist carries `cnsSocketPath` to the CNI.
CNS releases older than this variable ignore it and keep IPAM on TCP.
Do not set this variable on Windows. Windows CNS rejects the enabled setting and exits.

## Transitions

When a node switches from TCP to UDS, the old conflist stays on disk until CNS writes the new conflist.
During that interval, CNI requests use TCP and fail with HTTP 404.
The kubelet retries failed ADD and DEL operations.
When a node switches from UDS to TCP, the CNI can find no socket until CNS writes the new conflist.
In that case, the CNI defers DEL operations and CNS completes them over TCP.

## E2E coverage

The Azure CNI overlay and Cilium overlay pipelines create pods while CNS uses TCP.
They then enable UDS through the pod variable and check the following on each Linux node:

- The socket exists with mode `0600`.
- The conflist contains the socket path.
- TCP returns HTTP 404 for IPAM requests.
- The socket accepts IPAM requests.

The pipelines delete the pods that the CNI created over TCP, create new pods, and check the IP state.
After the node restart, the pipelines repeat the socket checks.
The restart recreates `/var/run/azure-cns` with mode `0755`, so it also tests a cold start.
The pipelines run these steps only when the CNS and CNI installer images come from the source build.
If a run overrides either image with a release, the pipeline tests TCP only, because the release can lack UDS support.

The `cns_ipam_requests_total{route,transport}` counter distinguishes TCP requests from Unix requests.
See [CNS metrics](examples/metrics/README.md) for the query.
