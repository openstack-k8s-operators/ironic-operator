#!/bin/bash
#
# Copyright 2023 Red Hat Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may
# not use this file except in compliance with the License. You may obtain
# a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and limitations
# under the License.
set -ex

# Get the statefulset pod index
export PODINDEX=$(echo ${HOSTNAME##*-})

# DHCP server configuration
export InspectorNetworkIP=$(/usr/local/bin/container-scripts/get_net_ip ${InspectionNetwork})
export INSPECTOR_HTTP_URL=$(python3 -c '
import os
import ipaddress

# Get the IP address
ip_str = os.environ.get("InspectorNetworkIP", "")

# Check if it is an IPv6 address and format accordingly
try:
    ip = ipaddress.ip_address(ip_str)
    if isinstance(ip, ipaddress.IPv6Address):
        # For IPv6, we need to wrap the IP in brackets for URL formatting
        formatted_env = dict(os.environ)
        formatted_env["InspectorNetworkIP"] = f"[{ip_str}]"
        print(os.environ["InspectorHTTPURL"] % formatted_env)
    else:
        # For IPv4, use as-is
        print(os.environ["InspectorHTTPURL"] % os.environ)
except ValueError:
    # If IP parsing fails, use as-is (fallback)
    print(os.environ["InspectorHTTPURL"] % os.environ)
')

# Export URL-formatted IP for use in config templates
export InspectorNetworkIPForURL=$(python3 -c '
import os
import ipaddress

# Get the IP address
ip_str = os.environ.get("InspectorNetworkIP", "")

# Check if it is an IPv6 address and format accordingly
try:
    ip = ipaddress.ip_address(ip_str)
    if isinstance(ip, ipaddress.IPv6Address):
        print(f"[{ip_str}]")
    else:
        print(ip_str)
except ValueError:
    # If IP parsing fails, use as-is (fallback)
    print(ip_str)
')

# Copy required config to modifiable location
cp /var/lib/config-data/default/dnsmasq.conf /var/lib/ironic/
cp /var/lib/config-data/default/inspector.ipxe /var/lib/ironic/

export DNSMASQ_CFG=/var/lib/ironic/dnsmasq.conf
sed -e "/BLOCK_PODINDEX_${PODINDEX}_BEGIN/,/BLOCK_PODINDEX_${PODINDEX}_END/p" \
    -e "/BLOCK_PODINDEX_.*_BEGIN/,/BLOCK_PODINDEX_.*_END/d" \
    -i ${DNSMASQ_CFG}
sed -e "/BLOCK_PODINDEX_${PODINDEX}_BEGIN/d" \
    -e "/BLOCK_PODINDEX_${PODINDEX}_END/d" \
    -i ${DNSMASQ_CFG}

# OSPCIX-870: Copy the config to tempdir to avoid race condition where pipe to
#             tee may wipe the file.
export TMP_DNSMASQ_CFG=/var/tmp/dnsmasq.conf
cp ${DNSMASQ_CFG} ${TMP_DNSMASQ_CFG}
envsubst < ${TMP_DNSMASQ_CFG} | tee ${DNSMASQ_CFG}

export INSPECTOR_IPXE=/var/lib/ironic/inspector.ipxe
# OSPCIX-870: Copy the config to tempdir to avoid race condition where pipe to
#             tee may wipe the file.
export TMP_INSPECTOR_IPXE=/var/tmp/inspector.ipxe
cp ${INSPECTOR_IPXE} ${TMP_INSPECTOR_IPXE}
envsubst < ${TMP_INSPECTOR_IPXE} | tee ${INSPECTOR_IPXE}

# run common pxe-init script
/usr/local/bin/container-scripts/pxe-init.sh
