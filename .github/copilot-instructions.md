# ETOS (Eiffel Test Orchestration System) - Github Copilot Instructions

## Project Overview

ETOS is a Kubernetes operator that orchestrates test execution using the Eiffel protocol. It's a **hybrid Go/Python codebase**:
- **Go**: Kubernetes controller (main component) using Kubebuilder v4
- **Python**: CLI tools (`etos_client`, `etosctl`) and local deployment utilities

The system orchestrates distributed test execution by:
1. Creating test environments via providers (IUT, ExecutionSpace, LogArea)
2. Running test suites in Kubernetes Jobs
3. Publishing/consuming Eiffel events via RabbitMQ
4. Managing test lifecycle through Custom Resources

## Two Major Versions: v0 (Stable) vs v1alpha (Development)

**CRITICAL**: ETOS has two parallel architectures serving different use cases:

### **v0 (Stable/Production)**
- **Location**: `cli/src/etos_client/etos/v0/`
- **Architecture**: Microservices-based with separate ETOS services (API, Suite Starter, etc.)
- **Deployment**: Uses existing Helm charts and service deployments
- **Triggering**: REST API call to `/api/etos` endpoint
- **Event Flow**: Eiffel events via RabbitMQ, tracked through SSE v1
- **Status**: Production-ready, currently default version in CLI (`etosctl testrun start`)
- **Use Case**: Running tests in existing ETOS deployments

### **v1alpha (Development)**
- **Location**: `api/v1alpha1/` (Go CRDs), `cli/src/etos_client/etos/v1alpha/` (Python client)
- **Architecture**: Kubernetes-native operator with Custom Resources
- **Deployment**: Operator pattern with CRDs (Cluster, TestRun, Provider, Environment, EnvironmentRequest)
- **Triggering**: Create TestRun CRD or POST to `/api/v1alpha/testrun` endpoint
- **Event Flow**: Supports both SSE v1 (Eiffel) and SSE v2alpha (new protocol), Kubernetes resources track state
- **Status**: Active development, not production-ready
- **Use Case**: Local development, testing new architecture, Kubernetes-native deployments
- **Key Benefit**: Self-contained deployment via Kubernetes operator, no external services required

### Version-Specific Components

**Common Components** (both versions):
- `cli/src/etos_client/shared/` - Shared utilities (downloader, etc.)
- `cli/src/etos_client/sse/` - SSE client implementations (v1 and v2alpha)
- `cli/src/etos_client/types/` - Result types and enums
- Eiffel event definitions and GraphQL queries

#### **v0 (Stable/Production) Components**

**Client Components** (Python CLI):
- `cli/src/etos_client/etos/v0/` - v0 client implementation
- `cli/src/etos_client/etos/v0/command.py` - CLI command for v0
- `cli/src/etos_client/etos/v0/etos.py` - v0 orchestration logic
- `cli/src/etos_client/etos/v0/event_repository/` - GraphQL event queries
- `cli/src/etos_client/etos/v0/events/` - Eiffel event collectors
- `cli/src/etos_client/etos/v0/test_run/` - v0 test run tracking

**API/Backend Components** (Microservices):
- ETOS API - Entry point for test execution (`/api/etos`)
- Suite Starter - Listens for TERCC events, splits test suites
- Environment Provider - External HTTP services for environment provisioning
- Suite Runner - Executes tests in provisioned environments
- Event Repository - GraphQL API for Eiffel event storage/querying
- **Note**: These services are deployed separately via Helm charts (not in this repo)

#### **v1alpha (Development) Components**

**Client Components** (Python CLI):
- `cli/src/etos_client/etos/v1alpha/` - v1alpha client implementation
- `cli/src/etos_client/etos/v1alpha/test_run/` - v1alpha test run tracking (supports SSE v2alpha)

