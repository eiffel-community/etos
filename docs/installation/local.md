<!---
   Copyright Axis Communications AB
   For a full list of individual contributors, please see the commit history.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
--->
# Installation Guide

Great for development and testing purposes. This will run ETOS in a single-node Kubernetes cluster on your local machine using [kind](https://kind.sigs.k8s.io/).

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [Kind](https://kind.sigs.k8s.io/docs/user/quick-start/)

## Installation

#### Clone the repository:

```bash
git clone https://github.com/eiffel-community/etos
cd etos
```

#### Create a kind cluster:

```bash
kind create cluster
```

#### Install ETOS:

```bash
make deploy-local
```

This will install a local instance of ETOS on your kind cluster and will have executed a single testrun to verify that the installation was successful.
For more information on how to run tests locally, please see the [getting started guide](../getting_started/index.md).

## Verify the local API

The repository contains a small v1beta1 suite for verifying that a TestRun can be
started through the ETOS API:
[`local/verification/v1beta1-api-suite.yaml`](../../local/verification/v1beta1-api-suite.yaml).

Forward the API and log-area services to the local machine:

```bash
kubectl port-forward -n etos-test svc/cluster-sample-etos-api 8081:80
kubectl port-forward -n etos-test svc/cluster-sample-etos-logarea 8082:80
```

Run each port-forward in a separate terminal. In another terminal, upload the
suite to log-area:

```bash
curl --fail-with-body -X POST \
  --data-binary @local/verification/v1beta1-api-suite.yaml \
  'http://127.0.0.1:8082/logarea/upload?path=suites/api-v1beta1-verification.yaml'
```

Create a TestRun through the v1beta1 API. Replace the artifact ID if the local
event repository contains a different artifact:

```bash
curl --fail-with-body -X POST \
  'http://127.0.0.1:8081/api/v1beta1/testrun' \
  -H 'Content-Type: application/json' \
  --data-raw '{
    "artifact_id": "268dd4db-93da-4232-a544-bf4c0fb26dac",
    "artifact_identity": null,
    "test_suite_url": "http://cluster-sample-etos-logarea.etos-test/logarea/upload?path=suites/api-v1beta1-verification.yaml",
    "execution_space_provider": "cluster-sample-execution-space-provider",
    "iut_provider": "cluster-sample-iut-provider",
    "log_area_provider": "cluster-sample-log-area-provider",
    "timeout": 3600
  }'
```

The response contains the TestRun's `tercc` identifier. Check the Kubernetes
resource and wait for a `Passed` verdict:

```bash
kubectl get testrun -n etos-test
kubectl get testrun -n etos-test \
  -o custom-columns=NAME:.metadata.name,VERDICT:.status.verdict
```

## Update services in a running local cluster

ETOS service images and pull policies can be changed by patching the running
`Cluster` resource. For a local image, load it into kind first:

```bash
kind load docker-image etos-api:local --name kind
```

Patch the ETOS API image and make Kubernetes use the image already loaded into
kind:

```bash
kubectl patch cluster cluster-sample -n etos-test --type=merge \
  -p '{"spec":{"etos":{"api":{"image":"etos-api:local","imagePullPolicy":"IfNotPresent"}}}}'
```

The same pattern applies to other service fields in the cluster specification,
for example the log-area service:

```bash
kubectl patch cluster cluster-sample -n etos-test --type=merge \
  -p '{"spec":{"etos":{"logArea":{"image":"etos-logarea:local","imagePullPolicy":"IfNotPresent"}}}}'
```

Use `kubectl get cluster` to follow the operator's reconciliation and
`kubectl rollout status` to wait for the affected Deployment:

```bash
kubectl get cluster cluster-sample -n etos-test -w
kubectl rollout status deployment/cluster-sample-etos-api -n etos-test
```

For a registry image, set `imagePullPolicy` to `Always` when applying the
patch. The operator reconciles the Deployment from the `Cluster` specification,
so patching the Deployment directly is temporary and will be overwritten.

### Update provider images

Provider images are built with their provider-specific Make targets. Build the
provider image and load it into kind:

```bash
make iutprovider-docker IMG=iutprovider:local
kind load docker-image iutprovider:local --name kind
```

Do not patch the generated `cluster-sample-*-provider` resources. They are
owned by the `Cluster` resource and can be restored by the controller. Create a
separate, explicitly named provider resource instead:

```bash
cat <<'EOF' | kubectl apply -f -
apiVersion: etos.eiffel-community.github.io/v1alpha1
kind: Provider
metadata:
  name: local-iut-provider
  namespace: etos-test
spec:
  type: iut
  image: iutprovider:local
EOF
```

Use the corresponding custom resource name when starting a TestRun or calling
the API. For example, set
`iut_provider` to `local-iut-provider` in the v1beta1 API request above.

Create custom execution-space and log-area providers with the same pattern:

```bash
make executionspaceprovider-docker IMG=executionspaceprovider:local
kind load docker-image executionspaceprovider:local --name kind
cat <<'EOF' | kubectl apply -f -
apiVersion: etos.eiffel-community.github.io/v1alpha1
kind: Provider
metadata:
  name: local-execution-space-provider
  namespace: etos-test
spec:
  type: execution-space
  image: executionspaceprovider:local
EOF

make logareaprovider-docker IMG=logareaprovider:local
kind load docker-image logareaprovider:local --name kind
cat <<'EOF' | kubectl apply -f -
apiVersion: etos.eiffel-community.github.io/v1alpha1
kind: Provider
metadata:
  name: local-log-area-provider
  namespace: etos-test
spec:
  type: log-area
  image: logareaprovider:local
  logAreaProviderConfig:
    livelogs: http://cluster-sample-etos-logarea/logarea/v1alpha/$context
    upload:
      as_json: false
      method: POST
      url: http://cluster-sample-etos-logarea/logarea/v1alpha/upload
EOF
```

Patch a custom provider whenever its image changes:

```bash
make iutprovider-docker IMG=iutprovider:local
kind load docker-image iutprovider:local --name kind
kubectl patch provider local-iut-provider -n etos-test --type=merge \
  -p '{"spec":{"image":"iutprovider:local"}}'
```

Verify the resource before starting a TestRun:

```bash
kubectl get provider local-iut-provider -n etos-test \
  -o jsonpath='{.spec.image}{"\n"}'
```

Custom providers are independent resources and can be reused by multiple
TestRuns. Keep their names stable and update only their `spec.image` when
needed; the generated default providers remain available for workloads that
still reference them.

### Update the ETOS test runner package locally

#### Build a preinstalled runner image

Build the committed package in the `etos-test-runner` repository. No package
publication, remote Git push or cluster-side Git server is needed. Worktrees
are supported: the build uses `git archive HEAD` and substituted SCM metadata,
not the worktree's `.git` pointer or uncommitted files.

From the runner checkout (or its named worktree), review and commit only the
intended files before building:

```bash
git status --short
git add src/etos_test_runner/etr.py  # Example: name your changed files explicitly
git commit -m 'Update local test runner'
RUNNER_IMAGE="etos-test-runner:local-$(git rev-parse --short=12 HEAD)"
make docker-build IMG="$RUNNER_IMAGE"
make docker-test IMG="$RUNNER_IMAGE"
docker image inspect "$RUNNER_IMAGE" \
  --format '{{index .Config.Labels "org.opencontainers.image.revision"}}'
kind load docker-image "$RUNNER_IMAGE" --name kind
```

Skip the commit commands if the desired revision is already committed. Use your
actual kind cluster name instead of `kind` where appropriate. The local image
test is offline and does not start a TestRun. Building requires access to
`ghcr.io/eiffel-community/etos-base-test-runner:ubuntu-noble` and Python package
indexes. Loading the image is a separate, explicit cluster operation.

The image installs the package and constrained dependencies at build time into
the base image's `/home/etos/.venv` (Python 3.13). Its entrypoint executes
`python -m etos_test_runner.etr`, forwarding runner arguments without running
the base image's installation script. Runtime `DEV`, `ETR_REPOSITORY`,
`ETR_BRANCH` and `ETR_VERSION` therefore cannot replace the baked package.
The base tools, etos user (UID/GID 3705), artifact paths and
`TEST_FRAMEWORK_VENV` are inherited; the package's `lib/executor.sh` still
executes the actual tests. This removes **runner package installation** at
startup, not any dependencies or network access required by your test commands.

Select the image in **each test execution's** suite `environment.testRunner`.
For example, from the ETOS checkout, create a new suite without modifying the
verification fixture (set `RUNNER_IMAGE` in this terminal too):

```bash
RUNNER_IMAGE='etos-test-runner:local-<12-character-commit>'
sed "s|ghcr.io/eiffel-community/etos-base-test-runner:ubuntu-noble|${RUNNER_IMAGE}|g" \
  local/verification/v1beta1-api-suite.yaml > local/verification/prebuilt-runner-suite.yaml
```

Upload this new suite as described under [Verify the local API](#verify-the-local-api),
using a distinct log-area path such as `suites/prebuilt-runner.yaml` and that
same path in `test_suite_url`. The normal execution-space provider can be reused;
no `dev` dataset override or custom Provider is required for a prebuilt runner.
The suite field is an image **string**, not an object containing a pull policy.

The generic execution-space provider in `cmd/executionspaceprovider/main.go`
sets the Job container image from this field and leaves `imagePullPolicy`
unset. Kubernetes defaults it to `IfNotPresent` for an explicit, non-`latest`
tag, so the image loaded into kind is used. Use a new commit-derived tag on
each build and load it into all nodes before submitting the suite. Do not use
`:latest`, an untagged name, or try to configure this runner's pull policy via a
service image patch. A missing local image can cause a registry pull attempt.

Inspect the installed package without starting the runner:

```bash
docker run --rm --network none --entrypoint /home/etos/.venv/bin/python \
  "$RUNNER_IMAGE" -c 'from importlib.metadata import version; print(version("etos_test_runner"))'
```

The SCM-derived installed version and the image's full revision label identify
the baked source. Rebuild, reload and update the suite image when the local
commit changes; uncommitted edits are deliberately excluded.
