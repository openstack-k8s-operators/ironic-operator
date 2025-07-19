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

/usr/local/bin/kolla_set_configs

# prepare for 'upgrade check' loading all drivers
if [ ! -d "/var/lib/ironic/tmp" ]; then
    mkdir /var/lib/ironic/tmp
fi
if [ ! -d "/var/lib/ironic/httpboot" ]; then
    mkdir -p /var/lib/ironic/httpboot
fi

ironic-status upgrade check && ret_val=$? || ret_val=$?
if [ $ret_val -gt 1 ] ; then
    # NOTE(TheJulia): We need to evaluate the return code from the
    # upgrade status check as the framework defines
    # Warnings are permissible and returned as status code 1, errors are
    # returned as greater than 1 which means there is a major upgrade
    # stopping issue which needs to be addressed.
    echo "WARNING: Status check failed, we're going to attempt to apply the schema update and then re-evaluate."
    ironic-dbsync --config-file /etc/ironic/ironic.conf --config-dir /etc/ironic/ironic.conf.d/ upgrade
    ironic-status upgrade check && ret_val=$? || ret_val=$?
    if [ $ret_val -gt 1 ] ; then
        echo $LINENO "Ironic DB Status check failed, returned: $ret_val"
        exit $ret_val
    fi
fi
ironic-dbsync --config-file /etc/ironic/ironic.conf --config-dir /etc/ironic/ironic.conf.d/

ironic-dbsync --config-file /etc/ironic/ironic.conf --config-dir /etc/ironic/ironic.conf.d/ online_data_migrations
