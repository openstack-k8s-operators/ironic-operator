#!/bin/bash
#
# Copyright 2020 Red Hat Inc.
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

echo "Starting conductor init"

# Get the statefulset pod index
export PODINDEX=$(echo ${HOSTNAME##*-})

# Get required environment variables
IRONICPASSWORD=${IronicPassword:-""}
TRANSPORTURL=${TransportURL:-""}
QUORUMQUEUES=${QuorumQueues:-"false"}

INIT_CONFIG="/var/lib/config-data/merged/03-init-container-conductor.conf"

if [ -n "${ProvisionNetwork}" ]; then
    export ProvisionNetworkIP=$(/usr/local/bin/container-scripts/get_net_ip ${ProvisionNetwork})
    crudini --set ${INIT_CONFIG} DEFAULT my_ip $ProvisionNetworkIP
fi


export DEPLOY_HTTP_URL=$(python3 -c '
import os
import ipaddress

# Get the IP address
ip_str = os.environ.get("ProvisionNetworkIP", "")

# Check if it is an IPv6 address and format accordingly
try:
    ip = ipaddress.ip_address(ip_str)
    if isinstance(ip, ipaddress.IPv6Address):
        # For IPv6, we need to wrap the IP in brackets for URL formatting
        formatted_env = dict(os.environ)
        formatted_env["ProvisionNetworkIP"] = f"[{ip_str}]"
        print(os.environ["DeployHTTPURL"] % formatted_env)
    else:
        # For IPv4, use as-is
        print(os.environ["DeployHTTPURL"] % os.environ)
except ValueError:
    # If IP parsing fails, use as-is (fallback)
    print(os.environ["DeployHTTPURL"] % os.environ)
')

crudini --set ${INIT_CONFIG} deploy http_url ${DEPLOY_HTTP_URL}
crudini --set ${INIT_CONFIG} conductor bootloader ${DEPLOY_HTTP_URL}esp.img
crudini --set ${INIT_CONFIG} conductor deploy_kernel ${DEPLOY_HTTP_URL}ironic-python-agent.kernel
crudini --set ${INIT_CONFIG} conductor deploy_ramdisk ${DEPLOY_HTTP_URL}ironic-python-agent.initramfs
crudini --set ${INIT_CONFIG} conductor rescue_kernel ${DEPLOY_HTTP_URL}ironic-python-agent.kernel
crudini --set ${INIT_CONFIG} conductor rescue_ramdisk ${DEPLOY_HTTP_URL}ironic-python-agent.initramfs


# Copy required config to modifiable location
cp /var/lib/config-data/default/dnsmasq.conf /var/lib/ironic/

export DNSMASQ_CFG=/var/lib/ironic/dnsmasq.conf
sed -e "/BLOCK_PODINDEX_${PODINDEX}_BEGIN/,/BLOCK_PODINDEX_${PODINDEX}_END/p" \
    -e "/BLOCK_PODINDEX_.*_BEGIN/,/BLOCK_PODINDEX_.*_END/d" \
    -i ${DNSMASQ_CFG}
sed -e "/BLOCK_PODINDEX_${PODINDEX}_BEGIN/d" \
    -e "/BLOCK_PODINDEX_${PODINDEX}_END/d" \
    -i ${DNSMASQ_CFG}

if [ ! -d "/var/lib/ironic/tmp" ]; then
    mkdir /var/lib/ironic/tmp
fi
if [ ! -d "/var/lib/ironic/httpboot" ]; then
    mkdir /var/lib/ironic/httpboot
fi
if [ ! -d "/var/lib/ironic/ramdisk-logs" ]; then
    mkdir /var/lib/ironic/ramdisk-logs
fi

NOVNC_PROXY_URL=$(python3 -c '
import os

url_template = os.environ.get("NoVNCProxyURL", "")
if url_template:
    print(url_template % os.environ)
')
crudini --set ${INIT_CONFIG} vnc public_url ${NOVNC_PROXY_URL}

echo "Conductor init successfully completed"
