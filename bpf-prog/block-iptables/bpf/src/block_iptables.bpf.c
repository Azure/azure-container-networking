#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

#define sk_family __sk_common.skc_family
#define EPERM 1
#define IPT_SO_SET_REPLACE
char LICENSE[] SEC("license") = "GPL";

volatile const __u32 host_netns_inode = 4026531840; // Initialized by userspace
#define TASK_COMM_LEN 16
#define COMM_COUNT 3
int is_allowed_parent ()
{
    struct task_struct *task = (struct task_struct *)bpf_get_current_task();
    struct task_struct *parent_task = NULL;

    char parent_comm[TASK_COMM_LEN] = {};
    const char target_prefixes[COMM_COUNT][TASK_COMM_LEN] = {"cilium-agent", "ip-masq", "azure-cns"};

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
            bpf_printk("Allowed netlink from parent: %s\n", parent_comm);
            return 1;
        }
    }


    return 0; // Block
}

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

SEC("lsm/socket_setsockopt")
int BPF_PROG(iptables_legacy_block, struct socket *sock, int level, int optname)
{
    if (sock == NULL) {
        return 0;
    }

    // bpf_printk("setsockopt called %d %d\n", level, optname);
    if (level == 0 /*IPPROTO_IP*/ || level == 41 /*IPPROTO_IP6*/) {
        if (optname == IPT_SO_SET_REPLACE) { // IPT_SO_SET_REPLACE represents IPT_SO_SET_REPLACE or IP6T_SO_SET_REPLACE, depending on the level
            if (is_host_ns() && !is_allowed_parent()) {
                return -EPERM;
            }
        }
    }

    return 0;
}

SEC("lsm/netlink_send")
int BPF_PROG(iptables_nftables_block, struct sock *sk, struct sk_buff *skb) {
    __u16 family = 0, proto = 0;
    if (sk != NULL) {
        bpf_probe_read_kernel(&family, sizeof(family), &sk->sk_family);
    }

    if (family != 16) // Not AF_NETLINK
        return 0;

    if (sk != NULL) {
        bpf_probe_read_kernel(&proto, sizeof(proto), &sk->sk_protocol);
    }

    if (proto != 12) // Not NETLINK_NETFILTER
        return 0;

    if (!is_host_ns()) {
        return 0;
    }

    struct nlmsghdr nlh = {};
    void *data = NULL;
    __u32 skb_len = 0;

    if (bpf_core_read(&data, sizeof(data), &skb->data) < 0)
        return 0;

    if (!data)
        return 0;

    if (bpf_core_read(&skb_len, sizeof(skb_len), &skb->len) < 0)
        return 0;

    #pragma unroll
    for (int i = 0; i < 4; i++) {
        if (skb_len < sizeof(struct nlmsghdr))
            return 0;

        if (bpf_probe_read_kernel(&nlh, sizeof(nlh), data) < 0)
            return 0;
        __u16 type = nlh.nlmsg_type;
        __u8 subsys_id = type >> 8;
        __u8 cmd = type & 0xFF;
        __u32 nlmsg_len = nlh.nlmsg_len;

        if (subsys_id == 10 && cmd == 6) {
            if(is_allowed_parent()) {
                    return 0;
            } else {
                    return -EPERM;
            }
        }

        data = data + nlmsg_len;
        skb_len = skb_len - nlmsg_len;
    }

    return 0;
}
