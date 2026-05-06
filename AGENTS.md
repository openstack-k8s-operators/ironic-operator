# AGENTS.md - ironic-operator

## Project overview

ironic-operator is a Kubernetes operator that manages
[OpenStack Ironic](https://docs.openstack.org/ironic/latest/) (the bare metal
provisioning service: hardware enrollment, inspection, cleaning, provisioning,
and PXE/DHCP boot) on OpenShift/Kubernetes. It is part of the
[openstack-k8s-operators](https://github.com/openstack-k8s-operators) project.

Key Ironic domain concepts: **bare metal nodes**, **enrollment**,
**hardware inspection** (ironic-inspector), **cleaning**, **provisioning**,
**conductors** (one per group), **DHCP ranges**, **PXE boot**,
**ironic-neutron-agent** (network provisioning for bare metal),
**standalone mode**, **conductor groups**.
## Tech stack

| Layer | Technology |
|-------|------------|
| Language | Go (modules, multi-module workspace via `go.work`) |
| Scaffolding | [Kubebuilder v4](https://book.kubebuilder.io/) + [Operator SDK](https://sdk.operatorframework.io/) |
| CRD generation | controller-gen (DeepCopy, CRDs, RBAC, webhooks) |
| Config management | Kustomize |
| Packaging | OLM bundle |
| Testing | Ginkgo/Gomega + envtest (functional), KUTTL (integration) |
| Linting | golangci-lint (`.golangci.yaml`) |
| CI | Zuul (`zuul.d/`), Prow (`.ci-operator.yaml`), GitHub Actions |

## Custom Resources

| Kind | Purpose |
|------|---------|
| `Ironic` | Top-level CR. Owns the database, keystone service, transport URL, and spawns sub-CRs for each service component. |
| `IronicAPI` | Manages the Ironic API deployment. |
| `IronicConductor` | Manages conductor service instances (hardware management, one per conductor group). |
| `IronicInspector` | Manages the hardware inspection service. |
| `IronicNeutronAgent` | Manages the Ironic Neutron agent (network provisioning for bare metal). |

The `Ironic` CR has defaulting and validating admission webhooks.
Sub-CRs are created and owned by the `Ironic` controller -- not intended to
be created directly by users.

## Directory structure

**Maintenance rule:** when directories are added, removed, or renamed, or when
their purpose changes, update this table to match.

| Directory | Contents |
|-----------|----------|
| `api/v1beta1/` | CRD types (`ironic_types.go`, `ironicapi_types.go`, `ironicconductor_types.go`, `ironicinspector_types.go`, `ironicneutronagent_types.go`), conditions, webhook markers |
| `cmd/` | `main.go` entry point |
| `internal/controller/` | Reconcilers: `ironic_controller.go`, `ironicapi_controller.go`, `ironicconductor_controller.go`, `ironicinspector_controller.go`, `ironicneutronagent_controller.go` |
| `internal/ironic/` | Ironic-level resource builders (db-sync, common helpers) |
| `internal/ironicapi/` | IronicAPI resource builders |
| `internal/ironicconductor/` | IronicConductor resource builders |
| `internal/ironicinspector/` | IronicInspector resource builders |
| `internal/ironicneutronagent/` | IronicNeutronAgent resource builders |
| `internal/keystone/` | Keystone integration helpers |
| `internal/webhook/` | Webhook implementation |
| `templates/` | Config files and scripts mounted into pods via `OPERATOR_TEMPLATES` env var |
| `config/crd,rbac,manager,webhook/` | Generated Kubernetes manifests (CRDs, RBAC, deployment, webhooks) |
| `config/samples/` | Example CRs (Kustomize overlays). Includes standalone, conductor groups, and TLS variants. |
| `test/functional/` | envtest-based Ginkgo/Gomega tests |
| `test/kuttl/` | KUTTL integration tests |
| `hack/` | Helper scripts (CRD schema checker, local webhook runner) |

## Build commands

After modifying Go code, always run: `make generate manifests fmt vet`.

## Code style guidelines

- Follow standard openstack-k8s-operators conventions and lib-common patterns.
- Use `lib-common` modules for conditions, endpoints, TLS, storage, and other
  cross-cutting concerns rather than re-implementing them.
- CRD types go in `api/v1beta1/`. Controller logic goes in
  `internal/controller/`. Resource-building helpers go in `internal/ironic*`
  packages matching the CR they support.
- Config templates are plain files in `templates/` -- they are mounted at
  runtime via the `OPERATOR_TEMPLATES` environment variable.
- Webhook logic is split between the kubebuilder markers in `api/v1beta1/` and
  the implementation in `internal/webhook/`.

## Testing

- Functional tests use the envtest framework with Ginkgo/Gomega and live in
  `test/functional/`.
- KUTTL integration tests live in `test/kuttl/`.
- Run all functional tests: `make test`.
- When adding a new field or feature, add corresponding test cases in
  `test/functional/` and update fixture data accordingly.

## Key dependencies

- [lib-common](https://github.com/openstack-k8s-operators/lib-common): shared modules for conditions, endpoints, database, TLS, secrets, etc.
- [infra-operator](https://github.com/openstack-k8s-operators/infra-operator): RabbitMQ and topology APIs.
- [mariadb-operator](https://github.com/openstack-k8s-operators/mariadb-operator): database provisioning.
- [keystone-operator](https://github.com/openstack-k8s-operators/keystone-operator): identity service registration.
- [dev-docs/developer.md](https://github.com/openstack-k8s-operators/dev-docs/blob/main/developer.md): developer guide and coding conventions.
