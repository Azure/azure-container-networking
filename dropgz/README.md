# dropgz payload deployment

`dropgz deploy` extracts each payload to a unique temporary file in the
destination directory. It completes the copy, closes the source, sets executable
permissions (`0755` on Unix), syncs and closes the temporary file, then publishes
it. Copy, decompression, close, and sync failures leave the destination unchanged.
Temporary files are removed on return; cleanup errors are reported.

On Unix, deployment copies the existing regular file through an open descriptor
to a staged backup and publishes it as `<destination>.old`. It then atomically
renames the completed temporary file over the destination. The live pathname is not removed first.
Open descriptors and running executables keep the previous inode. The destination
filesystem must support same-directory atomic rename.

On Windows, deployment retains the rename-to-`.old` sequence so that an executing
binary can be moved aside before replacement. Only completed files are published,
but the pathname can be absent between the two renames. If publication fails,
deployment attempts to restore `.old` and reports any restore error. Windows
file-sharing restrictions can still prevent replacement.

Directories and symlinks at the destination are rejected without moving them or
writing through them. An existing `.old` file is replaced, not written in place.
The destination directory must be trusted and writable by the deploying process.

Concurrent Unix deployments use unique temporary files and publish complete
files; the last successful rename wins. `.old` is a complete snapshot, but it
need not be the immediate predecessor of the winner when deployments overlap.
Windows publication is serialized within a process. Separate Windows deployment
processes must be serialized by the caller.

Publication is per file, not a transaction across all command arguments or across
the destination and its backup. A failed final rename can leave an updated `.old`.
Directory entries are not synced, so this is not a power-loss durability guarantee.

Run the deployment tests from this directory:

```sh
go test -race -timeout 2m ./pkg/embed
```
