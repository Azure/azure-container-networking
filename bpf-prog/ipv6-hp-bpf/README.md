# ipv6-hp-bpf

`ipv6-hp-bpf` is a project that leverages eBPF (Extended Berkeley Packet Filter) technology for traffic control in Linux kernel. This is a POC to fix external load balancer services in cilium dualstack clusters.

## Description

The goal of this bpf program is to fix the issue described [here](https://github.com/cilium/cilium/issues/31326). It includes both egress and ingress TC programs. These programs are meant to replace the nftable rules since they don't work on cilium clusters. 
The egress bpf code converts the destination IPv6 of the packet from global unicast to link local, and ingress converts the source IPv6 from link local to global unicast.

## Usage

Follow the steps below to compile the program and install it onto your node:

1. Use the make command to build the binary or follow the steps below.
    ```bash
    make ipv6-hp-bpf-binary
    ```

2. Copy the new binary to your node(s).

3. Remove the nftable rules for ipv6 with the following commands:
    ```bash
    nft delete chain ip6 azureSLBProbe postrouting 
    nft delete chain ip6 azureSLBProbe prerouting 
    nft -n list table ip6 azureSLBProbe 
    ```

4. Start the program with:
    ```bash
    ./ipv6-hp-bpf
    ```
5. Debugging logs can be seen in the node under `/sys/kernel/debug/traceing/trace_pipe`

## Manual Compilation
For testing purposes you can compile the bpf program without go, and attach it to the interface yourself. This is how you would do it for egress:
```bash
clang -O2 -g -target bpf -c egress.c -o egress.o
```

This will generate the egress.o file, which you can copy over to your cluster's node.
To copy to the node you need to create a node-shell instance
```bash
kubectl cp egress.o nsenter-xxxxx:<path-in-node>
```

Since this is for cilium clusters, cilium already creates a qdisc on eth0 of type clsact (which allows both ingress and egress filters to be attached). If cilium is not installed, you would have to create the qdisc on your own by doing the following: 
```bash
tc qdisc add dev eth0 clsact
```

## Attach the filter
```bash
tc filter add dev eth0 egress prio 1 bpf da obj egress.o sec classifier
```

## Verify the filter is attached
```bash
tc filter show dev eth0 egress
```