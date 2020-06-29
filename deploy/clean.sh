#!/bin/bash

oc delete secret heat-db-password
oc delete secret heat-service-password
oc delete secret heat-stack-domain-admin-password

oc delete configmaps heat

oc delete jobs heat-db-sync

oc delete deployment heat-api
oc delete deployment heat-engine

oc delete service heat-api
oc delete route heat

oc delete heat heat
oc delete crd heats.heat.openstack.org
