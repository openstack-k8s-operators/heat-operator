# heat-operator

The Heat Operator deploys the OpenStack Heat project in a OpenShift cluster.

## Description

This project should be used to deploy the OpenStack Heat project. It expects that there is an existing Database and Keystone service available to connect to.

## Getting Started

This operator is deployed via Operator Lifecycle Manager as part of the OpenStack Operator bundle:
https://github.com/openstack-k8s-operators/openstack-operator

To configure Heat specifically, we expose options to add custom configuration and the number of replicas for both Heat API and Heat Engine:

The following is taken from the Sample config in this repo:

```yaml
spec:
  customServiceConfig: "# add your customization here"
  databaseInstance: openstack
  databaseUser: "heat"
  rabbitMqClusterName: rabbitmq
  debug:
    dbSync: false
    service: false
  heatAPI:
    containerImage: "quay.io/podified-antelope-centos9/openstack-heat-api:current-podified"
    customServiceConfig: "# add your customization here"
    databaseUser: "heat"
    debug:
      dbSync: false
      service: false
    passwordSelectors:
      service: AdminPassword
      database: AdminPassword
    replicas: 1
    resources: {}
    secret: "osp-secret"
    serviceUser: ""
  heatEngine:
    containerImage: "quay.io/podified-antelope-centos9/openstack-heat-engine:current-podified"
    customServiceConfig: "# add your customization here"
    databaseUser: "heat"
    debug:
      dbSync: false
      service: false
    passwordSelectors:
      service: AdminPassword
      database: AdminPassword
    replicas: 1
    resources: {}
    secret: "osp-secret"
    serviceUser: ""
  passwordSelectors:
    service: AdminPassword
    database: AdminPassword
  preserveJobs: false
  secret: osp-secret
  serviceUser: "heat"
```

If we want to make some changes to the Heat configuration, for example `num_engine_workers`.
We would modify the resource to look like this:

```yaml
spec:
  customServiceConfig: |
    [DEFAULT]
    num_engine_workers=4
  databaseInstance: openstack
  databaseUser: heat
  debug:
    dbSync: false
    service: false
```

In this example, we are setting `num_engine_workers` to 4. After this resource has been updated, the controller will
regenerate the ConfigMap to include the updated values in the `custom.conf` file.

```sh
❯ oc get cm heat-config-data -o jsonpath={.data} | jq '."custom.conf"' | sed 's/\\n/\n/g'
"[DEFAULT]
num_engine_workers=4
"
```

We can see this change reflected in the `/etc/heat/heat.conf.d/custom.conf` file within each of the API and Engine pods:

```sh
❯ oc get po -l service=heat
NAME                           READY   STATUS    RESTARTS   AGE
heat-api-5bd49b9c6d-6cprh      1/1     Running   0          2m51s
heat-engine-5565547478-v2n4j   1/1     Running   0          2m51s

❯ oc exec -it heat-engine-5565547478-v2n4j -c heat-engine -- cat /etc/heat/heat.conf.d/custom.conf
[DEFAULT]
num_engine_workers=4
```

### Running on the cluster

To enable the Heat service, we simply need to set the Heat service to enabled in the `OpenStackControlPlane`.
The following snippet is taken from the `OpenStackControlPlane` Custom Resource:

```yaml
heat:
  enabled: true # <<-- Enable the Heat service by setting `enabled: true`
  template:
    customServiceConfig: "# add your customization here"
    databaseInstance: openstack
    databaseUser: "heat"
    rabbitMqClusterName: rabbitmq
    debug:
      dbSync: false
      service: false
    heatAPI:
      containerImage: "quay.io/tripleozedcentos9/openstack-heat-api:current-tripleo"
      customServiceConfig: "# add your customization here"
      databaseUser: "heat"
      debug:
        dbSync: false
        service: false
      passwordSelectors:
        service: AdminPassword
        database: AdminPassword
      replicas: 1
      resources: {}
      secret: "osp-secret"
      serviceUser: ""
    heatEngine:
      containerImage: "quay.io/tripleozedcentos9/openstack-heat-engine:current-tripleo"
      customServiceConfig: "# add your customization here"
      databaseUser: "heat"
      debug:
        dbSync: false
        service: false
      passwordSelectors:
        service: AdminPassword
        database: AdminPassword
      replicas: 1
      resources: {}
      secret: "osp-secret"
      serviceUser: ""
    passwordSelectors:
      service: AdminPassword
      database: AdminPassword
    preserveJobs: false
    secret: osp-secret
    serviceUser: "heat"
```

As we can see, the Heat definition remains consistent within the `OpenStackControlPlane` resource, so the same
logic applies here when we want to customize the service. Once again, configuring `num_engine_workers` in
this example, it would like look this:

```yaml
heat:
  enabled: true
  template:
    customServiceConfig: |
      [DEFAULT]
      num_engine_workers=4
    databaseInstance: openstack
    databaseUser: "heat"
    rabbitMqClusterName: rabbitmq
```

The `AdminPassword` defined in the examples here are user defined and contained within the provided secret:

```yaml
[...]
    passwordSelectors:
      service: AdminPassword
      database: AdminPassword
    [...]
    secret: osp-secret
```

The `osp-secret` contains the base64 encoded password string:

```sh
❯ oc get secret osp-secret -o jsonpath={.data.AdminPassword}
MTIzNDU2Nzg=

❯ oc get secret osp-secret -o jsonpath={.data.AdminPassword} | base64 -d
12345678
```

### Undeploy controller

To undeploy the operator, simply set the `enabled` value to false from within the `OpenStackControlPlane` resource.

## Contributing

The following guide relies on a already deployed `OpenStackControlPlane`. If you don't already have this, you can
follow the guides located on the following repo:
https://github.com/openstack-k8s-operators/install_yamls

To contribute, you can disabled the Heat service in the `OpenStackControlPlane` resouce and run the Operator locally
from your laptop using `make install run`. This will start the operator locally, and debugging can be done without
building and pushing a new container.

Once you have tested your feature, you can use `pre-commit run --all-files` to ensure the change passes preliminary
testing.

Ensure you have created an issue against the project and send a PR linking to the relevant issue. You can reach out
to the maintainers from the included OWNERS file for change reviews.

### How it works

This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/)
which provides a reconcile function responsible for synchronizing resources untile the desired state is reached on the cluster

### Modifying the API definitions

If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
