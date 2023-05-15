#!/bin/bash
set -ex

oc delete validatingwebhookconfiguration/vironic.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mironic.kb.io --ignore-not-found
