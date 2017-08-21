# Changelog

## v0.9 (2017-08-17)
- Limit port aggregation to owned interfaces for coexistence with other plugins.
- Made CNI DEL commands idempotent for soft failures, required for Kubernetes.
- Changed CNI default networking mode to bridge.
- Added ARP reply rules for bridge and tunnel modes to decouple from VNET behavior.
- Fixed bug in libnetwork option parser.
- Improved reliability of network failure paths.
- Added logging for soft failures in IPAM plugin.

## v0.8 (2017-06-29)
- Added Windows libnetwork network and IPAM plugins.
- Added support for CNI spec versions 0.3.0 and 0.3.1.
- Updated CNI plugin to recover from panics and convert them to CNI errors.
- Added log file rotation.
- Added libnetwork pluginv2 support.
- Updated Linux build and pluginv2 base images to golang1.8.1 and ubuntu16.04.
- Renamed libnetwork plugin to azure-vnet-plugin.
- Updated IPAM to return VNET default gateway and DNS server addresses.
- Added support for custom API server URLs to libnetwork plugins.
- Added CNI plugin install scripts for Linux and Windows.

## v0.7 (2017-03-31)
- Unified CNI network config file for Linux and Windows containers.
- Added hairpinning for Linux l2tunnel mode.
- Added master interface discovery.

## v0.6 (2017-03-13)
- Refactored all code to compile on both Windows and Linux.
- Added Windows CNI network and IPAM plugins.
- Added Linux l2tunnel mode for CNI and libnetwork.
- Added ability to request address pools on a specific interface.
- Added support for multiple subnets and gateways per network.
- Updated official plugin names and file locations.
- Separated CNI IPAM plugin to its own binary.
- Improved overall logging.
- Added containerized builds.
- Added basic documentation.
- Added license.

## v0.5 (2017-01-06)
- Added CNI support.
- Added Docker pluginv2 support.
- Various bug fixes.

## v0.4 (2016-09-30)
- Azure and Azure Stack environment support.
- Added automatic address configuration from WireServer based on environment.
- Refactored core net and IPAM logic for CNI support.
- Various bug fixes and polish.

## v0.3 (2016-07-29)
- Initial release of network and IPAM plugin.
- Added libnetwork plugin support.
- Added JSON store support.
