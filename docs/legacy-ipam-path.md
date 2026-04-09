# Legacy IPAM Path: End-to-End Flow

## Overview

The legacy Azure IPAM path (`azure-vnet-ipam`) is the older, node-based IP allocation system that predates the modern CNS-based approach. It works by querying the Azure wire server for secondary IPs assigned to the VM's NICs and managing them locally via a persistent JSON state file.

**Key characteristics:**
- Node-scoped: all pod IPs come from the VM's pre-allocated secondary IPs
- Uses a separate binary (`azure-vnet-ipam`) invoked via CNI delegation
- State persisted to `/var/run/azure-vnet-ipam.json` (Linux) or `C:\k\azure-vnet-ipam.json` (Windows)
- Queries wire server at `168.63.129.16` for available IPs

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│  Azure ARM                                               │
│  (allocates secondary IPs to VM NIC from VNet subnet)    │
└──────────────────┬───────────────────────────────────────┘
                   │
┌──────────────────▼───────────────────────────────────────┐
│  Wire Server / NMAgent  (168.63.129.16)                  │
│  Exposes NIC metadata via XML HTTP API                   │
│  GET /machine/plugins?comp=nmagent&type=getinterfaceinfov1│
└──────────────────┬───────────────────────────────────────┘
                   │
┌──────────────────▼───────────────────────────────────────┐
│  azure-vnet-ipam binary  (/opt/cni/bin/azure-vnet-ipam)  │
│  Parses XML, builds address pools, allocates IPs         │
│  Persists state to /var/run/azure-vnet-ipam.json         │
└──────────────────┬───────────────────────────────────────┘
                   │  (CNI delegation: stdin JSON → stdout JSON)
┌──────────────────▼───────────────────────────────────────┐
│  AzureIPAMInvoker  (cni/network/invoker_azure.go)        │
│  Calls DelegateAdd/DelegateDel on the IPAM binary        │
└──────────────────┬───────────────────────────────────────┘
                   │
