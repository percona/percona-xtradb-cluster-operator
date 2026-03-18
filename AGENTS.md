This file provides guidance to AI agents when working with code in this repository.

This is the Percona Kubernetes Operator for MySQL based on Percona XtraDB Cluster (PXC) repository. It contains:

- Kubernetes operator for managing Percona XtraDB Cluster deployments
- Custom Resource Definitions (PerconaXtraDBCluster, PerconaXtraDBClusterBackup, PerconaXtraDBClusterRestore)
- Backup and restore tooling (XtraBackup, Point-in-Time Recovery)
- Proxy layer management (HAProxy, ProxySQL)
- TLS certificate lifecycle management
- Health monitoring and automatic recovery

## Key Architecture Components

### PerconaXtraDBCluster Custom Resource
The primary API (`pkg/apis/pxc/v1/pxc_types.go`) for users to configure a PXC cluster. It defines:
- PXC node configuration (size, resources, storage, affinity)
- Proxy configuration (HAProxy or ProxySQL)
- Backup schedules and storage destinations (S3, Azure, PVC)
- TLS settings (self-signed or cert-manager integration)
- PMM monitoring integration
- User management and password policies
- Upgrade strategy (SmartUpdate, RollingUpdate)

The operator watches PerconaXtraDBCluster resources and reconciles StatefulSets, Services, Secrets, ConfigMaps, and Jobs to maintain the desired cluster state.

### PerconaXtraDBClusterBackup / PerconaXtraDBClusterRestore
Companion CRDs for managing backup and restore operations:
- `PerconaXtraDBClusterBackup` (`pkg/apis/pxc/v1/pxc_backup_types.go`) - on-demand backup managements
- `PerconaXtraDBClusterRestore` (`pkg/apis/pxc/v1/pxc_prestore_types.go`) - full restores and Point-in-Time Recovery (PITR) via GTID or date

### Controller Architecture
The operator uses controller-runtime and has three main reconcilers in `pkg/controller/`:
- `pxc/controller.go` - Main cluster reconciler (StatefulSets, Services, TLS, users, replication, scheduled backups)
- `pxcbackup/controller.go` - Backup operations
- `pxcrestore/controller.go` - Restore operations

### Supporting Binaries
Built from `cmd/`:
- `manager/main.go` - Main operator entry point
- `peer-list/` - Peer discovery for Galera cluster formation
- `pitr/` - Point-in-Time Recovery tool
- `mysql-state-monitor/` - Health monitoring sidecar
- `xtrabackup/` - Backup server sidecar and backup executor (communicates via gRPC, see `pkg/xtrabackup/`)

### Webhooks
Validating admission webhook (`pkg/webhook/`) for PerconaXtraDBCluster CR validation. Enabled via `enableCRValidationWebhook` field in the CR.

## Common Development Commands

### Building
```bash
make build              # Build Docker image (runs code generation first)
./e2e-tests/build       # Build Docker image directly
```

The `IMAGE` environment variable controls the target image name:
```bash
export IMAGE=myregistry/percona-xtradb-cluster-operator:my-branch
```

### Code Generation
```bash
make generate           # Generate CRDs, RBAC, DeepCopy, and protobuf code
make manifests          # Generate deploy/crd.yaml, deploy/bundle.yaml, deploy/cw-bundle.yaml
```

**After modifying API types in `pkg/apis/pxc/v1/`, always run both `make generate` and `make manifests` to update generated code and deployment manifests.** Forgetting this step will cause CI failures and runtime mismatches between your code and the deployed CRDs.

### Testing
```bash
make test                           # Run unit tests (includes generate, fmt, vet, envtest setup)
./e2e-tests/run                     # Run all e2e tests
./e2e-tests/<test-name>/run         # Run specific e2e test
```

Unit tests use **Ginkgo v2 + Gomega**, **envtest** (Kubernetes API version 1.34.1) or fake client (controller-runtime). E2E tests are **bash script-based** and require a live Kubernetes cluster.

### Deployment
```bash
make install            # Install CRDs and RBAC to current cluster
make uninstall          # Remove CRDs and RBAC
make deploy             # Deploy operator (with DEBUG logging and telemetry disabled)
make undeploy           # Remove operator deployment
```