**API/Backend Components** (Kubernetes Operator):
- `api/v1alpha1/` - Go CRD definitions (Cluster, TestRun, Provider, Environment, EnvironmentRequest)
- `internal/controller/` - Kubernetes reconcilers for v1alpha CRDs
- `internal/etos/` - v1alpha ETOS service deployments (API, SSE, LogArea, SuiteStarter, SuiteRunner)
- `internal/extras/` - v1alpha auxiliary services (RabbitMQ, database)
- `internal/webhook/` - Validation/defaulting webhooks for CRDs
- `config/samples/etos_v1alpha1_*.yaml` - Sample CRDs for testing/documentation
- `cmd/main.go` - Controller manager entry point
- **Note**: ALL Go controller code is v1alpha only

### Architecture Comparison

| Aspect | v0 (Stable) | v1alpha (Development) |
|--------|-------------|----------------------|
| **Deployment** | Helm charts, separate services | Kubernetes Operator, CRDs |
| **Orchestration** | Event-driven (Eiffel/RabbitMQ) | Controller reconciliation loops |
| **State Management** | Eiffel events, external DB | Kubernetes resources (CRDs) |
| **Test Triggering** | POST `/api/etos` | Create TestRun CRD or POST `/api/v1alpha/testrun` |
| **Environment Providers** | External HTTP services | Kubernetes Jobs with RBAC |
| **Status Tracking** | SSE v1 + GraphQL queries | SSE v1/v2alpha + Kubernetes status |
| **Self-Contained** | No (requires external services) | Yes (operator deploys everything) |
| **Production Ready** | ✅ Yes | ❌ No (active development) |

## v0 Architecture (Microservices)

### Service Components
v0 uses a microservices architecture with the following key services:

1. **ETOS API** (`etos-api`)
   - Entry point for test execution requests
   - Receives POST to `/api/etos` with test suite definitions
   - Publishes TERCC (Test Execution Recipe Collection Created) Eiffel event
   - Deployed as separate service/pod

2. **Suite Starter** (`etos-suite-starter`)
   - Listens for TERCC events on RabbitMQ queue
   - Splits test suites and triggers environment provisioning
   - Gateway between API and execution

3. **Environment Provider** (`etos-environment-provider`)
   - Provisions test environments via external providers
   - Three provider types: IUT, ExecutionSpace, LogArea
   - Uses JSONTas for dynamic provider configuration
   - Deployed as separate microservice

4. **Suite Runner** (`etos-suite-runner`)
   - Executes test suites in provisioned environments
   - Deploys as Kubernetes Jobs
   - Manages test execution lifecycle

5. **Event Repository**
   - GraphQL API for Eiffel event storage/querying
   - MongoDB backend for event persistence
   - Queried by CLI for test status tracking

### v0 Execution Flow
```
Client (etosctl v0) → ETOS API (/api/etos)
                           ↓
                     TERCC Event → RabbitMQ
                           ↓
                     Suite Starter (consumes TERCC)
                           ↓
                     Environment Provider (provisions)
                           ↓
                     Suite Runner Jobs (execute tests)
                           ↓
                     Eiffel Events → Event Repository
                           ↓
                     Client (tracks via SSE v1 + GraphQL)
```

### v0 Key Patterns
- **Event-Driven**: All coordination via Eiffel events on RabbitMQ
- **External Providers**: Environment providers are external HTTP services
- **GraphQL Queries**: Test tracking uses GraphQL queries to Event Repository
- **Helm Deployment**: Services deployed via traditional Helm charts
- **Stateless Services**: No CRDs, state tracked through Eiffel events

### v0 Request Schema
POST to `/api/etos`:
```json
{
  "artifact_identity": "pkg:...",
  "test_suite_url": "http://...",
  "dataset": {},
  "iut_provider": "default",
  "execution_space_provider": "default",
  "log_area_provider": "default"
}
```

## Core Architecture (v1alpha - Kubernetes Operator)

### Custom Resources (CRDs)
Five main CRDs in `api/v1alpha1/`:

1. **Cluster** - ETOS deployment configuration (ETOS API, Suite Starter, database, message buses)
2. **TestRun** - Test execution request with artifact ID, providers, and test suites
3. **Provider** - Test environment providers (types: `iut`, `execution-space`, `log-area`)
4. **EnvironmentRequest** - Environment provisioning request (creates multiple Environments)
5. **Environment** - Individual test environment instance

