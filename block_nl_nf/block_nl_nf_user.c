// block_nl_nf_user.c

#include <stdio.h>
#include <stdlib.h>
#include <bpf/libbpf.h>
#include <unistd.h>
#include "block_nl_nf.skel.h"

int main()
{
    struct block_nl_nf *skel;
    int err;

    skel = block_nl_nf__open();
    if (!skel) {
        fprintf(stderr, "Failed to open BPF skeleton\n");
        return 1;
    }

    err = block_nl_nf__load(skel);
    if (err) {
        fprintf(stderr, "Failed to load BPF object: %d\n", err);
        return 1;
    }

    err = block_nl_nf__attach(skel);
    if (err) {
        fprintf(stderr, "Failed to attach BPF program: %d\n", err);
    }

    while(true) {
	sleep(1);
    }

}

