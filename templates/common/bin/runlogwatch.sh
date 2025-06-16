#!/bin/bash

# Ramdisk logs path
LOG_DIR=${LOG_DIR:-/var/lib/ironic/ramdisk-logs}

# NOTE(hjensas): OSPRH-17520
if ! command -v inotifywait; then
    echo "WARNING: ramdisk-logs will not be captured, because the inotifywait command is missing. Please patch the OpenstackVersion to update container images."
    # Start a tail on /dev/null to do nothing forever
    tail -f /dev/null
fi

inotifywait -m "${LOG_DIR}" -e close_write |
    while read -r path _action file; do
        echo "************ Contents of ${path}${file} ramdisk log file bundle **************"
        tar -xOzvvf "${path}${file}" | sed -e "s/^/${file}: /"
        rm -f "${path}/${file}"
    done
