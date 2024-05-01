#include <netinet/in.h>
#include <stdbool.h>

#define L4_HDR_OFF (ETH_HLEN + sizeof(struct ipv6hdr))
#define BPF_F_PSEUDO_HDR (1ULL << 4)

static __always_inline bool compare_ipv6_addr(const struct in6_addr *addr1, const struct in6_addr *addr2)
{
#pragma unroll
    for (int i = 0; i < sizeof(struct in6_addr); i++)
    {
        if (addr1->s6_addr[i] != addr2->s6_addr[i])
        {
            return false;
        }
    }
    return true;
}