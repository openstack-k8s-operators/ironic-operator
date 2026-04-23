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

# Copy pre-populated boot assets from container image if they exist
if [ -d "/usr/share/ironic-operator/var-lib-ironic" ]; then
    cp -a /usr/share/ironic-operator/var-lib-ironic/. /var/lib/ironic/
fi

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
    if [ ! -e "/var/lib/ironic/$dir/snponly.efi" ]; then
        cp /usr/share/ipxe/ipxe-snponly-x86_64.efi /var/lib/ironic/$dir/snponly.efi
    fi
    if [ ! -e "/var/lib/ironic/$dir/undionly.kpxe" ]; then
        cp /usr/share/ipxe/undionly.kpxe /var/lib/ironic/$dir/undionly.kpxe
    fi
    # ipxe.lkrn is not packaged in RHEL 10
    if [ -f "/usr/share/ipxe/ipxe.lkrn" ] && [ ! -e "/var/lib/ironic/$dir/ipxe.lkrn" ]; then
        cp /usr/share/ipxe/ipxe.lkrn /var/lib/ironic/$dir/ipxe.lkrn
    fi
    if [ ! -e "/var/lib/ironic/$dir/bootx64.efi" ]; then
        cp /boot/efi/EFI/$efi_dir/shimx64.efi /var/lib/ironic/$dir/bootx64.efi
    fi
    if [ ! -e "/var/lib/ironic/$dir/grubx64.efi" ]; then
        cp /boot/efi/EFI/$efi_dir/grubx64.efi /var/lib/ironic/$dir/grubx64.efi
    fi
done

# Ensure all boot assets are readable
chmod -R +r /var/lib/ironic/httpboot /var/lib/ironic/tftpboot

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
    popd
    rm -rf /initramfs
fi

# TODO: Remove the block below after confirming that the ESP image is prebuilt in the ironic-pxe container
# Build an ESP image at runtime
pushd /var/lib/ironic/httpboot
if [ ! -a "esp.img" ]; then
    if ! command -v dd || ! command -v mkfs.msdos || ! command -v mmd; then
        echo "WARNING: esp.img will not be created because dd/mkfs.msdos/mmd are missing. Please patch the OpenstackVersion to update container images."
    else
        dd if=/dev/zero of=esp.img bs=4096 count=2048
        mkfs.msdos -F 12 -n 'ESP_IMAGE' esp.img

        mmd -i esp.img EFI
        mmd -i esp.img EFI/BOOT
        mcopy -i esp.img -v bootx64.efi ::EFI/BOOT
        mcopy -i esp.img -v grubx64.efi ::EFI/BOOT
        mdir -i esp.img ::EFI/BOOT;
    fi
fi
popd

# Configure HTTP deployment URL for boot assets (for conductor only)
# Skip this section if called from inspector-pxe-init.sh (DeployHTTPURL not set for inspector)
if [ -n "${DeployHTTPURL}" ]; then
    # Calculate provision network IP if ProvisionNetwork is set
    if [ -n "${ProvisionNetwork}" ]; then
        export ProvisionNetworkIP=$(/usr/local/bin/container-scripts/get_net_ip ${ProvisionNetwork})
    fi

    # Configure HTTP deployment URL for boot assets
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

    INIT_CONFIG="/var/lib/config-data/merged/03-init-container-conductor.conf"

    crudini --set ${INIT_CONFIG} deploy http_url ${DEPLOY_HTTP_URL}
    crudini --set ${INIT_CONFIG} conductor bootloader ${DEPLOY_HTTP_URL}esp.img
    crudini --set ${INIT_CONFIG} conductor deploy_kernel ${DEPLOY_HTTP_URL}ironic-python-agent.kernel
    crudini --set ${INIT_CONFIG} conductor deploy_ramdisk ${DEPLOY_HTTP_URL}ironic-python-agent.initramfs
    crudini --set ${INIT_CONFIG} conductor rescue_kernel ${DEPLOY_HTTP_URL}ironic-python-agent.kernel
    crudini --set ${INIT_CONFIG} conductor rescue_ramdisk ${DEPLOY_HTTP_URL}ironic-python-agent.initramfs
fi

# Validate boot file existence
echo "Validating boot file existence..."

# Check legacy (root-level) boot files
HTTPBOOT_DIR="/var/lib/ironic/httpboot"
MISSING_FILES=()

for file in esp.img bootx64.efi undionly.kpxe snponly.efi; do
    if [ ! -f "${HTTPBOOT_DIR}/${file}" ]; then
        MISSING_FILES+=("${HTTPBOOT_DIR}/${file}")
    fi
done

# Check x86_64 architecture-specific files if directory exists
if [ -d "${HTTPBOOT_DIR}/x86_64" ]; then
    echo "Detected x86_64 boot assets directory"
    for file in esp.img bootx64.efi undionly.kpxe snponly.efi; do
        if [ ! -f "${HTTPBOOT_DIR}/x86_64/${file}" ]; then
            MISSING_FILES+=("${HTTPBOOT_DIR}/x86_64/${file}")
        fi
    done
fi

# Check aarch64 architecture-specific files if directory exists
if [ -d "${HTTPBOOT_DIR}/aarch64" ]; then
    echo "Detected aarch64 boot assets directory"
    for file in esp.img bootaa64.efi snponly.efi; do
        if [ ! -f "${HTTPBOOT_DIR}/aarch64/${file}" ]; then
            MISSING_FILES+=("${HTTPBOOT_DIR}/aarch64/${file}")
        fi
    done
fi

# Fail fast if any required files are missing
if [ ${#MISSING_FILES[@]} -gt 0 ]; then
    echo "ERROR: Missing required boot files:"
    for missing in "${MISSING_FILES[@]}"; do
        echo "  - ${missing}"
    done
    echo "Boot asset initialization failed. Please ensure the ironic-pxe container image contains all required files."
    exit 1
fi

echo "All required boot files validated successfully"
