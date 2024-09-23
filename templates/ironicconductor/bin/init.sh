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

# Get the statefulset pod index
export PODINDEX=$(echo ${HOSTNAME##*-})

# expect that the common.sh is in the same dir as the calling script
SCRIPTPATH="$( cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
. ${SCRIPTPATH}/common.sh --source-only

common_ironic_config

if [ -n "${ProvisionNetwork}" ]; then
    export ProvisionNetworkIP=$(/usr/local/bin/container-scripts/get_net_ip ${ProvisionNetwork})
    crudini --set ${SVC_CFG_MERGED} DEFAULT my_ip $ProvisionNetworkIP
fi
export DEPLOY_HTTP_URL=$(python3 -c 'import os; print(os.environ["DeployHTTPURL"] % os.environ)')
SVC_CFG_MERGED=/var/lib/config-data/merged/ironic.conf
crudini --set ${SVC_CFG_MERGED} deploy http_url ${DEPLOY_HTTP_URL}
crudini --set ${SVC_CFG_MERGED} conductor bootloader ${DEPLOY_HTTP_URL}esp.img
crudini --set ${SVC_CFG_MERGED} conductor deploy_kernel ${DEPLOY_HTTP_URL}ironic-python-agent.kernel
crudini --set ${SVC_CFG_MERGED} conductor deploy_ramdisk ${DEPLOY_HTTP_URL}ironic-python-agent.initramfs
crudini --set ${SVC_CFG_MERGED} conductor rescue_kernel ${DEPLOY_HTTP_URL}ironic-python-agent.kernel
crudini --set ${SVC_CFG_MERGED} conductor rescue_ramdisk ${DEPLOY_HTTP_URL}ironic-python-agent.initramfs

export DNSMASQ_CFG=/var/lib/config-data/merged/dnsmasq.conf
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
