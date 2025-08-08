//go:build ignore

// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>
#include <stdbool.h>

#define sk_family __sk_common.skc_family
#define EPERM 1
#define IPT_SO_SET_REPLACE 64
#define TASK_COMM_LEN 16
#define COMM_COUNT 3
#define IPPROTO_IP 0
#define IPPROTO_IP6 41
#define AF_NETLINK 16
#define NETLINK_NETFILTER 12
#define NETLINK_MSG_COUNT 4
#define NFNL_SUBSYS_NFTABLES 10
#define NFT_MSG_NEWRULE 6

#define CILIUM_AGENT "cilium-agent"
#define IP_MASQ "ip-masq"
#define AZURE_CNS "azure-cns"

char __license[] SEC("license") = "Dual MIT/GPL";
volatile const u64 host_netns_inode = 4026531840; // Initialized by userspace

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 2);
    __type(key, u32);
    __type(value, u64);
    __uint(pinning, LIBBPF_PIN_BY_NAME);
} iptables_block_event_counter SEC(".maps");

// This function checks if the parent process of the current task is allowed to install iptables rules.
// It checks the parent's command name against a predefined list of allowed prefixes.
bool is_allowed_parent ()
{
    struct task_struct *task = (struct task_struct *)bpf_get_current_task();
    struct task_struct *parent_task = NULL;

    // Allow cilium-agent, ip-masq-agent and azure-cns
    char parent_comm[TASK_COMM_LEN] = {};
    const char target_prefixes[COMM_COUNT][TASK_COMM_LEN] = {CILIUM_AGENT, IP_MASQ, AZURE_CNS};

    // Safely get parent task_struct
    parent_task = BPF_CORE_READ(task, real_parent);
    if (!parent_task)
        return 0;

    // Safely read parent->comm
    if (bpf_core_read_str(&parent_comm, sizeof(parent_comm), &parent_task->comm) < 0)
        return 0;

    // Check if parent_comm is an allowed command
    #pragma unroll
    for(int p = 0; p < COMM_COUNT; p++) {
        int match = 1;
        for (int i = 0; i < TASK_COMM_LEN && target_prefixes[p][i] != '\0'; i++) {
            if (parent_comm[i] != target_prefixes[p][i]) {
                match = 0;
                break;
            }
        }

        if(match) {
            return 1;
        }
    }

    return 0; // Block
}

// check if the current task is in the host network namespace
// This function compares the inode number of the current network namespace with the host's network namespace inode
// The host's network namespace inode is initialized by userspace when the BPF program is loaded.
bool is_host_ns() {
    struct task_struct *task = (struct task_struct *)bpf_get_current_task();
    struct nsproxy *nsproxy;
    struct net *net_ns;
    unsigned int netns_ino = 0;

    nsproxy = BPF_CORE_READ(task, nsproxy);
    if (!nsproxy)
        return 0;

    net_ns = BPF_CORE_READ(nsproxy, net_ns);
    if (!net_ns)
        return 0;

    netns_ino = BPF_CORE_READ(net_ns, ns.inum);

    if (netns_ino != host_netns_inode) {
        return 0;
    }

    return 1;
}

// Increment the event counters in the BPF map. Key is 0 for blocked rules and 1 for allowed rules.
// This counter will be read from userspace to track the number of blocked/allowed events.
void increment_event_counter(bool isAllow) {
    u32 key = isAllow ? 1 : 0;
    u64 *value;

    value = bpf_map_lookup_elem(&iptables_block_event_counter, &key);
    if (value) {
        __sync_fetch_and_add(value, 1);
    } else {
        u64 initial_value = 1;
        bpf_map_update_elem(&iptables_block_event_counter, &key, &initial_value, BPF_ANY);
    }
}

// blocking hook for iptables-legacy rule installation
SEC("lsm/socket_setsockopt")
int BPF_PROG(iptables_legacy_block, struct socket *sock, int level, int optname)
{
    if (sock == NULL) {
        return 0;
    }

    //block both ipv4 and ipv6 iptables rule installation
    if (level == IPPROTO_IP || level == IPPROTO_IP6) {
        //iptables-legacy uses IPT_SO_SET_REPLACE to install rules
        if (optname == IPT_SO_SET_REPLACE) {
            // block if not in host network namespace, and if the parent process is not allowed
            if (is_host_ns()) {
                if (!is_allowed_parent()) {
                    increment_event_counter(false);
                    return -EPERM;
                } else {
                    increment_event_counter(true);
                    return 0; // Allow the operation
                }
            }
        }
    }

    return 0;
}

// blocking hook for iptables-nftables rule installation
SEC("lsm/netlink_send")
int BPF_PROG(iptables_nftables_block, struct sock *sk, struct sk_buff *skb) {
    if (sk == NULL || skb == NULL) {
        return 0;
    }
    __u16 family = 0, proto = 0;
    bpf_probe_read_kernel(&family, sizeof(family), &sk->sk_family);

    // Check if the socket family is AF_NETLINK (just a sanity check)
    if (family != AF_NETLINK) 
        return 0;

    bpf_probe_read_kernel(&proto, sizeof(proto), &sk->sk_protocol);


    // Check if the protocol is NETLINK_NETFILTER
    // This is the protocol used for netfilter messages
    if (proto != NETLINK_NETFILTER) 
        return 0;

    if (!is_host_ns()) {
        return 0;
    }

    struct nlmsghdr nlh = {};
    void *data = NULL;
    __u32 skb_len = 0;

    // Read the skb data pointer
    if (bpf_core_read(&data, sizeof(data), &skb->data) < 0)
        return 0;

    if (!data)
        return 0;

    // Read the skb length
    if (bpf_core_read(&skb_len, sizeof(skb_len), &skb->len) < 0)
        return 0;

    // Check the first NETLINK_MSG_COUNT messages. We cap the number of messages
    // at 4 to make the verifier happy. We have seen that typically NEWRULE messages
    // appear as the second message. 
    #pragma unroll
    for (int i = 0; i < NETLINK_MSG_COUNT; i++) {
        if (skb_len < sizeof(struct nlmsghdr))
            return 0;

        if (bpf_probe_read_kernel(&nlh, sizeof(nlh), data) < 0)
            return 0;

        // type variable holds subsystem ID and command
        // subsys_id is the upper byte and cmd is the lower byte of the type
        __u16 type = nlh.nlmsg_type;
        __u8 subsys_id = type >> 8;
        __u8 cmd = type & 0xFF;
        __u32 nlmsg_len = nlh.nlmsg_len;

        if (subsys_id == NFNL_SUBSYS_NFTABLES && cmd == NFT_MSG_NEWRULE) {
            // If the message is a new rule, check if the parent process is allowed
            // and whether we are in the host network namespace.
            // If not allowed, increment the event counter and return -EPERM.
            if(is_allowed_parent()) {
                increment_event_counter(true);
                // Allow the operation
                return 0;
            } else {
                increment_event_counter(false);
                return -EPERM;
            }
        }

        data = data + nlmsg_len;
        skb_len = skb_len - nlmsg_len;
    }

    return 0;
}
