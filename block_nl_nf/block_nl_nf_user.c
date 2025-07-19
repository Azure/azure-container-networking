//#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <errno.h>
#include <sys/inotify.h>
#include <sys/stat.h>
#include <bpf/libbpf.h>
#include "block_nl_nf.skel.h"

#define DEFAULT_CONFIG_FILE "/etc/cni/net.d/iptables-allow-list"
#define EVENT_SIZE (sizeof(struct inotify_event))
#define BUF_LEN (1024 * (EVENT_SIZE + 16))

static __u32 get_host_netns_inode(void) {
    struct stat st;
    
    // Read the network namespace inode of the current process (which should be host ns)
    if (stat("/proc/self/ns/net", &st) != 0) {
        fprintf(stderr, "Failed to stat /proc/self/ns/net: %s\n", strerror(errno));
        return 0;
    }
    
    printf("Host network namespace inode: %lu\n", (unsigned long)st.st_ino);
    return (__u32)st.st_ino;
}

static int is_file_empty_or_missing(const char *filename) {
    struct stat st;
    
    // Check if file exists
    if (stat(filename, &st) != 0) {
        if (errno == ENOENT) {
            printf("Config file %s does not exist\n", filename);
            return -1; // File missing (special case)
        }
        fprintf(stderr, "Error checking file %s: %s\n", filename, strerror(errno));
        return -1; // Treat errors as missing
    }
    
    // Check if file is empty
    if (st.st_size == 0) {
        printf("Config file %s is empty\n", filename);
        return 1; // File empty
    }
    
    printf("Config file %s has content (size: %ld bytes)\n", filename, st.st_size);
    return 0; // File exists and has content
}

static int attach_bpf_program(struct block_nl_nf **skel_ptr) {
    struct block_nl_nf *skel;
    int err;
    __u32 host_netns_inode;
    
    if (*skel_ptr) {
        printf("BPF program already attached\n");
        return 0;
    }
    
    printf("Attaching BPF program...\n");
    
    skel = block_nl_nf__open();
    if (!skel) {
        fprintf(stderr, "Failed to open BPF skeleton\n");
        return -1;
    }
    
    // Get the host network namespace inode
    host_netns_inode = get_host_netns_inode();
    if (host_netns_inode == 0) {
        fprintf(stderr, "Failed to get host network namespace inode\n");
        block_nl_nf__destroy(skel);
        return -1;
    }
    
    // Set the host_netns_inode in the BPF program before loading
    skel->data->host_netns_inode = host_netns_inode;
    
    err = block_nl_nf__load(skel);
    if (err) {
        fprintf(stderr, "Failed to load BPF object: %d\n", err);
        block_nl_nf__destroy(skel);
        return -1;
    }
    
    err = block_nl_nf__attach(skel);
    if (err) {
        fprintf(stderr, "Failed to attach BPF program: %d\n", err);
        block_nl_nf__destroy(skel);
        return -1;
    }
    
    *skel_ptr = skel;
    printf("BPF program attached successfully with host_netns_inode=%u\n", host_netns_inode);
    return 0;
}

static int detach_bpf_program(struct block_nl_nf **skel_ptr) {
    if (!*skel_ptr) {
        printf("BPF program already detached\n");
        return 0;
    }
    
    printf("Detaching BPF program...\n");
    block_nl_nf__destroy(*skel_ptr);
    *skel_ptr = NULL;
    printf("BPF program detached successfully\n");
    return 0;
}

static int setup_inotify_watch(const char *filename) {
    int fd, wd;
    char *dir_path;
    char *filename_copy;
    char *dir_end;
    
    fd = inotify_init();
    if (fd < 0) {
        fprintf(stderr, "Failed to initialize inotify: %s\n", strerror(errno));
        return -1;
    }
    
    // Extract directory path from filename
    filename_copy = strdup(filename);
    if (!filename_copy) {
        fprintf(stderr, "Failed to allocate memory\n");
        close(fd);
        return -1;
    }
    
    dir_end = strrchr(filename_copy, '/');
    if (dir_end) {
        *dir_end = '\0';
        dir_path = filename_copy;
    } else {
        dir_path = ".";
    }
    
    // Watch the directory for changes
    wd = inotify_add_watch(fd, dir_path, IN_CREATE | IN_DELETE | IN_MODIFY | IN_MOVED_TO | IN_MOVED_FROM);
    if (wd < 0) {
        fprintf(stderr, "Failed to add inotify watch for %s: %s\n", dir_path, strerror(errno));
        free(filename_copy);
        close(fd);
        return -1;
    }
    
    printf("Watching directory %s for changes to %s\n", dir_path, filename);
    free(filename_copy);
    return fd;
}

int main(int argc, char *argv[]) {
    const char *config_file = DEFAULT_CONFIG_FILE;
    struct block_nl_nf *skel = NULL;
    int inotify_fd;
    char buffer[BUF_LEN];
    int length;
    int i;
    
    // Parse command line arguments
    if (argc > 1) {
        config_file = argv[1];
    }
    
    printf("Using config file: %s\n", config_file);
    
    // Initial state check
    int file_state = is_file_empty_or_missing(config_file);
    if (file_state == 1) {
        // File is empty, attach BPF program
        if (attach_bpf_program(&skel) < 0) {
            return 1;
        }
    } else if (file_state == 0) {
        // File has content, don't attach
        printf("Config file has content, BPF program will remain detached\n");
    } else {
        // File is missing, wait for it to be created
        printf("Config file is missing, waiting for file to be created...\n");
    }
    
    // Setup inotify watch
    inotify_fd = setup_inotify_watch(config_file);
    if (inotify_fd < 0) {
        if (skel) {
            block_nl_nf__destroy(skel);
        }
        return 1;
    }
    
    printf("Starting file watch loop...\n");
    
    // Main event loop
    while (1) {
        length = read(inotify_fd, buffer, BUF_LEN);
        if (length < 0) {
            fprintf(stderr, "Error reading inotify events: %s\n", strerror(errno));
            break;
        }
        
        // Process inotify events
        for (i = 0; i < length; ) {
            struct inotify_event *event = (struct inotify_event *)&buffer[i];
            
            // Check if the event is for our config file
            if (event->len > 0) {
                char *filename = strrchr(config_file, '/');
                filename = filename ? filename + 1 : config_file;
                
                if (strcmp(event->name, filename) == 0) {
                    printf("Config file changed: %s\n", event->name);
                    
                    // Check current state and take action
                    int file_state = is_file_empty_or_missing(config_file);
                    if (file_state == 1) {
                        // File is empty, attach BPF program
                        if (attach_bpf_program(&skel) < 0) {
                            fprintf(stderr, "Failed to attach BPF program\n");
                        }
                    } else if (file_state == 0) {
                        // File has content, detach BPF program
                        if (detach_bpf_program(&skel) < 0) {
                            fprintf(stderr, "Failed to detach BPF program\n");
                        }
                    } else {
                        // File is missing, detach BPF program
                        printf("Config file was deleted, detaching BPF program\n");
                        if (detach_bpf_program(&skel) < 0) {
                            fprintf(stderr, "Failed to detach BPF program\n");
                        }
                    }
                }
            }
            
            i += EVENT_SIZE + event->len;
        }
    }
    
    // Cleanup
    if (skel) {
        block_nl_nf__destroy(skel);
    }
    close(inotify_fd);
    
    return 0;
}

