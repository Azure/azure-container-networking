# Image test bundles

Choose `testBundle` when queueing; named values replace `images`.
`setup` decides once and emits flags that the existing stages read.
Image overrides choose images, not coverage; omit them for normal builds.

| Bundle | Image category | Stage pairs below |
| --- | --- | --- |
| `full` (default) | All components | Original full path, including pairs 1-18 |
| `cni-cns` | `azure-cni`, `azure-cns` | 1-18 |
| `ipam` | `azure-ipam` | 1-14, 17-18 |
| `cilium` | `cilium-log-collector` | 1-9 |
| `cilium-ebpf` | `azure-iptables-monitor` | 3, 7, 8 |
| `cilium-masq` | `azure-ip-masq-merger` | 7, 8 |
| `ipv6-bpf` | `ipv6-hp-bpf` | 6, 8 |
| `npm` | `azure-npm` | Separate pipeline `195725`, not an ACN `testBundle` value |

## Selected stage pairs

Each selected row runs **Create -> E2E** after its existing prerequisites succeed.
Unselected pairs skip. `full` retains original source/tag filters; named bundles
bypass those filters. No jobs, assertions, or failure/exception policies change.

| # | Create stage | E2E stage |
| --- | --- | --- |
| 1 | `ciliume2e` | `cilium_e2e` |
| 2 | `ciliumvscalee2e` | `cilium_vnetscale_e2e` |
| 3 | `cilbpfpode2e` | `cilium_ebpf_podsubnet_e2e` |
| 4 | `cilndsubnete2e` | `cilium_nodesubnet_e2e` |
| 5 | `cilovere2e` | `cilium_overlay_e2e` |
| 6 | `cildsovere2e` | `cilium_dualstackoverlay_e2e` |
| 7 | `cilbpfovere2e` | `cilium_ebpf_overlay_e2e` |
| 8 | `cilbpfdse2e` | `cilium_ebpf_ds_e2e` |
| 9 | `cilwhleovere2e` | `cilium_h_overlay_e2e` |
| 10 | `linuxazovere2e` | `linux_azure_overlay_e2e` |
| 11 | `winazovere2e` | `win_azure_overlay_e2e` |
| 12 | `statelesswin` | `azure_overlay_stateless_e2e` |
| 13 | `swifte2e` | `aks_swift_e2e` |
| 14 | `vscaleswifte2e` | `aks_swift_vnetscale_e2e` |
| 15 | `ubuntu22e2e` | `aks_ubuntu_22_linux_e2e` |
| 16 | `win22e2e` | `aks_windows_22_e2e` |
| 17 | `linuxdsovere2e` | `linux_dualstackoverlay_e2e` |
| 18 | `windsovere2e` | `win_dualstackoverlay_e2e` |

Unchanged base stages: `setup`, `test`, `test_windows`, `code_coverage`, `binaries`,
`containerize`, `containerize_npm`, `publish`, `publish_npm`, `sbom`, `delete_clusters`.
Failure analysis, governed checks, and nightly stages keep their existing conditions.
Builds, artifacts, cleanup, and image-reference validation are unchanged.

Tests: `python3 -m unittest discover -s hack/scripts -p 'test_image_test_bundle.py'`.
