# azure-ip-masq-merger

`azure-ip-masq-merger` is a utility for merging multiple ip-masq-agent configuration files into a single, valid configuration for use in Kubernetes clusters.

## Description

The goal of this program is to periodically scan a directory for configuration fragments (YAML or JSON files starting with `ip-masq`), validate and merge them, and write the resulting configuration to a target directory for consumption. This allows us to combine non-masquerade CIDRs and related options between multiple files, for example if we had one ip masq config managed by the cloud provider and another supplied by the user.

## Usage

Follow the steps below to build and run the program:

1. Build the binary using `make`:
    ```bash
    make azure-ip-masq-merger
    ```
    or make an image:
    ```bash
    make azure-ip-masq-merger-image
    ```

2. Deploy or copy the binary to your node(s).

3. Prepare your configuration fragments in the input directory (see below for defaults). Each file should be named with the prefix `ip-masq` and contain valid YAML or JSON for the ip-masq-agent config.

4. Start the program with:
    ```bash
    ./azure-ip-masq-merger --input=/etc/config/ --output=/etc/merged-config/
    ```
    - The `--input` flag specifies the directory to scan for config fragments. Default: `/etc/config/`
    - The `--output` flag specifies where to write the merged config. Default: `/etc/merged-config/`

5. The merged configuration will be written to the output directory as `ip-masq-agent`. If no valid configs are found, any existing merged config will be removed.

## Manual Testing

You can test the merger locally by creating sample config files in your input directory and running the merger.

## Configuration File Format

Each config fragment should be a YAML or JSON file that may have the following fields:
```yaml
nonMasqueradeCIDRs:
  - 10.0.0.0/8
  - 192.168.0.0/16
masqLinkLocal: true
masqLinkLocalIPv6: false
```
- `nonMasqueradeCIDRs`: List of CIDRs that should not be masqueraded. Appended between configs.
- `masqLinkLocal`: Boolean to enable/disable masquerading of link-local addresses. OR'd between configs.
- `masqLinkLocalIPv6`: Boolean to enable/disable masquerading of IPv6 link-local addresses. OR'd between configs.

## Debugging

Logs are output to standard error. Increase verbosity with the `-v` flag:
```bash
./azure-ip-masq-merger -v 2
```

## Development

To run tests:
```bash
go test ./...
```
or at the repository level:
```bash
make test-azure-ip-masq-merger
```