### Controller Flow
`TestRunReconciler` (`internal/controller/testrun_controller.go`) orchestrates:
```
TestRun → EnvironmentRequest → Environments → SuiteRunner Jobs → Test Execution
```

Key reconciliation points:
- Validates providers exist via indexed fields (`.spec.providers.{iut,executionSpace,logarea}`)
- Creates EnvironmentRequest from TestRun spec
- Launches SuiteRunner Kubernetes Jobs (one per suite)
- Tracks status through Conditions (Environment, SuiteRunner, Active)

### Internal Packages
- `internal/controller/` - Reconcilers for each CRD
- `internal/etos/` - ETOS service deployments (API, SSE, LogArea, SuiteStarter)
- `internal/extras/` - RabbitMQ/database deployments
- `internal/webhook/` - Validation/defaulting webhooks

## Development Workflows

### Building & Testing
```bash
# Generate CRDs and deepcopy code (ALWAYS run after changing api/v1alpha1/)
make manifests generate

# Run unit tests
make test

# Build controller binary
make build

# Run e2e tests (requires Kind cluster)
make test-e2e

# Lint Go code
make lint
```

### Local Development

**CRITICAL**: Local development requires a Kind (Kubernetes in Docker) cluster.

#### Setup Local Environment
1. **Install Kind**: https://kind.sigs.k8s.io/#installation-and-usage
2. **Create cluster**: `kind create cluster`
3. **Deploy ETOS**: `make deploy-local`

The `make deploy-local` command:
- Uses "packs" (modular deployment units) defined in `local/packs/`
- Deploys v1alpha operator and all services
- Creates namespaces: `etos-system` (controller), `etos-test` (cluster)

#### Testing with Local Cluster
```bash
# Deploy sample TestRun directly to Kubernetes
kubectl create -f config/samples/etos_v1alpha1_testrun.yaml

# Watch TestRun status
kubectl get testrun -n etos-test -w

# Teardown environment
make undeploy-local
```

**Note**: `etosctl` CLI not supported in local dev (requires ingresses). Use direct Kubernetes resources instead.

#### Testing Local Changes

**For Controller changes**:
```bash
# Build and load image
docker build -t my-controller:dev .
kind load docker-image my-controller:dev

# Patch deployment
kubectl patch -n etos-system deployment etos-controller-manager \
  --patch '{"spec": {"template": {"spec": {"containers": [{"name": "manager", "image": "my-controller:dev"}]}}}}'
```

**For ETOS services** (suite-runner, api, sse, log-area, suite-starter, environment-provider):
```bash
# Build and load image
docker build -t my-service:dev .
kind load docker-image my-service:dev

# Update Cluster spec (example for suite-runner)
kubectl patch -n etos-test cluster cluster-sample --type merge \
  --patch '{"spec": {"etos": {"suiteRunner": {"image": "my-service:dev"}}}}'

# Restart dependent services if needed
kubectl rollout restart -n etos-test deployment \
  cluster-sample-etos-api cluster-sample-etos-suite-starter
```

**For Test Runner (ETR) changes**:
Edit `config/samples/etos_v1alpha1_testrun.yaml` and add:
```yaml
dataset:
  DEV: true
  ETR_BRANCH: <your-branch>
  ETR_REPO: <your-fork-url>
```

### Working with CRDs

**After modifying types in `api/v1alpha1/*.go`:**
1. Run `make manifests` to regenerate CRDs in `config/crd/bases/`
2. Run `make generate` to regenerate deepcopy methods
3. Kubebuilder markers control CRD generation - see existing examples for patterns

**Conversion webhooks**: `TestRun`, `Provider`, and `EnvironmentRequest` have conversion logic for API versioning (currently all v1alpha1/hub).

### Controller Development Patterns

