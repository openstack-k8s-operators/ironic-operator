#!/bin/bash

# Ramdisk logs path
LOG_DIR=${LOG_DIR:-/var/lib/ironic/ramdisk-logs}

while :; do
    sleep 5

    while read -r fn; do
        echo
        echo "************ Contents of $fn ramdisk log file bundle **************"
        tar -xOzvvf "$fn" | sed -e "s/^/$(basename "$fn"): /"
        rm -f "$fn"
    # find all *.tar.gz files which are older than six seconds
    done < <(find "${LOG_DIR}" -mmin +0.1 -type f -name "*.tar.gz")

done
