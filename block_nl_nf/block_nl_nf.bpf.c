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
#define NLA_ALIGNTO 4
#define NLA_ALIGN(len) (((len) + NLA_ALIGNTO - 1) & ~(NLA_ALIGNTO - 1))
#define NLA_HDRLEN ((int) NLA_ALIGN(sizeof(struct nlattr)))
#define NLA_F_NESTED (1 << 15)

struct nfgenmsg {
    __u8 nfgen_family;    /* AF_xxx */
    __u8 version;         /* nfnetlink version */
    __be16 res_id;        /* resource id */
};

static const char ALLOWED_CHAINS[][MAX_CHAIN_LEN] = {
    "IP-MASQ-AGENT",
    "CILIUM"
};
static const char COMMENTSTR[] = "comment";
static const int ALLOWED_CHAINS_SIZES[MAX_CHAIN_LEN] = {
    sizeof("IP-MASQ-AGENT") - 1,
    sizeof("CILIUM") - 1
};
static char payload[32];

static __always_inline int is_comment(void *attr1_ptr, __u32 remaining1) {
    if (remaining1 < NLA_HDRLEN) {
        return 0;
    }

    struct nlattr attr1 = {};
    if (bpf_probe_read_kernel(&attr1, sizeof(attr1), attr1_ptr) < 0)
        return 0;

    __u16 attr1_len = attr1.nla_len;
    __u16 attr1_type = attr1.nla_type & 0x3fff;

    if (attr1_len < sizeof(struct nlattr) || attr1_len > remaining1)
        return 0;

    int copy_len = attr1_len - NLA_HDRLEN;
    if (copy_len > 32) {
        copy_len = 32;
    }

    void *payload_ptr = attr1_ptr + NLA_HDRLEN;
    if (bpf_probe_read_kernel(payload, copy_len, payload_ptr) < 0)
        return 0;

    bpf_printk("payload: %s", payload);
    if (payload[0] == 'c' && payload[1] == 'o' && payload[2] == 'm' &&
        payload[3] == 'm' && payload[4] == 'e' && payload[5] == 'n' &&
        payload[6] == 't') {
        bpf_printk("found comment");
        return 1;
    } else {
        return 0;
    }
}

static __always_inline int is_cilium_comment(void *attr1_ptr, __u32 remaining) {
    if (remaining < NLA_HDRLEN) {
        return 0;
    }

    struct nlattr attr1 = {};
    if (bpf_probe_read_kernel(&attr1, sizeof(attr1), attr1_ptr) < 0)
        return 0;

    __u16 attr1_len = attr1.nla_len;
    __u16 attr1_type = attr1.nla_type & 0x3fff;

    if (attr1_len < sizeof(struct nlattr) || attr1_len > remaining)
        return 0;

    void *attr2_ptr = attr1_ptr + NLA_ALIGN(attr1_len);
    remaining = remaining - NLA_ALIGN(attr1_len);

    if (remaining < sizeof(struct nlattr))
        return 0;

    struct nlattr attr2 = {};
    if (bpf_probe_read_kernel(&attr2, sizeof(attr2), attr2_ptr) < 0)
        return 0;

    __u16 attr2_len = attr2.nla_len;
    remaining -= NLA_ALIGN(attr2_len);
    if (remaining < NLA_HDRLEN) {
        return 0;
    }

    struct nlattr attr3 = {};
    void *attr3_ptr = attr2_ptr + NLA_ALIGN(attr2_len);
    if (bpf_probe_read_kernel(&attr3, sizeof(attr3), attr3_ptr) < 0)
        return 0;

    __u16 attr3_len = attr3.nla_len;

    if (attr3_len < sizeof(struct nlattr) || attr3_len > remaining)
        return 0;

    int copy_len = attr3_len - NLA_HDRLEN;
    if (copy_len > 32) {
        copy_len = 32;
    }

    void *payload_ptr = attr3_ptr + NLA_HDRLEN;
    if (bpf_probe_read_kernel(payload, copy_len, payload_ptr) < 0)
        return 0;

    bpf_printk("payload: %s", payload);
    if (payload[0] == 'C' && payload[1] == 'I' && payload[2] == 'L' &&
        payload[3] == 'I' && payload[4] == 'U' && payload[5] == 'M') {
        bpf_printk("found cilium");
        return 1;
    } else {
        return 0;
    }

    return 0;
}

