# CNS IPAM Unix socket

On Linux, `EnableIPAMUnixSocket` enables an additional CNS listener.
The setting defaults to `false`.
The socket path is `/var/run/azure-cns/cns.sock`.
TCP continues to serve all existing routes.

Before enabling the listener, provision `/var/run/azure-cns` as a root-owned directory with mode `0700`.
CNS rejects a missing directory, a directory symlink, a different owner, or different permissions.
CNS does not create the directory or change its permissions.
CNS rejects existing files, symlinks, directories, and active sockets at the socket path.
CNS replaces a stale socket only when the connection attempt returns `ECONNREFUSED`.

The socket serves only `RequestIPConfig`, `RequestIPConfigs`, `ReleaseIPConfig`, and `ReleaseIPConfigs`.
CNS sets the socket mode to `0600` before accepting connections.
At shutdown, CNS closes the listener and removes the socket.
Other platforms reject the enabled setting.

The `cns_ipam_requests_total{route,transport}` counter distinguishes TCP requests from Unix requests.
See [CNS metrics](examples/metrics/README.md) for the query.
