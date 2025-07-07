#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

#define EINVAL 13

SEC("lsm/netlink_send")
int BPF_PROG(block_nf_netlink, struct sock *sk, struct sk_buff *skb) {
    struct nlmsghdr nlh;
    int ret;

    // Read the first netlink message header
    ret = bpf_skb_load_bytes(skb, 0, &nlh, sizeof(nlh));
    if (ret < 0)
        return 0; // allow if failed to read

    // Parse subsystem from nlmsg_type
    __u16 type = nlh.nlmsg_type;
    __u8 subsys_id = type >> 8;

    // Block specific netfilter-related subsystems
    if (subsys_id == 0x0C /* NFNL_SUBSYS_NFTABLES */ ||
        subsys_id == 0x13 /* NFNL_SUBSYS_NFT_COMPAT */) {
        return -EINVAL;
    }

    return 0;
}

