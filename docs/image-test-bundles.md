# Image-driven stage bundles

The PR pipeline accepts `testBundle: full` (the default) or `testBundle: images`.
This selects complete existing stages, not individual tests. E2E jobs, test
steps, assertions, logging, and failure handlers are unchanged.

`full` retains the ordinary build, release-artifact, and source-driven test
path. When external images are supplied, Windows/Cilium source-diff and
Swift/Cilium current-source-tag gates no longer suppress their validation.
Ordinary callers without image overrides retain those gates.

For `images`, supply at least one of the existing image override parameters.
The existing `setup` stage resolves the union of their consumer stages and the
companion images those stages need. It emits runtime outputs; the complete
stage graph is still visible, with unrelated stages skipped.

| Override | Selected existing E2E stages |
| --- | --- |
| `cniImage` | All 18 CNI/Cilium suites |
| `cnsImage`, `ipamImage` | The 16 CNIv2/Cilium suites; not CNIv1 |
| `azureIptablesMonitorImage` | Cilium EBPF podsubnet, overlay, dualstack |
| `azureIpMasqMergerImage` | Cilium EBPF overlay, dualstack |
| `ciliumLogCollectorImage` | All 9 Cilium suites |
| `ipv6HpBpfImage` | Cilium dualstack overlay and EBPF dualstack |

Each suite includes its existing cluster creation/install stage and E2E stage.
The resolver and exact stage names live in `hack/scripts/image_test_bundle.py`.
Overlapping selections are deduplicated. CNI/CNS pairing is simply the union of
the two existing image overrides. Scheduled-only suites remain scheduled-only.

Only missing companion images used by the selected suites are built/published.
The existing build/publish matrix legs are gated as complete jobs; their build
steps are unchanged. NPM images and binary release packages are not required by
these bundles and are skipped. Source unit tests, Windows unit tests, coverage,
and governed checks remain unchanged, including Linux CodeQL build tracing.
Cleanup still runs after failed or skipped suites. A failed prerequisite cannot
be treated as an intentionally omitted build.

Images mode keeps companion archives and signed SBOM metadata under the
`image-validation-companions` pipeline artifact, not the full release-ready
`output` artifact. With no missing companions, build/publish/SBOM are skipped.
`image-bundle-results` records the supplied image references, selected suites,
companion builds, and actual stage results. Its new summary stage fails if a
required stage is missing, skipped, failed, canceled, or only partially
successful. Existing exception/approval policy is not changed.

## Queueing and rollback

For example, queue with these template parameters:

```json
{
  "testBundle": "images",
  "azureIpMasqMergerImage": "acnpublic.azurecr.io/dalec/azure-ip-masq-merger:v1.0.0"
}
```

Use fully qualified tagged images. Optional `@sha256:<64 hex characters>`
suffixes are accepted. Core CNI/CNS/IPAM references must use registries
understood by the existing installer: `acnpublic.azurecr.io` or
`mcr.microsoft.com/containernetworking`. Invalid image-mode inputs fail in setup before provisioning.
Full mode retains existing image parsing behavior.

Keep producer wiring dormant until consumer support is deployed and each
component's stage selection is verified with authorized real runs. Where
coverage has not been established, select `full` explicitly and record why.
Rollback is `testBundle: full`; an older producer can omit the parameter.
Do not use a partial validation run as a source-release build.

## Validation and onboarding

Run local regression checks:

```sh
python3 -m unittest discover -s hack/scripts -p 'test_image_test_bundle.py'
```

When adding an image or suite, update consumer membership, companion closure,
stage gates, and the focused regression tests together. Verify the unchanged
installation path actually consumes the supplied reference. Stage success alone
does not prove image identity or new functional coverage. Do not add or change
tests inside stages as part of bundling.

Before enabling a component, preview both modes against the governed template,
then inspect real stage outcomes, existing test results/install logs, deployed
image references, and cleanup. Include a failed prerequisite and the
all-images-supplied/no-companion case. Check release consumers and artifact
contracts. Measure executed stages, clusters, agent-minutes, and wall time;
previews and local checks do not establish runtime savings.
