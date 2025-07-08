#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

#define sk_family __sk_common.skc_family
#define EINVAL 13
char LICENSE[] SEC("license") = "GPL";

__u32 host_netns_inode = 4026531840;  // Initialized by userspace, not const

int is_host_ns() {
    struct task_struct *task = (struct task_struct *)bpf_get_current_task();
    struct nsproxy *nsproxy;
    struct net *net_ns;
    u32 netns_ino = 0;

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

// LSM hook: security_socket_create(family, type, protocol, kern)
SEC("lsm/socket_setsockopt")
int BPF_PROG(iptables_legacy_block, struct socket *sock, int level, int optname)
{
    if(sock == NULL) {
        return 0;
    }


    //bpf_printk("setsockopt called %d %d\n", level, optname);
    if(level == 0 /*IPPROTO_IP*/ || level == 41 /*IPPROTO_IP6*/) {
        if(optname == 64) { // 64 represents IPT_SO_SET_REPLACE or IP6T_SO_SET_REPLACE, depending on the level
            if(is_host_ns()) {
                return -EINVAL;
            }
        }
    }

    return 0;
}

SEC("lsm/netlink_send")
int BPF_PROG(block_nf_netlink, struct sock *sk, struct sk_buff *skb) {
    __u16 family = 0, proto = 0;
    if(sk != NULL) {
        bpf_probe_read_kernel(&family, sizeof(family), &sk->sk_family);
    }

    if (family != 16) // Not AF_NETLINK
        return 0;

    if(sk != NULL) {
        bpf_probe_read_kernel(&proto, sizeof(proto), &sk->sk_protocol);
    }

    if (proto != 12) // Not NETLINK_NETFILTER
        return 0;

    struct nlmsghdr nlh = {};
    void *data = NULL;
    __u32 skb_len = 0;

    // Step 1: Read skb->len to ensure there's enough data
    if (bpf_core_read(&skb_len, sizeof(skb_len), &skb->len) < 0)
        return 0;

    bpf_printk("step1");
    if (skb_len < sizeof(struct nlmsghdr))
        return 0;
    bpf_printk("step2");
    // Step 2: Read skb->data pointer safely
    if (bpf_core_read(&data, sizeof(data), &skb->data) < 0)
        return 0;
    bpf_printk("step3");
    // Step 3: Validate that skb->data is not NULL
    if (!data)
        return 0;
    bpf_printk("step4");
    // Step 4: Read the nlmsghdr from skb->data
    if (bpf_probe_read_kernel(&nlh, sizeof(nlh), data) < 0)
        return 0;
    bpf_printk("step5");
    // Step 5: Extract subsystem ID from nlmsg_type
    __u16 type = nlh.nlmsg_type;
    __u8 subsys_id = type >> 8;

    // Step 6: Optionally validate nlmsg_len (sanity check)
    if (nlh.nlmsg_len < sizeof(struct nlmsghdr) || nlh.nlmsg_len > skb_len)
        return 0;
    bpf_printk("step6");
    // Step 7: Block known netfilter-related subsystems
    if (subsys_id == 0x0A /* NFNL_SUBSYS_NFTABLES */ ||
        subsys_id == 0x0B /* NFNL_SUBSYS_NFT_COMPAT */) {
        return -EINVAL;
    }
    bpf_printk("step7");
    return 0;
}
