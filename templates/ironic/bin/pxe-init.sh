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

# Create HTTP serving directories
mkdir -p /var/lib/ironic/httpboot/tftpboot/pxelinux.cfg

# Check for expected EFI directories
if [ -d "/boot/efi/EFI/centos" ]; then
    efi_dir=centos
elif [ -d "/boot/efi/EFI/rhel" ]; then
    efi_dir=rhel
else
    echo "No EFI directory detected"
    exit 1
fi

# Copy iPXE and grub files to tftpboot, httpboot
for dir in httpboot httpboot/tftpboot; do
    cp /usr/share/ipxe/ipxe-snponly-x86_64.efi /var/lib/ironic/$dir/snponly.efi
    cp /usr/share/ipxe/undionly.kpxe           /var/lib/ironic/$dir/undionly.kpxe
    cp /usr/share/ipxe/ipxe.lkrn               /var/lib/ironic/$dir/ipxe.lkrn
    cp /boot/efi/EFI/$efi_dir/shimx64.efi      /var/lib/ironic/$dir/bootx64.efi
    cp /boot/efi/EFI/$efi_dir/grubx64.efi      /var/lib/ironic/$dir/grubx64.efi
done