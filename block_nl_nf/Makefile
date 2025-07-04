BPF_CLANG=clang
BPF_CFLAGS=-g -O2 -target bpf

all: block_nl_nf.skel.h block_nl_nf_user

block_nl_nf.o: block_nl_nf.bpf.c vmlinux.h
	$(BPF_CLANG) $(BPF_CFLAGS) -c $< -o $@

block_nl_nf.skel.h: block_nl_nf.o
	bpftool gen skeleton $< > $@

block_nl_nf_user: block_nl_nf_user.c block_nl_nf.skel.h
	$(CC) -g -O2 -o $@ $< -lbpf -lelf -lz

clean:
	rm -f *.o *.skel.h block_nl_nf_user