### Validation
```bash
make fmt                # Run go fmt
make vet                # Run go vet
```

CI also runs: golangci-lint, shfmt, shellcheck, misspell, and alex (inclusive language).

## Coding Conventions

- Keep reconciler logic idempotent. Re-running reconcile should converge to the same state without duplicating resources or mutating unrelated fields.
- Keep strong focus on code readibility over performance, unless explicitly asked.
- Prefer explicit, small helper functions over large monolithic reconcile blocks. Keep package boundaries clear (`pkg/controller` for orchestration, `pkg/pxc` for app/resource builders, `pkg/k8s` for Kubernetes helpers).
- Always pass `context.Context` through API calls and use `client.IgnoreNotFound(err)` when handling not-found reads/deletes.
- Wrap returned errors using `errors.Wrap` (from `github.com/pkg/errors`) so callers and logs preserve root causes
- Use structured logging with stable keys and include CR identity (`namespace`, `name`) for operator actions that may be debugged in production.
- Keep CRD/API changes backward compatible: add optional fields, preserve deprecated fields, and gate behavior changes with `cr.CompareVersionWith(...)`.
- Avoid implicit behavior in defaulting. Set defaults centrally in API/defaulting paths instead of scattering defaults across reconcilers.
- Minimize StatefulSet/Service spec churn. Unnecessary spec changes can trigger rolling restarts; only change fields when required.
- Add tests with each behavior change: unit tests for helpers/reconcilers and e2e coverage for user-visible workflows or upgrade compatibility.
- Before opening a PR, run at least `make fmt`, `make vet`, and relevant unit tests; run `make generate` + `make manifests` for API changes.
- Use `github.com/stretchr/testify/assert` and `github.com/stretchr/testify/require` for unit test assertions.

## Adding New APIs or Modifying Existing CRDs

### Modifying CRD Types
1. Edit the type definition in `pkg/apis/pxc/v1/*_types.go`
2. Add appropriate kubebuilder markers for validation, defaults, etc.
3. Run `make generate` to update DeepCopy methods and CRD YAML
4. Run `make manifests` to regenerate `deploy/crd.yaml` and bundle files
5. Update or add controller logic in `pkg/controller/` to handle the new field
6. Add unit tests
7. Update the rbac in `deploy/rbac.yaml` and run `make manifests`
8. Add e2e tests if the change affects user workflows

### Validation Markers
Use kubebuilder markers to add validation to CRD fields:
```go
// Schedule is a cron-formatted backup schedule.
// +optional
// +kubebuilder:validation:Optional
Schedule string `json:"schedule,omitempty"`
```

