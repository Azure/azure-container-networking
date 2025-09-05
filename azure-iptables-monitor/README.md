# azure-iptables-monitor

`azure-iptables-monitor` is a utility for monitoring iptables rules on Kubernetes nodes and labeling a ciliumnode resource based on whether the corresponding node contains user-defined iptables rules.

## Description

The goal of this program is to periodically scan iptables rules across all tables (nat, mangle, filter, raw, security) and determine if any rules exist that don't match expected patterns. When unexpected rules are found, the ciliumnode resource is labeled to indicate the presence of user-defined iptables rules.

## Usage

Follow the steps below to build and run the program:

1. Build the binary using `make`:
    ```bash
    make azure-iptables-monitor
    ```
    or make an image:
    ```bash
    make azure-iptables-monitor-image
    ```

2. Deploy or copy the binary to your node(s).

3. Prepare your allowed pattern files in the input directory. Each file should be named after an iptables table (`nat`, `mangle`, `filter`, `raw`, `security`) or `global` and contain regex patterns that match expected iptables rules. You may want to mount a configmap for this purpose.

4. Start the program with:
    ```bash
    ./azure-iptables-monitor -input=/etc/config/ -interval=300
    ```
    - The `-input` flag specifies the directory containing allowed regex pattern files. Default: `/etc/config/`
    - The `-input6` flag specifies the directory containing allowed regex pattern files for IPv6 ip6tables. Default: `/etc/config6/`
    - The `-interval` flag specifies how often to check iptables rules and the bpf map in seconds. Default: `300`
    - The `-events` flag enables Kubernetes event creation for rule violations. Default: `false`
    - The `-ipv6` flag enables IPv6 ip6tables monitoring using the IPv6 allowlists. Default: `false`
    - The `-checkMap` flag enables checking the pinned bpf map specified in mapPath for increases. Default: `false`
    - The `-mapPath` flag specifies the pinned bpf map path to check. Default: `/azure-block-iptables-bpf-map/iptables_block_event_counter`
    - The `-terminateOnSuccess` flag, when set, will exit the program once there are no longer user iptables rules detected. Default: `false`
    - The program must be in a k8s environment and `NODE_NAME` must be a set environment variable with the current node.

5. The program will set the `kubernetes.azure.com/user-iptables-rules` label to `true` on the specified ciliumnode resource if unexpected rules are found, or `false` if all rules match expected patterns. Proper RBAC is required for patching (patch for ciliumnodes, create for events, get for nodes).

6. The program will also send out an event if the bpf map value specified increases between checks


## Pattern File Format

Each pattern file should contain one regex pattern per line:
```
^-A INPUT -i lo -j ACCEPT$
^-A FORWARD -j DOCKER.*
^-A POSTROUTING -s 10\.0\.0\.0/8 -j MASQUERADE$
```

- `global`: Patterns that can match rules in any iptables table
- `nat`, `mangle`, `filter`, `raw`, `security`: Patterns specific to each iptables table
- Empty lines are ignored
- Each line should be a valid Go regex pattern
- The ipv6 config directory uses files with same names, but will match against ipv6 iptables rules

## Debugging

Logs are output to standard error. Increase verbosity with the `-v` flag:
```bash
./azure-iptables-monitor -v 3
```

## Development

To run tests at the repository level:
```bash
make test-azure-iptables-monitor
```
