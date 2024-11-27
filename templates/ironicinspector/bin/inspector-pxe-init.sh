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
export INSPECTOR_HTTP_URL=$(python3 -c 'import os; print(os.environ["InspectorHTTPURL"] % os.environ)')

export DNSMASQ_CFG=/var/lib/config-data/merged/dnsmasq.conf
sed -e "/BLOCK_PODINDEX_${PODINDEX}_BEGIN/,/BLOCK_PODINDEX_${PODINDEX}_END/p" \
    -e "/BLOCK_PODINDEX_.*_BEGIN/,/BLOCK_PODINDEX_.*_END/d" \
    -i ${DNSMASQ_CFG}
sed -e "/BLOCK_PODINDEX_${PODINDEX}_BEGIN/d" \
    -e "/BLOCK_PODINDEX_${PODINDEX}_END/d" \
    -i ${DNSMASQ_CFG}
envsubst < ${DNSMASQ_CFG} | tee ${DNSMASQ_CFG}

export INSPECTOR_IPXE=/var/lib/config-data/merged/inspector.ipxe
envsubst < ${INSPECTOR_IPXE} | tee ${INSPECTOR_IPXE}

# run common pxe-init script
/usr/local/bin/container-scripts/pxe-init.sh