static __always_inline int is_chain_allowed_or_missing(void *data, __u32 data_len) {
    // Check we can read nfgenmsg
    if (data_len < sizeof(struct nfgenmsg))
        return 1;

    bpf_printk("read past nfgenmsg");
    void *attr1_ptr = data + sizeof(struct nfgenmsg);
    __u32 remaining1 = data_len - sizeof(struct nfgenmsg);

    #pragma unroll
    for (int i = 0; i < 4; i++) {
        if (remaining1 < sizeof(struct nlattr))
            break;

        struct nlattr attr1 = {};
        if (bpf_probe_read_kernel(&attr1, sizeof(attr1), attr1_ptr) < 0)
            break;

        __u16 attr1_len = attr1.nla_len;
        __u16 attr1_type = attr1.nla_type & 0x3fff;

        bpf_printk("attr1 type: %d", attr1_type);

        if (attr1_len < sizeof(struct nlattr) || attr1_len > remaining1)
            break;

        if ((attr1.nla_type & NLA_F_NESTED) && attr1_type == 0x4) {
            void *attr2_ptr = attr1_ptr + NLA_HDRLEN;
            __u32 remaining2 = remaining1 - NLA_HDRLEN;

            for (int k = 0; k < 4; k++) {
                if (remaining2 < sizeof(struct nlattr))
                    break;

                struct nlattr attr2 = {};
                if (bpf_probe_read_kernel(&attr2, sizeof(attr2), attr2_ptr) < 0)
                    break;

                __u16 attr2_len = attr2.nla_len;
                __u16 attr2_type = attr2.nla_type & 0x3fff;
                bpf_printk("read attr2 type %d is nested? %d", attr2_type, (attr2.nla_type & NLA_F_NESTED));
                bpf_printk("attr2 len %d", attr2_len);

                if (attr2.nla_type & NLA_F_NESTED) {
                    void *attr3_ptr = attr2_ptr + NLA_HDRLEN;
                    __u32 remaining3 = remaining2 - NLA_HDRLEN;

                    for (int l = 0; l < 3; l++) {
                        if (remaining3 < sizeof(struct nlattr))
                            break;

                        struct nlattr attr3 = {};
                        if (bpf_probe_read_kernel(&attr3, sizeof(attr3), attr3_ptr) < 0)
                            break;

                        __u16 attr3_len = attr3.nla_len;
                        __u16 attr3_type = attr3.nla_type & 0x3fff;
                        bpf_printk("read attr3 type %d is nested? %d", attr3_type, (attr3.nla_type & NLA_F_NESTED));
                        bpf_printk("attr3 len %d", attr3_len);

                        if (attr3.nla_type & NLA_F_NESTED) {
                            void *attr4_ptr = attr3_ptr + NLA_HDRLEN;
                            __u32 remaining4 = remaining3 - NLA_HDRLEN;

                            for (int m = 0; m < 3; m++) {
                                if (remaining4 < sizeof(struct nlattr))
                                    break;

                                struct nlattr attr4 = {};
                                if (bpf_probe_read_kernel(&attr4, sizeof(attr4), attr4_ptr) < 0)
                                    break;

                                __u16 attr4_len = attr4.nla_len;
                                __u16 attr4_type = attr4.nla_type & 0x3fff;
                                bpf_printk("read attr4 type %d is nested? %d", attr4_type, (attr4.nla_type & NLA_F_NESTED));
                                bpf_printk("attr4 len %d", attr4_len);

                                if (attr4.nla_type & NLA_F_NESTED) {
                                    // We don't handle deeper nesting, so we just skip it
                                    bpf_printk("skipping deeper nested attr4");
                                } else {
                                    if (is_comment(attr4_ptr, remaining4)) {
                                        if (is_cilium_comment(attr4_ptr, remaining4)) {
                                            bpf_printk("found cilium comment in attr4");
                                            return 1; // allow because we found cilium comment
                                        }
                                    }
                                }
                                bpf_printk("attr4 payload %s", attr4_ptr + NLA_HDRLEN);

                                attr4_ptr += NLA_ALIGN(attr4_len);
                                remaining4 -= NLA_ALIGN(attr4_len);
                            }
                        } else {
                            bpf_printk("attr3 payload %s", attr3_ptr + NLA_HDRLEN);
                        }

                        attr3_ptr += NLA_ALIGN(attr3_len);
                        remaining3 -= NLA_ALIGN(attr3_len);
                    }
                } else {
                    bpf_printk("attr2 payload %s", attr2_ptr + NLA_HDRLEN);
                }

                attr2_ptr += NLA_ALIGN(attr2_len);
                remaining2 -= NLA_ALIGN(attr2_len);
            }
        }

        attr1_ptr += NLA_ALIGN(attr1_len);
        remaining1 -= NLA_ALIGN(attr1_len);
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
    if (sock == NULL) {
        return 0;
    }

    // bpf_printk("setsockopt called %d %d\n", level, optname);
    if (level == 0 /*IPPROTO_IP*/ || level == 41 /*IPPROTO_IP6*/) {
        if (optname == 64) { // 64 represents IPT_SO_SET_REPLACE or IP6T_SO_SET_REPLACE, depending on the level
            if (is_host_ns()) {
                return -EPERM;
            }
        }
    }

    return 0;
}

SEC("lsm/netlink_send")
int BPF_PROG(block_nf_netlink, struct sock *sk, struct sk_buff *skb) {
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
            if (is_chain_allowed_or_missing(data + sizeof(nlh), skb_len - sizeof(nlh))) {
                return 0;
            }
            return -EPERM;
        }

        data = data + nlmsg_len;
        skb_len = skb_len - nlmsg_len;
    }

    return 0;
}
