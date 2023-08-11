# ironic-operator
A Kubernetes Operator built using the [Operator Framework](https://github.com/operator-framework) for Go. The Operator provides a way to easily install and manage an OpenStack Ironic installation
on Kubernetes. This Operator was developed using [RDO](https://www.rdoproject.org/) containers for openStack.

# Deployment

The operator is intended to be deployed via OLM [Operator Lifecycle Manager](https://github.com/operator-framework/operator-lifecycle-manager)

# Example

The Operator creates a custom Ironic resource that can be used to create Ironic
API and conductor instances within the cluster. Example CR to create an Ironic
in your cluster:

```yaml
apiVersion: ironic.openstack.org/v1beta1
kind: Ironic
metadata:
  name: ironic
  namespace: openstack
spec:
  serviceUser: ironic
  customServiceConfig: |
    [DEFAULT]
    debug = true
  databaseInstance: openstack
  ironicAPI:
    replicas: 1
    containerImage: quay.io/podified-antelope-centos9/openstack-ironic-api:current-podified
  ironicConductors:
  - replicas: 1
    containerImage: quay.io/podified-antelope-centos9/openstack-ironic-conductor:current-podified
    pxeContainerImage: quay.io/podified-antelope-centos9/openstack-ironic-pxe:current-podified
    provisionNetwork: provision-net
  secret: ironic-secret
```

This example assumes the existence of additional network `provision_net` to use
for exposing provision boot services (DHCP, TFTP, HTTP). Currently this
additional network needs to be managed manually with
`NetworkAttachmentDefinition` resources. For example, applying the following
will result in network interface `eno1` being available in the conductor pod:

```yaml
apiVersion: k8s.cni.cncf.io/v1
kind: NetworkAttachmentDefinition
metadata:
  name: provision-net
spec:
  config: |-
    {
      "cniVersion": "0.3.1",
      "name": "provision-net",
      "type": "host-device",
      "device": "eno1"
    }
```

## Example: expose IronicAPI or IronicInspector to an isolated network

The Ironic spec can be used to register e.g. the internal endpoint to
an isolated network. MetalLB is used for this scenario.

As a pre requisite, MetalLB needs to be installed and worker nodes
prepared to work as MetalLB nodes to serve the LoadBalancer service.

In this example the following MetalLB IPAddressPool is used:

```
---
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: osp-internalapi
  namespace: metallb-system
spec:
  addresses:
  - 172.17.0.200-172.17.0.210
  autoAssign: false
```

The following represents an example of Ironic resource that can be used
to trigger the service deployment, and have the internal ironicAPI endpoint
registerd as a MetalLB service using the IPAddressPool `osp-internal`,
request to use the IP `172.17.0.202` as the VIP and the IP is shared with
other services.

```
apiVersion: ironic.openstack.org/v1beta1
kind: Ironic
metadata:
  name: ironic
spec:
  ...
  ironicAPI:
    ...
    override:
      service:
        metadata:
          annotations:
            metallb.universe.tf/address-pool: internalapi
            metallb.universe.tf/allow-shared-ip: internalapi
            metallb.universe.tf/loadBalancerIPs: 172.17.0.202
        spec:
          type: LoadBalancer
    ...
...
```

The internal ironicAPI endpoint gets registered with its service name. This
service name needs to resolve to the `LoadBalancerIP` on the isolated network
either by DNS or via /etc/hosts:

```
# openstack endpoint list -c 'Service Name' -c Interface -c URL --service ironic
+--------------+-----------+-------------------------------------------------+
| Service Name | Interface | URL                                             |
+--------------+-----------+-------------------------------------------------+
| ironic       | internal  | http://ironic-internal.openstack.svc:6385       |
| ironic       | public    | http://ironic-public-openstack.apps-crc.testing |
+--------------+-----------+-------------------------------------------------+
```

# Design
The current design takes care of the following:

- Creates ironic config files via config maps
- Creates an ironic deployment with the specified replicas
- Creates an ironic service
- Ironic bootstrap, and db sync are executed automatically on install and updates
- ConfigMap is recreated on any changes Ironic object changes and the Deployment updated.
