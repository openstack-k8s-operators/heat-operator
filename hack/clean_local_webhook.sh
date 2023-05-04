#!/bin/bash
set -ex

oc delete validatingwebhookconfiguration/vheat.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mheat.kb.io --ignore-not-found
