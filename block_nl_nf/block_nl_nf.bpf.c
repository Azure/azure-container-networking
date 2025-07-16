#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

#define sk_family __sk_common.skc_family
#define EPERM 1
char LICENSE[] SEC("license") = "GPL";

__u32 host_netns_inode = 4026531840;  // Initialized by userspace, not const

#define MAX_CHAIN_LEN 32
#define NFTA_RULE_CHAIN 2
#define NLA_ALIGNTO		4
#define NLA_ALIGN(len)		(((len) + NLA_ALIGNTO - 1) & ~(NLA_ALIGNTO - 1))
#define NLA_HDRLEN		((int) NLA_ALIGN(sizeof(struct nlattr)))

struct nfgenmsg {
        __u8  nfgen_family;             /* AF_xxx */
        __u8  version;          /* nfnetlink version */
        __be16    res_id;               /* resource id */
};

static const char ALLOWED_CHAINS[][MAX_CHAIN_LEN] = {
    "IP-MASQ-AGENT",
    "CILIUM"
};
static const int ALLOWED_CHAINS_SIZES[MAX_CHAIN_LEN] = {
    sizeof("IP-MASQ-AGENT") - 1,
    sizeof("CILIUM") - 1
};

static __always_inline int is_chain_allowed_or_missing(void *data, __u32 data_len) {
    // Check we can read nfgenmsg
    if (data_len < sizeof(struct nfgenmsg))
        return 1;

    bpf_printk("read past nfgenmsg");
    void *attr_ptr = data + sizeof(struct nfgenmsg);
    __u32 remaining = data_len - sizeof(struct nfgenmsg);

    #pragma unroll
    for (int i = 0; i < 4; i++) {
        if (remaining < sizeof(struct nlattr))
            break;

        bpf_printk("reading 1 attribute");

        struct nlattr attr = {};
        if (bpf_probe_read_kernel(&attr, sizeof(attr), attr_ptr) < 0)
            break;

        __u16 attr_len = attr.nla_len;
        __u16 attr_type = attr.nla_type & 0x3fff;

        bpf_printk("read %d", attr_type);

        if (attr_len < sizeof(struct nlattr) || attr_len > remaining)
            break;

        if (attr_type == NFTA_RULE_CHAIN) {
            char chain[MAX_CHAIN_LEN] = {};
            __u32 copy_len = attr_len - sizeof(struct nlattr);
            if (copy_len >= MAX_CHAIN_LEN)
                copy_len = MAX_CHAIN_LEN - 1;

            if (bpf_probe_read_kernel(chain, copy_len, attr_ptr + sizeof(struct nlattr)) < 0)
                break;
            bpf_printk("chain is %s", chain);

            #pragma unroll
            for (int j = 0; j < 2; j++) {
                if (__builtin_memcmp(chain, ALLOWED_CHAINS[j], ALLOWED_CHAINS_SIZES[j]) == 0) {
                    return 1; // explicitly allowed
                }
            }

            return 0; // explicitly found and disallowed
        }

        attr_ptr += NLA_ALIGN(attr_len);
        remaining -= NLA_ALIGN(attr_len);
    }

    return 0; // no NFTA_RULE_CHAIN found → allow
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
                return -EPERM;
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
	    if(is_chain_allowed_or_missing(data + sizeof(nlh), skb_len - sizeof(nlh))) {
                return 0;
	    }

	    return -EPERM;
	}

	data = data + nlmsg_len;
	skb_len = skb_len - nlmsg_len;

    }

    return 0;
}
