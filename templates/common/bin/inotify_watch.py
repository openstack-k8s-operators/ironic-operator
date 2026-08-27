#!/usr/bin/env python3
# inotify_watch.py - Watch a directory for IN_CLOSE_WRITE events using
# Linux inotify syscalls via ctypes.
#
# This replaces the inotifywait command which requires inotify-tools
# (not packaged in RHEL).
#
# Usage: python3 inotify_watch.py <directory>
#
# Output format matches inotifywait -m <dir> -e close_write:
#   <directory>/ CLOSE_WRITE <filename>

import ctypes
import ctypes.util
import os
import signal
import struct
import sys

# inotify constants from <sys/inotify.h>
IN_CLOSE_WRITE = 0x00000008

# struct inotify_event {
#     int      wd;       /* Watch descriptor */
#     uint32_t mask;     /* Mask describing event */
#     uint32_t cookie;   /* Unique cookie associating related events */
#     uint32_t len;      /* Size of name field */
#     char     name[];   /* Optional null-terminated name */
# };
EVENT_HEADER_SIZE = struct.calcsize("iIII")
EVENT_HEADER_FMT = "iIII"

# Buffer size for reading events
BUF_SIZE = 4096


def main():
    if len(sys.argv) != 2:
        print("Usage: {} <directory>".format(sys.argv[0]), file=sys.stderr)
        sys.exit(1)

    watch_dir = sys.argv[1]

    if not os.path.isdir(watch_dir):
        print("Error: {} is not a directory".format(watch_dir),
              file=sys.stderr)
        sys.exit(1)

    # Ensure trailing slash to match inotifywait output format
    if not watch_dir.endswith("/"):
        watch_dir += "/"

    libc_name = ctypes.util.find_library("c")
    if libc_name is None:
        print("Error: cannot find libc", file=sys.stderr)
        sys.exit(1)
    libc = ctypes.CDLL(libc_name, use_errno=True)

    # int inotify_init(void)
    fd = libc.inotify_init()
    if fd < 0:
        errno = ctypes.get_errno()
        print("Error: inotify_init failed: {}".format(os.strerror(errno)),
              file=sys.stderr)
        sys.exit(1)

    # int inotify_add_watch(int fd, const char *pathname, uint32_t mask)
    wd = libc.inotify_add_watch(fd, watch_dir.encode("utf-8"), IN_CLOSE_WRITE)
    if wd < 0:
        errno = ctypes.get_errno()
        print("Error: inotify_add_watch failed: {}".format(
            os.strerror(errno)), file=sys.stderr)
        os.close(fd)
        sys.exit(1)

    # Handle SIGTERM for clean shutdown
    def handle_signal(signum, frame):
        os.close(fd)
        sys.exit(0)

    signal.signal(signal.SIGTERM, handle_signal)
    signal.signal(signal.SIGINT, handle_signal)

    # Monitor loop - read events and print to stdout
    try:
        while True:
            buf = os.read(fd, BUF_SIZE)
            offset = 0
            while offset < len(buf):
                wd_val, mask, cookie, name_len = struct.unpack_from(
                    EVENT_HEADER_FMT, buf, offset)
                offset += EVENT_HEADER_SIZE
                if name_len > 0:
                    name = buf[offset:offset + name_len]
                    # name is null-padded, strip null bytes
                    name = name.rstrip(b"\x00").decode("utf-8", errors="replace")
                    if mask & IN_CLOSE_WRITE:
                        print("{} CLOSE_WRITE {}".format(watch_dir, name),
                              flush=True)
                offset += name_len
    except OSError:
        # fd was closed by signal handler
        pass


if __name__ == "__main__":
    main()
