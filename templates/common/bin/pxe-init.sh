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


# Create TFTP, HTTP serving directories
if [ ! -d "/var/lib/ironic/tftpboot/pxelinux.cfg" ]; then
    mkdir -p /var/lib/ironic/tftpboot/pxelinux.cfg
fi
if [ ! -d "/var/lib/ironic/httpboot" ]; then
    mkdir -p /var/lib/ironic/httpboot
fi
# Check for expected EFI directories
if [ -d "/boot/efi/EFI/centos" ]; then
    efi_dir=centos
elif [ -d "/boot/efi/EFI/redhat" ]; then
    efi_dir=redhat
else
    echo "No EFI directory detected"
    exit 1
fi

# Copy iPXE and grub files to tftpboot, httpboot
for dir in httpboot tftpboot; do
    cp /usr/share/ipxe/ipxe-snponly-x86_64.efi /var/lib/ironic/$dir/snponly.efi
    cp /usr/share/ipxe/undionly.kpxe           /var/lib/ironic/$dir/undionly.kpxe
    cp /usr/share/ipxe/ipxe.lkrn               /var/lib/ironic/$dir/ipxe.lkrn
    cp /boot/efi/EFI/$efi_dir/shimx64.efi      /var/lib/ironic/$dir/bootx64.efi
    cp /boot/efi/EFI/$efi_dir/grubx64.efi      /var/lib/ironic/$dir/grubx64.efi
    # Ensure all files are readable
    chmod -R +r /var/lib/ironic/$dir
done

# Patch ironic-python-agent with custom CA certificates
if [ -f "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem" ] && [ -f "/var/lib/ironic/httpboot/ironic-python-agent.initramfs" ]; then
    # Extract the initramfs
    cd /
    mkdir initramfs
    pushd initramfs
    zcat /var/lib/ironic/httpboot/ironic-python-agent.initramfs | cpio -idmV
    popd

    # Copy the CA certificates
    cp /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem /initramfs/etc/pki/ca-trust/source/anchors/
    echo update-ca-trust | unshare -r chroot ./initramfs

    # Repack the initramfs
    pushd initramfs
    find . | cpio -o -c --quiet -R root:root | gzip -1 > /var/lib/ironic/httpboot/ironic-python-agent.initramfs
fi

# Build an ESP image
pushd /var/lib/ironic/httpboot
if ! command -v dd || ! command -v mkfs.msdos || ! command -v mmd; then
    echo "WARNING: esp.img will not be created because dd/mkfs.msdos/mmd are missing. Please patch the OpenstackVersion to update container images."
elif [ ! -a "esp.img" ]; then
    dd if=/dev/zero of=esp.img bs=4096 count=1024
    mkfs.msdos -F 12 -n 'ESP_IMAGE' esp.img

    mmd -i esp.img EFI
    mmd -i esp.img EFI/BOOT
    mcopy -i esp.img -v bootx64.efi ::EFI/BOOT
    mcopy -i esp.img -v grubx64.efi ::EFI/BOOT
    mdir -i esp.img ::EFI/BOOT;
fi
popd