CRD generation markers can be found [here](https://book.kubebuilder.io/reference/markers/crd).

All validation constraints must be documented in the field's comment.

#### CEL Expressions

Prefer using CEL expressions for static validations using the `+kubebuilder:validation:XValidation` marker.

For example:
```go
// +kubebuilder:validation:XValidation:rule="self.maxLength >= self.minLength"
type PasswordGenerationOptions struct {
	MaxLength int `json:"maxLength"`
	MinLength int `json:"minLength"`
}
```
Documentation for common CEL expressions used in Kubernetes can be found [here](https://kubernetes.io/docs/reference/using-api/cel/)

### CRD Version Policy
- Current API version: `v1` (package path `pkg/apis/pxc/v1`)
- New fields should generally be optional to maintain backward compatibility
- The `crVersion` field in each CR tracks the operator version the CR was written for

#### Maintaining backward compatibility

The `PerconaXtraDBCluster` CRD includes `.spec.crVersion`, which helps the operator preserve behavior across upgrades.

When introducing new CRD fields that can change running Pods or StatefulSets, keep reconciliation backward compatible for existing clusters. Gate new behavior with `cr.CompareVersionWith()` so older CRs continue using legacy fields while newer/upgraded CRs use the new fields.

Generic pattern:
```go
config := cr.Spec.SomeConfig // for clusters below 1.20.0
if cr.CompareVersionWith("1.20.0") >= 0 {
    config = cr.Spec.SomeNewConfig // for new or upgraded clusters
}
applyConfigToPod(config)
```

Concrete examples from this repository:

- **Prefer new field, fallback to legacy field** (`pkg/pxc/service.go`, HAProxy service type):
```go
if cr.CompareVersionWith("1.14.0") >= 0 && len(cr.Spec.HAProxy.ExposePrimary.Type) > 0 {
    svcType = cr.Spec.HAProxy.ExposePrimary.Type
} else if len(cr.Spec.HAProxy.ServiceType) > 0 {
    svcType = cr.Spec.HAProxy.ServiceType
}
```

- **Same fallback rule for related settings** (`pkg/pxc/service.go`, external traffic policy):
```go
if cr.CompareVersionWith("1.14.0") >= 0 && len(cr.Spec.HAProxy.ExposePrimary.ExternalTrafficPolicy) > 0 {
    svcTrafficPolicyType = cr.Spec.HAProxy.ExposePrimary.ExternalTrafficPolicy
} else if len(cr.Spec.HAProxy.ExternalTrafficPolicy) > 0 {
    svcTrafficPolicyType = cr.Spec.HAProxy.ExternalTrafficPolicy
}
```

- **Version-aware defaults when a new nested field is introduced** (`pkg/apis/pxc/v1/pxc_types.go`):
```go
if cr.CompareVersionWith("1.14.0") >= 0 {
    if c.HAProxy.ExposeReplicas == nil {
        c.HAProxy.ExposeReplicas = &ReplicasServiceExpose{ServiceExpose: ServiceExpose{Enabled: true}}
    }
} else if c.HAProxy.ReplicasServiceEnabled == nil {
    t := true
    c.HAProxy.ReplicasServiceEnabled = &t
}
```

Do not remove existing fields from `spec` or `status`. If a field is deprecated, keep it for compatibility and log a clear deprecation notice that points users to the replacement field.

## Safety Guardrails

**This operator manages production database clusters. Incorrect changes can cause data loss, cluster unavailability, or split-brain scenarios.** Apply extra caution in these areas:

### Database State and Data Integrity
- **Never modify reconciliation order** without understanding Galera replication implications. PXC nodes must be shut down and started in a specific order to avoid data loss.
- **Finalizers** (`delete-pxc-pods-in-order`, `delete-ssl`, `delete-proxysql-pvc`, `delete-pxc-pvc`) ensure safe cleanup. Do not remove or reorder finalizer logic without understanding the consequences.
- **Full crash recovery** (`pkg/controller/pxc/full_crash_recovery.go`) handles Galera bootstrap after total cluster failure. Changes here risk making clusters unrecoverable.

### Backup and Restore
- Backup deadline logic (`pxcbackup/deadline.go`) prevents stuck backup jobs. Do not weaken or remove deadline enforcement.
- Restore operations pause the entire cluster. The restore controller coordinates a multi-step process (pause -> restore -> restart). Interrupting this sequence can leave the cluster in an inconsistent state.
- PITR relies on continuous binlog collection. Gaps in binlog collection mean unrecoverable data windows.

### TLS and Certificates
- TLS code (`pkg/controller/pxc/tls.go`, `pkg/pxctls/`) manages certificates for inter-node communication and client connections. Breaking TLS configuration can prevent cluster nodes from communicating.
- Certificate rotation must happen before expiry. The operator tracks `notAfter` dates and rotates proactively.

### User and Secret Management
- System users (root, monitor, proxysql, replication) are required for cluster operation. Do not remove or rename them.
- Password changes trigger rolling restarts. Understand the blast radius before modifying user reconciliation in `pkg/controller/pxc/users.go`.

### StatefulSet Changes
- PXC and proxy components use StatefulSets with persistent storage. Changes to StatefulSet specs (volume claims, pod templates) may require manual intervention or cause rolling restarts.
- The `SmartUpdate` strategy coordinates version upgrades across PXC nodes to maintain quorum. Do not bypass this logic.

### Feature Gates
- Alpha/beta features are gated via `pkg/features/`. Do not enable experimental features by default. Feature gates exist to protect production users.

## Testing Framework

### Unit Tests
Located alongside source code (`*_test.go`). Use Ginkgo/Gomega with envtest:
```go
var _ = Describe("PerconaXtraDBCluster Controller", func() {
    Context("When reconciling a new cluster", func() {
        It("Should create the PXC StatefulSet", func() {
            // Test implementation
        })
    })
})
```

Run with: `make test`

### E2E Tests
Located in `e2e-tests/`. Each subdirectory is a test suite with a `run` script. There are 68+ test suites covering:
- Cluster lifecycle: `init-deploy`, `recreate`, `scaling`, `limits`
- Backups: `demand-backup`, `scheduled-backup`, `pitr`, `backup-storage-tls`
- Upgrades: `smart-update1`, `smart-update2`, `upgrade-haproxy`, `upgrade-proxysql`
- High availability: `self-healing-chaos`, `operator-self-healing-chaos`, `affinity`
- Security: `security-context`, `tls-issue-cert-manager`, `validation-hook`
- Monitoring: `monitoring-pmm3`
- Network: `cross-site`, `proxy-protocol`

E2E tests require:
- A live Kubernetes cluster (EKS, GKE, AKS, OpenShift, or minikube)
- Cluster admin permissions
- The operator built and deployed from your local changes

Key environment variables for e2e tests:
```bash
export IMAGE=myregistry/percona-xtradb-cluster-operator:my-branch
export IMAGE_PXC=perconalab/percona-xtradb-cluster-operator:main-pxc8.4
export IMAGE_BACKUP=perconalab/percona-xtradb-cluster-operator:main-pxc8.4-backup
export IMAGE_HAPROXY=perconalab/percona-xtradb-cluster-operator:main-haproxy
export IMAGE_PROXY=perconalab/percona-xtradb-cluster-operator:main-proxysql
export IMAGE_PMM_CLIENT=perconalab/pmm-client:3-dev-latest
export IMAGE_LOGCOLLECTOR=perconalab/fluentbit:main-logcollector
export OPERATOR_NS=pxc-operator          # Enable cluster-wide mode
export SKIP_REMOTE_BACKUPS=1             # Skip S3/Azure tests (use MinIO only)
export CLEAN_NAMESPACE=1                 # Delete all non-system namespaces after tests (destructive!)
```

Shared test helpers live in `e2e-tests/functions`.

## Project Layout

| Directory | Purpose |
|-----------|---------|
| `cmd/` | Entry points for all binaries (operator, peer-list, pitr, etc.) |
| `pkg/apis/pxc/v1/` | CRD type definitions and generated code |
| `pkg/controller/` | Reconcilers for PXC cluster, backup, and restore |
| `pkg/pxc/` | PXC-specific logic (app specs, backup, queries, users) |
| `pkg/k8s/` | Kubernetes utility functions |
| `pkg/webhook/` | Admission webhook |
| `pkg/version/` | Version management and Percona Version Service client |
| `pkg/naming/` | Resource naming conventions |
| `pkg/pxctls/` | TLS certificate generation |
| `pkg/xtrabackup/` | gRPC backup server (protobuf API) |
| `pkg/features/` | Feature gates for alpha/beta features |
| `config/` | Kustomize overlays for CRDs, RBAC, operator deployment |
| `deploy/` | Ready-to-use deployment manifests (`bundle.yaml`, `cr.yaml`, etc.) |
| `build/` | Dockerfile and shell scripts for container images |
| `e2e-tests/` | Bash-based end-to-end test suites |
| `.github/workflows/` | CI pipelines (unit tests, code quality checks) |

## Container Images

The operator Dockerfile (`build/Dockerfile`) is a multi-stage build (Go 1.25 builder -> UBI9 minimal) that produces 6 binaries. The container runs as non-root (UID 2).

Related container images (PXC, XtraBackup, HAProxy, ProxySQL, LogCollector, PMM client) are maintained in the [percona-docker](https://github.com/percona/percona-docker) repository, not in this repo.

## Contributing

- Branch naming convention: `<Jira-issue>-<short-description>` (e.g., `K8SPXC-622-fix-feature-X`)
- Commit messages should reference the Jira issue (e.g., `K8SPXC-622 fixed by ...`)
- PRs go through automated testing (~3 hours) and manual code review
- CLA signature is required: https://cla-assistant.percona.com/percona/percona-xtradb-cluster-operator
- See `CONTRIBUTING.md` for full details
