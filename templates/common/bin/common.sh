#!/bin/bash
#
# Copyright 2022 Red Hat Inc.
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

set -e

function merge_config_dir {
    echo merge config dir $1
    for conf in $(find $1 -type f); do
        conf_base=$(basename $conf)

        # If CFG already exist in ../merged and is not a json or ipxe file,
        # we expect for now it can be merged using crudini.
        # Else, just copy the full file.
        if [[ -f /var/lib/config-data/merged/${conf_base} && ${conf_base} != *.json && ${conf_base} != *.ipxe ]]; then
            echo merging ${conf} into /var/lib/config-data/merged/${conf_base}
            crudini --merge /var/lib/config-data/merged/${conf_base} < ${conf}
        else
            echo copy ${conf} to /var/lib/config-data/merged/
            cp -f ${conf} /var/lib/config-data/merged/
        fi
    done
}

function common_ironic_config {
    # Secrets are obtained from ENV variables.
    export IRONICPASSWORD=${IronicPassword:?"Please specify a IronicPassword variable."}
    export TRANSPORTURL=${TransportURL:-""}
    # TODO: nova password
    #export NOVAPASSWORD=${NovaPassword:?"Please specify a NovaPassword variable."}

    export CUSTOMCONF=${CustomConf:-""}

    SVC_CFG=/etc/ironic/ironic.conf
    SVC_CFG_MERGED=/var/lib/config-data/merged/ironic.conf

    # Copy default service config from container image as base
    cp -a ${SVC_CFG} ${SVC_CFG_MERGED}

    # Merge all templates from config-data defaults first, then custom
    # NOTE: custom.conf files (for both the umbrella Ironic CR in config-data/defaults
    #       and each custom.conf for each sub-service in config-data/custom) still need
    #       to be handled separately below because the "merge_config_dir" function will
    #       not merge custom.conf into ironic.conf (because the files obviously have
    #       different names)
    for dir in /var/lib/config-data/default /var/lib/config-data/custom; do
        merge_config_dir ${dir}
    done

    # set secrets
    # Only set rpc_transport and transport_url if $TRANSPORTURL
    if [ -n "$TRANSPORTURL" ]; then
        crudini --set ${SVC_CFG_MERGED} DEFAULT transport_url $TRANSPORTURL
        crudini --set ${SVC_CFG_MERGED} DEFAULT rpc_transport oslo
    fi
    crudini --set ${SVC_CFG_MERGED} keystone_authtoken password $IRONICPASSWORD
    crudini --set ${SVC_CFG_MERGED} service_catalog password $IRONICPASSWORD
    crudini --set ${SVC_CFG_MERGED} cinder password $IRONICPASSWORD
    crudini --set ${SVC_CFG_MERGED} glance password $IRONICPASSWORD
    crudini --set ${SVC_CFG_MERGED} neutron password $IRONICPASSWORD
    crudini --set ${SVC_CFG_MERGED} nova password $IRONICPASSWORD
    crudini --set ${SVC_CFG_MERGED} swift password $IRONICPASSWORD
    crudini --set ${SVC_CFG_MERGED} inspector password $IRONICPASSWORD
}
