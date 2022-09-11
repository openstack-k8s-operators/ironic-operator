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

# Ensure localhost.crt is generated for mod_ssl
/usr/local/bin/kolla_httpd_setup

# TODO(sbaker): remove when https://review.opendev.org/c/openstack/tripleo-common/+/854459 is in the image
mkdir -p /var/www/cgi-bin/ironic
cp -a /usr/bin/ironic-api-wsgi /var/www/cgi-bin/ironic/app