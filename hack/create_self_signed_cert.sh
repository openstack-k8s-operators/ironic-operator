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

TMPDIR=${TMPDIR:-"/tmp/k8s-webhook-server/serving-certs"}
SERVICE=${SERVICE:-"glance"}
NAMESPACE=${NAMESPACE:-"openstack"}

mkdir -p ${TMPDIR}

cat <<EOF >> ${TMPDIR}/tls.conf
[req]
default_bits = 2048
prompt = no
default_md = sha256
req_extensions = req_ext
distinguished_name = dn
[dn]
O = system:nodes
CN = system:node:${SERVICE}.${NAMESPACE}.pod.cluster.local
[req_ext]
subjectAltName = @alt_names
[alt_names]
DNS.1 = ${SERVICE}.${NAMESPACE}.svc
DNS.2 = ${SERVICE}.${NAMESPACE}.svc.cluster.local
DNS.2 = ${SERVICE}.${NAMESPACE}.pod.cluster.local
EOF

 openssl req -newkey rsa:4096 -days 3650 -nodes -x509 \
  -keyout ${TMPDIR}/tls.key \
  -out ${TMPDIR}/tls.crt -config ${TMPDIR}/tls.conf