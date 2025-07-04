// block_nl_nf.bpf.c

#define __TARGET_ARCH_x86
#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>


char LICENSE[] SEC("license") = "GPL";
#define EINVAL 13
#define sk_family __sk_common.skc_family
#define NETLINK_NETFILTER 12
#define NFT_MSG_NEWRULE 0x0A
#define NFNL_SUBSYS_NFTABLES 10

#define NFNL_SUBSYS_ID(type) ((type) >> 8)
#define NFNL_MSG_TYPE(type)  ((type) & 0xFF)

__u32 host_netns_inode = 4026531840;  // Initialized by userspace, not const

SEC("lsm/socket_create")
int BPF_PROG(check_netns, int family, int type, int protocol)
{
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

    if (netns_ino == host_netns_inode) {
        bpf_printk("In host netns\n");
    } else {
        bpf_printk("In non-host netns: %u\n", netns_ino);
    }

    return 0;
}



// LSM hook: security_socket_create(family, type, protocol, kern)
//SEC("lsm/socket_create")
//int BPF_PROG(block_nf_netlink, int family, int type, int protocol, int kern)
//{
//    bpf_printk("socket create called %d %d\n", family, protocol);
//    if (family == 16 && protocol == 12) {
//        bpf_printk("Blocking NETLINK_NETFILTER socket\n");
//        return -EINVAL;
//    }
//    return 0;
//}

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


    bpf_printk("setsockopt called %d %d\n", level, optname);
    if(level == 0 /*IPPROTO_IP*/ || level == 41 /*IPPROTO_IP6*/) {
        if(optname == 64) { // 64 represents IPT_SO_SET_REPLACE or IP6T_SO_SET_REPLACE, depending on the level
            if(is_host_ns()) {
                return -EINVAL;
            }
        }
    }

    return 0;
}

SEC("lsm/socket_sendmsg")
int BPF_PROG(on_socket_sendmsg, struct socket *sock, struct msghdr *msg, size_t size)
{
    if(sock == NULL) {
        return 0;
    }

    struct sock *sk = sock->sk;
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

    struct iov_iter iter;
    if (bpf_core_read(&iter, sizeof(iter), &msg->msg_iter))
        return 0;

    // Ensure it's ITER_IOVEC (usually the case for sendmsg from user space)
    if (iter.count == 0 || !iter.iov)
        return 0;

    struct iovec iov;
    if (bpf_core_read(&iov, sizeof(iov), iter.iov))
        return 0;

    if (iov.iov_len < sizeof(struct nlmsghdr) || !iov.iov_base)
        return 0;

    struct nlmsghdr nlh;
    if (bpf_probe_read_user(&nlh, sizeof(nlh), iov.iov_base))
        return 0;

    __u16 subsys_id = NFNL_SUBSYS_ID(nlh.nlmsg_type);
    __u16 cmd_type  = NFNL_MSG_TYPE(nlh.nlmsg_type);

    if (subsys_id == NFNL_SUBSYS_NFTABLES && cmd_type == NFT_MSG_NEWRULE)
        return -EINVAL;


//    bpf_printk("send called on netfilter socket\n");

//    if(msg!=NULL && msg->msg_name != NULL) {
//       char buf[16];
//       __u32 user_len = 0;
//       bpf_probe_read_kernel(&user_len, sizeof(user_len), &msg->msg_namelen);
//       if (user_len < 16) {
//           bpf_probe_read_user(buf, user_len, msg->msg_name);
//           // Now print first few bytes as integers:
//           bpf_printk("first bytes: %c%c%c%c%c \n", buf[0], buf[1], buf[2], buf[3]);
//        }
//
//    }
    // Read first iovec
    //struct iovec iov0 = {};
    // msg->msg_iov is a pointer to user memory containing an array of iovec
    //bpf_probe_read_kernel(&iov0, sizeof(iov0), &msg->msg_iov[0]);

    // iov0.iov_base is user-space pointer to Netlink buffer
    //void *user_nlh = iov0.iov_base;
    //struct nlmsghdr hdr = {};
    // Read the Netlink message header from user-space
    //if (bpf_probe_read_user(&hdr, sizeof(hdr), user_nlh) < 0)
    //    return 0;  // failed to read header; allow by default

    //__u16 nlmsg_type = hdr.nlmsg_type;
    // Extract NFNL subsystem and command: high byte = subsystem, low byte = command
    //__u8 subsys = nlmsg_type >> 8;
    //__u8 cmd    = nlmsg_type & 0xff;

    // Now check for “new rule” commands in the Netfilter Netlink protocol
    // For nftables: NFNL_SUBSYS_NFTABLES and NFT_MSG_NEWRULE
    // These constants are defined in kernel headers, e.g.:
    //   #define NFNL_SUBSYS_NFTABLES 10
    //   #define NFT_MSG_NEWRULE     0  (or appropriate value from nf_tables.h)
    //if (subsys == NFNL_SUBSYS_NFTABLES && cmd == NFT_MSG_NEWRULE) {
    //    // Block the operation
    //    return -EINVAL;
    //}

    // (Optionally: handle other NFNL subsystems or commands, e.g., ipset, conntrack, etc.)

    return 0;
}
