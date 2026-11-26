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

<img src="./docs/images/etos-logo.png" alt="ETOS" width="350"/>

[![Sandbox badge](https://img.shields.io/badge/Stage-Sandbox-yellow)](https://github.com/eiffel-community/community/blob/master/PROJECT_LIFECYCLE.md#stage-sandbox)

# ETOS

ETOS (Eiffel Test Orchestration System) is a new test orchestration system which takes away control of how to run and what to run from the system itself and places it into the hands of the relevant engineers.

The idea of having a system dictate what and how to run is finished. Let's bring back control to the testers.

Please see the [documentation](./docs/index.md) for more information, tutorials and guides on how to use ETOS.

## Features

- Generic test suite execution based solely on JSON.
- Mix and match test suites, regardless of programming language.
- Separation of concerns between testers, test automation engineers and system engineers.
- Eiffel protocol implementation.

## Observability (OpenTelemetry)

ETOS adds native OpenTelemetry health metrics alongside its existing traces and logs. Custom ETOS
metrics use OpenTelemetry instruments directly; only controller-runtime and framework metrics use
the Prometheus-to-OpenTelemetry bridge. All three signals are exported over OTLP gRPC.

### Configuration

Export is opt-in: with
`OTEL_EXPORTER_OTLP_ENDPOINT` unset, nothing is exported (even if a signal-specific endpoint is set)
and the install is functional but unobserved. A configured empty or malformed endpoint is an error.

- Once enabled, a signal-specific endpoint takes precedence over the common endpoint. gRPC is the only
  protocol; explicitly incompatible protocol or delta temporality settings fail initialization.
- `OTEL_EXPORTER_OTLP_INSECURE=true` disables TLS towards the Collector.
- Standard common/signal-specific TLS, headers and exporter timeouts are interpreted by the pinned SDK.
  Invalid certificate files fail initialization, not just a warning. No headers or credentials are logged.
- Resource identity retains service name/namespace, hostname instance ID, VCS revision, SDK, OS and process
  attributes. `OTEL_RESOURCE_ATTRIBUTES`/`OTEL_SERVICE_NAME` overrides are not enabled.
  `service.instance.id` was already present on trace/log resources; its use in metric streams adds
  backend series-cardinality considerations.
- Metrics use cumulative temporality, `OTEL_METRIC_EXPORT_INTERVAL` defaults to 15s and
  `OTEL_METRIC_EXPORT_TIMEOUT` to 5s. Overrides are positive milliseconds, at most one hour.
  The per-instrument cardinality limit is 100000; it does not cap backend series accumulated across
  instances and retention. Treat any `otel.metric.overflow=true` aggregate as incomplete telemetry.
- Histograms are absent until the first real observation; dashboards and alerts must tolerate missing series.

Deploy with the opt-in overlay (edit the placeholder endpoint in `config/otel/manager_otel_patch.yaml`
or patch it in your own overlay, e.g. `kustomize build config/otel`). The default `config/default`
has no telemetry configuration.

### Health metrics and provider events

The new custom metrics cover resource inventory, execution outcomes and RabbitMQ publication:

| Metric family | Meaning |
| --- | --- |
| `etos_inventory_*` | Current resource counts, TestRun phases, test verdicts, operational conclusions, creation windows, oldest active age and overdue runs, plus snapshot readiness, freshness and collection failures. |
| `etos_testrun_finished_total`, `etos_testrun_duration_seconds` | TestRun completion counts and elapsed durations by outcome, recorded after terminal status is persisted. |
| `etos_environment_request_finished_total`, `etos_environment_request_duration_seconds` | EnvironmentRequest completion counts and elapsed durations by outcome, recorded after terminal status is persisted. |
| `etos_message_publish_attempts_total`, `etos_message_publish_duration_seconds` | Publication observations and durations by operation and outcome. |

Cluster-attributed metrics use `etos_cluster="<namespace>/<Cluster-name>"`, or `unassigned` when
attribution cannot be established. TestRun IDs are not metric dimensions. Inventory domain metrics
use `scope="cluster"`; snapshot health uses `scope="manager"` and covers the manager's inventory,
not an individual cluster. Test verdict and operational conclusion are distinct: a failed test does
not necessarily mean ETOS failed to execute it.

Inventory is reconstructed from the leader's manager cache every 15s. Interpret it only when
`etos_inventory_snapshot_ready` is 1 and
`etos_inventory_snapshot_last_success_timestamp_seconds` is no more than 30s old.
`etos_inventory_testrun_window_complete` indicates whether retention covers the creation-count window;
otherwise those counts are lower bounds. It does not establish completeness of transition counters.

Provider provisioning adds the structured log event `event="etos.provider.completed"`, with
`operation="provision"`, success/failure `outcome`, `providerType`, `etos_cluster` and numeric
`duration_seconds`. Successful Environment provisioning also includes numeric `request_to_ready_seconds`
when the request creation timestamp is valid. These events require updated provider images, not just
an updated manager. Preserve their fields when deriving log-based metrics, and avoid ingesting the
same console stream into the same backend again.

RabbitMQ `queued`, `confirmed` and `unconfirmed` durations are callback-local timings, not queue
residence time or end-to-end acknowledgement latency.

### Breaking change: the Prometheus scrape endpoint is removed

The manager no longer serves `/metrics`. The `--metrics-bind-address`, `--metrics-secure` and
`--metrics-cert-*` flags, the metrics Service, ServiceMonitor, metrics certificate, metrics RBAC
(`metrics-auth`, `metrics-reader`) and metrics NetworkPolicy are gone. Custom overlays that reference
`../prometheus`, `metrics_service.yaml` or the metrics patches must drop them. A Prometheus Operator is no
longer required by ETOS. Webhooks, cert-manager TLS and probes (8081) are unchanged. To roll back,
redeploy the previously released manifests.

Metrics consumers (dashboards, alerts, health checks) must read from the Collector pipeline instead of the
scrape job `etos-controller-manager-metrics-service`; replace `up{...}` checks with source-freshness checks
(for example, the inventory last-success timestamp for each writer).

### Metric consumption and aggregation

- Preserve a unique writer identity for independently produced cumulative metric streams, using
  `service.instance.id` or an equivalent backend instance dimension. Avoid duplicate identity labels,
  and budget series churn, especially for short-lived provider Jobs. Do not drop writer identity without
  reset-aware aggregation. `etos_cluster` identifies the managed cluster, not the writer.
- Promote resource attributes to metric labels selectively. Instance replacement creates new series;
  short-lived provider Jobs can create substantially more churn than long-lived manager replicas.
- Select fresh inventory instances using `etos_inventory_snapshot_last_success_timestamp_seconds`,
  readiness, and source identity. Require last-success age <=30s, and verify exported sample timestamps/
  Collector cache expiration. A healthy Collector's `up` is not evidence that a manager is still running.
- Calculate `rate`/`increase` on each process counter **before** summing. Use recording rules for
  lower-cardinality dashboard aggregates. For inventory gauges, select the fresh leader rather than
  summing old and new instances. Do not assume the former scrape target's `job,instance` labels persist.
- Native histograms have no series until a real duration. Raw first-export sum/count/buckets retain that
  sample; Prometheus windowed `rate`/`increase` may miss it without an earlier baseline. Empty quantiles
  remain Unknown. Do not manufacture zero observations or unconditionally fill missing data with zero.
- Check the Collector/backend's translated metric names when writing queries. Unit translation can
  append `_ratio` to dimensionless inventory gauges; avoid doubled `_total`/`_seconds` suffixes.
  Framework histograms may be exported as exponential histograms and require backend support.

### Operational limitations and rollback

Manager shutdown budgets 15s for manager workers, 15s for pooled publisher confirmations, and
15s for telemetry (five seconds per signal, logs last); the pod grace period is 60s.
Provider confirmation draining has a 30s budget followed by 15s telemetry cleanup; provider and
releaser Jobs have 60s pod grace periods.

**Known limitation:** the pinned RabbitMQ HA client's reconnecting `Send` and transport `Close` are
not cancellable. Confirmation waits are bounded, but transport stalls can still exceed these budgets.
Bounded process exit under broker outages requires a cancellable transport API. If the pod is killed
while transport shutdown is stuck, the final telemetry flush may never run.

## Support

If you need help, please get in contact with us.

For questions and discussions, please use the [discussions](https://github.com/eiffel-community/etos/discussions) on github.
For bug reports and feature requests, please use the [issue tracker](https://github.com/eiffel-community/etos/issues)

There is also a mailing list at: etos-maintainers@googlegroups.com

## Code of Conduct and Contributing
To get involved, please see [Code of Conduct](https://github.com/eiffel-community/.github/blob/master/CODE_OF_CONDUCT.md) and [contribution guidelines](https://github.com/eiffel-community/.github/blob/master/CONTRIBUTING.md).

Note that these files are located in the .github repository. See [this](https://docs.github.com/en/github/building-a-strong-community/creating-a-default-community-health-file) page for further details regarding default community health files.

## About this repository
The contents of this repository are licensed under the [Apache License 2.0](./LICENSE).

## About Eiffel
This repository forms part of the Eiffel Community. Eiffel is a protocol for technology agnostic machine-to-machine communication in continuous integration and delivery pipelines, aimed at securing scalability, flexibility and traceability. Eiffel is based on the concept of decentralized real time messaging, both to drive the continuous integration and delivery system and to document it.

Visit [Eiffel Community](https://eiffel-community.github.io) to get started and get involved.
