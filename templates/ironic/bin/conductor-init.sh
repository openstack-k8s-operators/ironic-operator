#!/bin//bash
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

export ProvisionNetworkIP=$(/usr/local/bin/container-scripts/provision-network-ip.py)
if [ -n "$ProvisionNetworkIP" ]; then
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

if [ ! -d "/var/lib/ironic/httpboot" ]; then
    mkdir /var/lib/ironic/httpboot
fi
# Build an ESP image
pushd /var/lib/ironic/httpboot
if [ ! -a "esp.img" ]; then
    dd if=/dev/zero of=/tmp/esp.img bs=4096 count=1024
    mkfs.fat -s 4 -r 512 -S 4096 /tmp/esp.img

    ESP_IMAGE_DIR=$(mktemp -t -d esp.XXXXXXXX)
    mount /tmp/esp.img $ESP_IMAGE_DIR
    mkdir -p $ESP_IMAGE_DIR/EFI/BOOT
    cp bootx64.efi $ESP_IMAGE_DIR/EFI/BOOT/BOOTX64.efi
    cp grubx64.efi $ESP_IMAGE_DIR/EFI/BOOT/GRUBX64.efi
    umount $ESP_IMAGE_DIR

    cp /tmp/esp.img ./
fi
popd

# Download ironic-python-agent and any other images
/usr/local/bin/container-scripts/imagetter /usr/local/bin/container-scripts/imagetter.yaml
