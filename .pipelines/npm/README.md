# NPM conformance fork snapshots

These tests depend on changes in personal forks. Each build uses a fixed source
revision rather than the current tip of a branch:

| Pipeline | Fork branch | Pinned revision |
| --- | --- | --- |
| `npm-conformance-tests.yaml` (`v2-place-first`) | `huntergregory/network-policy-api`: `huntergregory/service-types` | `0d0e470332d4b88854cfacdd931c19c0b2dce89a` |
| `npm-conformance-tests-latest-release.yaml` (Linux) | `vakalapa/kubernetes`: `vakr/sleepinnpmtests` | `5b0d95b7b0e033fbbe63fc19b275326cf354ee18` |
| `npm-conformance-tests-latest-release.yaml` (Windows) | `huntergregory/kubernetes`: `sleep-before-probing` | `fc67a48b5e3ccb44a3b8143b65ede161d8a234bb` |

The policy-assistant build uses the nested Go module in
`policy-assistant/go.mod`. Its fork revision is recorded as a pseudo-version
and fetched through the Go module proxy; `policy-assistant/go.sum` authenticates
the downloaded modules. To select another branch revision, resolve its commit
while the fork exists, download its nested module through the proxy, and update
the `require` and `replace` versions together. Rebuild and commit the resulting
`go.mod` and `go.sum`.

The Kubernetes test builds still need full Git checkouts. Their pinned commits
prevent branch movement from changing the tests, but **do not** preserve the
source if either personal fork is deleted. Move those repositories or the
built test binaries to an organization-controlled location before deletion.
The public Go proxy caches module versions but is not a substitute for an
organization-owned source or artifact backup.
