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
  databaseUser: ironic
  ironicAPI:
    replicas: 1
    containerImage: quay.io/tripleomastercentos9/openstack-ironic-api:current-tripleo
  ironicConductor:
    replicas: 1
    containerImage: quay.io/tripleomastercentos9/openstack-ironic-conductor:current-tripleo
    pxeContainerImage: quay.io/tripleomastercentos9/openstack-ironic-pxe:current-tripleo
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

# Design
The current design takes care of the following:

- Creates ironic config files via config maps
- Creates an ironic deployment with the specified replicas
- Creates an ironic service
- Ironic bootstrap, and db sync are executed automatically on install and updates
- ConfigMap is recreated on any changes Ironic object changes and the Deployment updated.
