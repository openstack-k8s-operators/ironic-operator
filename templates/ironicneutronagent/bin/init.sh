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

export IRONICPASSWORD=${IronicPassword:?"Please specify a IronicPassword variable."}
export TRANSPORTURL=${TransportURL:?"Please specify a TransportURL variable."}

SVC_CFG_MERGED=/var/lib/config-data/merged/ironic_neutron_agent.ini

# expect that the common.sh is in the same dir as the calling script
SCRIPTPATH="$( cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
. ${SCRIPTPATH}/common.sh --source-only

# Merge all templates from config-data defaults first, then custom
for dir in /var/lib/config-data/default /var/lib/config-data/custom; do
    merge_config_dir ${dir}
done

# set secrets
crudini --set ${SVC_CFG_MERGED} DEFAULT transport_url $TRANSPORTURL
crudini --set ${SVC_CFG_MERGED} keystone_authtoken password $IRONICPASSWORD
crudini --set ${SVC_CFG_MERGED} service_catalog password $IRONICPASSWORD
crudini --set ${SVC_CFG_MERGED} ironic password $IRONICPASSWORD