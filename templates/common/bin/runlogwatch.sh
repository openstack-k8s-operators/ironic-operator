#!/bin/bash

# Ramdisk logs path
LOG_DIR=${LOG_DIR:-/var/lib/ironic/ramdisk-logs}

SCRIPT_DIR=$(dirname "$0")

python3 "${SCRIPT_DIR}/inotify_watch.py" "${LOG_DIR}" |
    while read -r path _action file; do
        echo "************ Contents of ${path}${file} ramdisk log file bundle **************"
        tar -xOzvvf "${path}${file}" | sed -e "s/^/${file}: /"
        rm -f "${path}/${file}"
    done
