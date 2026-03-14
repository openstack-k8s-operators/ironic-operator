# sno-1-bm Scenario

## Overview

A Single Node OpenShift (SNO) scenario designed to test OpenStack Ironic bare
metal provisioning with 1 dedicated Ironic node using iPXE network boot. This
scenario validates the complete OpenStack bare metal lifecycle including node
enrollment, provisioning, and Tempest testing.

## Architecture

<!-- markdownlint-disable MD013 -->
```mermaid
graph TD
    Internet[("Internet")]
    Router{{"Neutron<br/>Router"}}

    MachineNet["Machine Network<br/>192.168.32.0/24"]
    CtlPlane["CtlPlane Network<br/>192.168.122.0/24"]
    VLANNets["VLAN Trunk Networks<br/>Internal API: 172.17.0.0/24<br/>Storage: 172.18.0.0/24<br/>Tenant: 172.19.0.0/24"]
    IronicNet["Ironic Network<br/>172.20.1.0/24"]

    Controller["Controller<br/>192.168.32.254<br/>DNS/HAProxy"]
    Master["SNO Master<br/>192.168.32.10"]
    IronicNodes["Ironic Node x1<br/>Virtual Baremetal"]

    LVM["TopoLVM<br/>20GB"]
    CinderVols["Cinder Volumes x3<br/>20GB each"]

    Internet --- Router

    Router --- MachineNet
    Router --- CtlPlane
    Router --- VLANNets
    Router --- IronicNet

    MachineNet --- Controller
    MachineNet --- Master
    CtlPlane --- Master
    VLANNets --- Master
    IronicNet --- Master
    IronicNet --- IronicNodes

    Master --- LVM
    Master --- CinderVols

    style Controller fill:#4A90E2,stroke:#2E5C8A,stroke-width:3px,color:#fff
    style Master fill:#F5A623,stroke:#C87D0E,stroke-width:3px,color:#fff
    style IronicNodes fill:#9B59B6,stroke:#6C3A82,stroke-width:2px,color:#fff
    style Router fill:#27AE60,stroke:#1E8449,stroke-width:3px,color:#fff
```
<!-- markdownlint-enable MD013 -->

### Component Details

- **Controller**: Hotstack controller providing DNS, load balancing, and
  orchestration services
- **SNO Master**: Single-node OpenShift cluster running the complete OpenStack
  control plane
- **Ironic Node**: 1 virtual bare metal node for testing Ironic provisioning workflows

## Networks

- **machine-net**: 192.168.32.0/24 (OpenShift cluster network)
- **ctlplane-net**: 192.168.122.0/24 (OpenStack control plane)
- **internal-api-net**: 172.17.0.0/24 (OpenStack internal services)
- **storage-net**: 172.18.0.0/24 (Storage backend communication)
- **tenant-net**: 172.19.0.0/24 (Tenant network traffic)
- **ironic-net**: 172.20.1.0/24 (Bare metal provisioning network)

## OpenStack Services

This scenario deploys a comprehensive OpenStack environment:

### Core Services

- **Keystone**: Identity service with LoadBalancer on Internal API
- **Nova**: Compute service with Ironic driver for bare metal
- **Neutron**: Networking service with OVN backend
- **Glance**: Image service with Swift backend
- **Swift**: Object storage service
- **Placement**: Resource placement service

### Bare Metal Services

- **Ironic**: Bare metal provisioning service
- **Ironic Inspector**: Hardware inspection service
- **Ironic Neutron Agent**: Network management for bare metal

## Usage

```bash
# Deploy the scenario
ansible-playbook -i inventory.yml bootstrap.yml \
  -e @scenarios/sno-1-bm/bootstrap_vars.yml \
  -e @~/cloud-secrets.yaml

# Run comprehensive tests
ansible-playbook -i inventory.yml 06-test-operator.yml \
  -e @scenarios/sno-1-bm/bootstrap_vars.yml \
  -e @~/cloud-secrets.yaml
```

## Configuration Files

- `bootstrap_vars.yml`: Infrastructure and OpenShift configuration.
- `automation-vars.yml`: Hotloop deployment stages
- `heat_template_ipxe.yaml`: OpenStack infrastructure template (iPXE network boot)
- `manifests/control-plane/control-plane.yaml`: OpenStack service configuration
- `test-operator/automation-vars.yml`: Comprehensive test automation
- `test-operator/tempest-tests.yml`: Tempest test specifications