**Reconciliation best practices:**
- Use `ctrl.SetControllerReference()` for owner relationships
- Leverage field indexing for efficient lookups (see `SetupWithManager`)
- Status updates should use `r.Status().Update(ctx, resource)`
- Follow structured logging: `log.FromContext(ctx, "key", "value")`

**Job management** (`internal/controller/jobs/`):
- `jobs.NewManager()` creates/tracks SuiteRunner Jobs
- Jobs inherit labels from TestRun (including `etos.eiffel-community.github.io/id`)

## Python CLI Components

Located in `cli/src/`:
- **etos_client**: Legacy entry point (deprecated, redirects to `etosctl testrun start`)
- **etosctl**: Main CLI for ETOS management
  - `etosctl testrun start` - Default uses v0, can specify version with `etosctl testrun v0 start` or `etosctl testrun v1alpha start`
  - Automatic version selection based on subcommand

Build with `setuptools_scm` (version from git tags), managed via `pyproject.toml`.

**Version Selection**:
```bash
# v0 (default)
etosctl testrun start -i <identity> -s <suite> <cluster>

# Explicit v0
etosctl testrun v0 start -i <identity> -s <suite> <cluster>

# v1alpha
etosctl testrun v1alpha start -i <identity> -s <suite> <cluster>
```

## Configuration Patterns

### TestRun Spec Key Fields
```yaml
spec:
  cluster: "cluster-name"           # Target ETOS Cluster
  artifact: "<uuid>"                # Software under test ID
  identity: "pkg:..."               # Artifact identifier (purl format)
  providers:                        # Must exist as Provider CRDs
    iut: "provider-name"
    executionSpace: "provider-name"
    logArea: "provider-name"
  suites:                           # Test suites to execute
    - name: "suite-name"
      priority: 1                   # Execution order
      tests: [...]
```

### Provider Types
Providers have a `type` field (enum: `execution-space`, `iut`, `log-area`) and must:
- Implement REST API with `/v1alpha1/selftest/ping` health endpoint
- Return JSON schema via `/v1alpha1/configure` endpoint
- Be deployed as Kubernetes Services

See examples in `config/samples/etos_v1alpha1_*_provider.yaml`.

## Integration Points

**Message Buses (RabbitMQ)**:
- Two separate buses: Eiffel events and ETOS internal logs
- Configured via `cluster.spec.messageBus.{eiffel,logs}`
- Secrets created: `{cluster-name}-rabbitmq` and `{cluster-name}-messagebus`

**Event Repository**:
- Eiffel GraphQL API for event storage/querying
- Deployed with MongoDB when `cluster.spec.eventRepository.deploy: true`

**Environment Provider Jobs**:
- Kubernetes Jobs that provision test environments
- Created by EnvironmentRequestReconciler
- Use ServiceAccount with RBAC to create/manage Environment CRDs

## Testing

**E2E tests** (`test/e2e/e2e_test.go`) use Ginkgo/Gomega and validate:
- Full TestRun lifecycle execution
- Provider registration and health checks
- SuiteRunner Job creation and completion
- Multi-suite test execution

**Test samples** (`config/samples/`) are used by both e2e tests and docs - don't change hardcoded names/IDs marked with comments.

## Common Gotchas

1. **Always run `make manifests generate`** after changing API types - CI will fail if generated code is out of sync
2. **Webhooks disabled in tests** - Set `ENABLE_WEBHOOKS=false` when running locally without cert-manager
3. **Field indexing** - Controllers use indexed fields for efficient queries; add indexes in `SetupWithManager()`
4. **Leader election** - Enabled by default in production; single controller manages all reconciliation
5. **Resource retention** - TestRuns have configurable retention via `spec.retention.{success,failure}` durations

## Useful Commands

```bash
# Check controller logs
kubectl logs -n etos-system deployment/etos-controller-manager

# Watch TestRun status
kubectl get testrun -w --show-labels

# Inspect generated CRD
kubectl get crd testruns.etos.eiffel-community.github.io -o yaml

# Debug provider connectivity
kubectl exec -it <pod> -- curl http://provider-service:8080/v1alpha1/selftest/ping
```

