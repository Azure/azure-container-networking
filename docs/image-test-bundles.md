# Image test bundles

`testBundle: full` is the default and preserves the existing pipeline behavior.
`testBundle: images` selects existing E2E/create-cluster stage pairs using the
image override parameters.

The decision happens once in the existing `setup` stage:

1. `ImageBundle` runs `hack/scripts/image_test_bundle.py`.
2. It validates the supplied references and selects the union of their consumers.
3. It publishes `run_<suite>` Boolean outputs.
4. Each existing create/E2E stage checks its output and normal dependencies.

For example, the stage condition is:

```yaml
condition: and(succeeded(), eq(dependencies.setup.outputs['env.ImageBundle.run_${{ parameters.name }}'], 'true'))
```

| Override | Selected existing E2E stages |
| --- | --- |
| `cniImage` | All 18 CNI/Cilium suites |
| `cnsImage`, `ipamImage` | The 16 CNIv2/Cilium suites; not CNIv1 |
| `azureIptablesMonitorImage` | Cilium EBPF podsubnet, overlay, dualstack |
| `azureIpMasqMergerImage` | Cilium EBPF overlay, dualstack |
| `ciliumLogCollectorImage` | All 9 Cilium suites |
| `ipv6HpBpfImage` | Cilium dualstack overlay and EBPF dualstack |

Image mode requires at least one fully qualified tagged image. An optional
`@sha256:<64 hex characters>` suffix is accepted. Core CNI/CNS/IPAM images must
use `acnpublic.azurecr.io` or `mcr.microsoft.com/containernetworking`, as required
by the existing installer. Invalid input fails setup before provisioning.

Only the suite-selection conditions change in image mode. Selected suites are
not suppressed by source-diff/current-source-tag filters. Unselected pairs skip.
Full mode retains the original conditions, including those source filters.

Builds, publishing, image override handling, artifacts, unit tests, governed
checks, cleanup, and all E2E job bodies remain unchanged. This does not optimize
build selection or change existing test failure/exception policy.

Run the focused resolver tests with:

```sh
python3 -m unittest discover -s hack/scripts -p 'test_image_test_bundle.py'
```