┌──────────────────▼───────────────────────────────────────┐
│  NetPlugin  (cni/network/network.go)                     │
│  Receives IP, creates veth pair, assigns IP to pod       │
└──────────────────────────────────────────────────────────┘
```

## Key Files

| File | Purpose |
|------|---------|
| `cni/network/invoker_azure.go` | `AzureIPAMInvoker` — orchestrates DelegateAdd/DelegateDel to the IPAM binary |
| `cni/plugin.go` | `DelegateAdd()`/`DelegateDel()` — spawns the IPAM subprocess |
| `cni/ipam/plugin/main.go` | Entry point for the `azure-vnet-ipam` binary |
| `cni/ipam/ipam.go` | IPAM plugin Add/Delete/Configure handlers |
| `ipam/manager.go` | `AddressManager` — state management, pool/address request APIs |
| `ipam/pool.go` | `addressPool`/`addressSpace` — allocation logic |
| `ipam/azure.go` | `azureSource` — wire server HTTP query and XML parsing |
| `cni/netconfig.go` | `NetworkConfig` — CNI config structure including IPAM section |

## ADD Flow (Pod Creation)

### Step 1: Kubelet invokes CNI

Kubelet calls the `azure-vnet` CNI binary with `CNI_COMMAND=ADD`, passing the network config on stdin and pod metadata in environment variables (`CNI_CONTAINERID`, `CNI_ARGS`).

### Step 2: NetPlugin selects the IPAM invoker

In `cni/network/network.go`, `NetPlugin.Add()` checks the config and creates an `AzureIPAMInvoker` (as opposed to the modern `CNSIPAMInvoker`).

### Step 3: AzureIPAMInvoker.Add() delegates to the IPAM binary

`cni/network/invoker_azure.go`:

1. Populates `nwCfg.IPAM.Subnet` from the network's subnet info.
2. Calls `plugin.DelegateAdd("azure-vnet-ipam", nwCfg)`.

This triggers `cni/plugin.go:DelegateAdd()`, which:
- Sets `CNI_COMMAND=ADD` in the environment.
- Serializes the `NetworkConfig` to JSON.
- Spawns `/opt/cni/bin/azure-vnet-ipam` as a subprocess, passing config on stdin.
- Reads the CNI result JSON from stdout.

### Step 4: azure-vnet-ipam binary starts

`cni/ipam/plugin/main.go`:

1. Creates an `ipamPlugin` wrapping an `AddressManager`.
2. Initializes the key-value store (persistent JSON file).
3. Restores prior state from disk.
4. Calls `ipamPlugin.Add()`.

### Step 5: Configure — select the IP source

`cni/ipam/ipam.go:Configure()`:

1. Parses the `NetworkConfig` from stdin.
2. Checks `IPAM.Environment` to pick the address source:
   - `"azure"` → `newAzureSource()` (wire server)
   - `"mas"` / `"file"` → `newFileIpamSource()`
   - `"ipv6-node-ipam"` → `newIPv6IpamSource()`
   - `"null"` → `newNullSource()` (testing)
3. Calls `am.StartSource()` to initialize the source.

### Step 6: RequestPool — query wire server for available IPs

`ipam/manager.go:RequestPool()`:

1. Calls `am.refreshSource()`, which calls `azureSource.refresh()`:

   **Wire server query** (`ipam/azure.go`):
   ```
   HTTP GET http://168.63.129.16/machine/plugins?comp=nmagent&type=getinterfaceinfov1
   ```

   Returns XML like:
   ```xml
   <Interfaces>
     <Interface MacAddress="22334455aabb" IsPrimary="true">
       <IPSubnet Prefix="10.0.0.0/24">
         <IPAddress Address="10.0.0.1" IsPrimary="true"/>   <!-- VM's IP, skipped -->
         <IPAddress Address="10.0.0.2" IsPrimary="false"/>  <!-- Available for pods -->
         <IPAddress Address="10.0.0.3" IsPrimary="false"/>  <!-- Available for pods -->
       </IPSubnet>
     </Interface>
   </Interfaces>
   ```

2. The source parses the XML, matches MACs to local interfaces, and creates an `addressPool` per subnet containing `addressRecord` entries for each **secondary** (non-primary) IP.

3. Calls `sink.setAddressSpace()` to merge the new pool data into the `AddressManager`, updating epochs to detect stale addresses.

4. `requestPool()` selects an available pool (one with `RefCount == 0`, correct address family, matching interface). Sets `RefCount = 1`.

5. Persists state to disk.

### Step 7: RequestAddress — allocate a specific IP

`ipam/manager.go:RequestAddress()` → `ipam/pool.go:requestAddress()`:

1. Finds the first `addressRecord` where `InUse == false` and `ID == ""`.
2. Sets `ar.InUse = true` and `ar.ID = containerID`.
3. Persists state to disk.
4. Returns the IP in CIDR notation (e.g., `10.0.0.2/24`).

### Step 8: Build and return the CNI result

`cni/ipam/ipam.go` queries pool info for gateway and DNS, then builds a standard CNI result:

```json
{
  "ips": [{ "address": "10.0.0.2/24", "gateway": "10.0.0.1" }],
  "routes": [{ "dst": "0.0.0.0/0", "gw": "10.0.0.1" }],
  "dns": { "nameservers": ["168.63.129.16"] }
}
```

This is serialized to stdout and read by `DelegateAdd()`.

### Step 9: IPv6 (optional)

If `IPV6Mode` is set, `AzureIPAMInvoker` repeats the delegation with:
- `IPAM.Type = "azure-vnet-ipamv6"`
- `IPAM.Environment = "ipv6-node-ipam"`
- `IPAM.Subnet` = the IPv6 subnet from `Subnets[1]`

IPv6 results are appended to the IPv4 result.

### Step 10: NetPlugin configures the pod network

`AzureIPAMInvoker.Add()` returns an `IPAMAddResult` containing `InterfaceInfo` (IPs, routes, DNS, NICType=InfraNIC). The `NetPlugin` then creates the veth pair, assigns the IP via netlink, and programs routes.

## DEL Flow (Pod Deletion)

### Step 1: Kubelet invokes CNI with DEL

### Step 2: AzureIPAMInvoker.Delete()

`cni/network/invoker_azure.go`:

1. Sets `nwCfg.IPAM.Address` to the IP being released.
2. Calls `plugin.DelegateDel("azure-vnet-ipam", nwCfg)`.
3. For IPv6 addresses, delegates to `azure-vnet-ipamv6` separately.

### Step 3: azure-vnet-ipam releases the IP

`cni/ipam/ipam.go:Delete()` → `ipam/manager.go:ReleaseAddress()` → `ipam/pool.go:releaseAddress()`:

1. Finds the `addressRecord` by IP address or container ID.
2. Sets `ar.InUse = false`, clears `ar.ID`.
3. If the source previously marked this address as stale (epoch mismatch), the record is deleted entirely.
4. Persists state to disk.

### Step 4: NetPlugin tears down the pod network

Deletes the veth pair, cleans up routes and iptables rules.

## State Management

### Persistent State File

Located at `/var/run/azure-vnet-ipam.json` (Linux) or `C:\k\azure-vnet-ipam.json` (Windows).

```json
{
  "Version": "v1.0.0",
  "TimeStamp": "2024-01-15T10:30:45Z",
  "AddressSpaces": {
    "local": {
      "Id": "local",
      "Scope": 0,
      "Pools": {
        "10.0.0.0/24": {
          "Id": "10.0.0.0/24",
          "IfName": "eth0",
          "Subnet": { "IP": "10.0.0.0", "Mask": "ffffffff00" },
          "Gateway": "10.0.0.1",
          "Addresses": {
            "10.0.0.2": { "ID": "abc123", "Addr": "10.0.0.2", "InUse": true },
            "10.0.0.3": { "ID": "", "Addr": "10.0.0.3", "InUse": false }
          },
          "RefCount": 1
        }
      }
    }
  }
}
```

State is saved after every mutation (pool request, address allocation, address release). This ensures durability across process crashes.

### Epoch-Based Pool Merging

Each `refreshSource()` call from the wire server increments an epoch counter. Address records carry their epoch, enabling:
- New addresses from wire server are added with the current epoch.
- Existing addresses that wire server still reports get their epoch updated.
- Stale addresses (epoch < current) that are not in-use are deleted.
- Stale addresses that are in-use are marked `unhealthy` and deleted upon release.

### Reboot Recovery

On startup, `AddressManager.restore()` compares the state file's modification time against the VM's last reboot time. If the VM rebooted after the state was last saved, all addresses are reset to `InUse = false` — since containers could not have survived the reboot.

## Error Handling

### ErrNoAvailableAddressPools

If all pools are exhausted or the state is corrupted:

1. `AzureIPAMInvoker` deletes the state file (`/var/run/azure-vnet-ipam.json`).
2. Retries `DelegateAdd()`, forcing a fresh wire server query and pool rebuild.

### Cleanup on Partial Failure

If `Add()` allocates an IP but a subsequent step fails (e.g., veth creation), a deferred function calls `Delete()` to release the IP back to the pool.

### Idempotent Deletes

`releaseAddress()` does not fail if the address is already released or not found — this makes DEL safe to retry.

## Legacy vs. Modern (CNS) Path

| Aspect | Legacy IPAM | Modern CNS |
|--------|------------|-----------|
| IP source | VM secondary IPs via wire server | Managed pools via NodeNetworkConfig CRD |
| Query mechanism | HTTP to 168.63.129.16 (XML) | gRPC/HTTP to local CNS daemon (:10090) |
| State storage | JSON file on disk | CNS in-memory state |
| Scaling | Limited to Azure NIC secondary IP count | Dynamic pool management, overlay support |
| IPv6 | Separate `azure-vnet-ipamv6` binary | Integrated in CNS |
| Config trigger | `IPAM.Type = "azure-vnet-ipam"` | `IPAM.Type = "azure-cns"` |
| Multi-tenancy | Not supported | Supported (Swift v1/v2) |
| Overlay | Not supported | Supported |

## Configuration

The legacy path is selected when the CNI network config specifies:

```json
{
  "ipam": {
    "type": "azure-vnet-ipam",
    "environment": "azure",
    "subnet": "10.0.0.0/24"
  }
}
```

Key IPAM config fields:
- **type**: `"azure-vnet-ipam"` (IPv4) or `"azure-vnet-ipamv6"` (IPv6)
- **environment**: `"azure"` (wire server), `"mas"`/`"file"` (file-based), `"null"` (test)
- **subnet**: Optional. If omitted, the plugin discovers subnets from wire server.
- **address**: Optional. If set, requests that specific IP.
- **queryInterval**: Wire server polling interval in seconds (default: 10).
