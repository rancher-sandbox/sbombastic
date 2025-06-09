|              |                                                                |
| :----------- | :------------------------------------------------------------- |
| Feature Name | Scan trigger                                                   |
| Start Date   | 2025-05-30                                                     |
| Category     | Architecture                                                   |
| RFC PR       | [#227](https://github.com/rancher-sandbox/sbombastic/pull/227) |
| State        | **ACCEPTED**                                                   |

# Summary

[summary]: #summary

This RFC introduces the `ScanJob` CRD and describes how a user or automated systems can trigger a scan on a `Registry`.
This supersedes part of [RFC 0001 - Scanner architecture and design](./0001_scanner_architecture_and_design.md), removing the need for a separate `DiscoveryJob` CRD.

# Motivation

[motivation]: #motivation

We need a way for users and other actors to trigger scans on container registries through a declarative, Kubernetes-native API.
The `ScanJob` CRD follows the same pattern as the native Kubernetes `batch/v1` `Job` resource: its status field will track scan progress and results, providing familiar Kubernetes-style observability for scan operations.
This enables better integration with the Rancher UI, workflows, automation, and GitOps processes while leveraging standard Kubernetes tooling for management and monitoring.

## Examples / User Stories

[examples]: #examples

- As a user I want to manually trigger the execution of a scan configuration on demand.
- As a user I want the system to automatically trigger scans on a registry periodically.
- As a user I want the system to automatically trigger a scan when a new registry is created or an existing one is updated with new repositories.

# Detailed design

[design]: #detailed-design

This RFC replaces the idea of having separate "discovery" and "scan" jobs.
From now on, "scan" means the full process: finding images in a registry, creating SBOMs, and checking for vulnerabilities.

## ScanJob CRD

To trigger a scan, we define the `ScanJob` custom resource, which serves as a trigger for scanning a specific `Registry` resource.

An example `ScanJob` manifest looks like this:

```yaml
apiVersion: sbombastic.rancher.io/v1alpha1
kind: ScanJob
metadata:
  name: scanjob-example
  namespace: default
spec:
  registry: example-registry # Name of the Registry resource (in the same namespace) to be scanned
```

### ScanJob status

The `ScanJob` resource will include a status field to reflect the scan's progress and outcome. This status will contain:

- `conditions`: Represents detailed job conditions, similar to those used in Kubernetes Jobs, showing whether the scan completed successfully or encountered issues (`Complete`, `Failed`).
- `imagesCount`: The number of images found in the registry during the scan.

Please refer to the [Kubernetes API conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties) for more information about the status conditions.

Example status:

```yaml
status:
  imagesCount: 100
  conditions:
    - type: Complete
      status: "False"
      reason: "Processing"
      message: "Job in progress"
    - type: Failed
      status: "False"
      reason: "Processing
      message: "Job in progress"
```

### Validation

Only one `ScanJob` can run against a `Registry` at a time. If a `ScanJob` is already in progress for a `Registry`, creating another one will be rejected.
A `ValidatingWebhook` enforces this by checking existing `ScanJob` resources in the same namespace to ensure no conflicts occur.

### Reconciler

A `ScanJob` reconciler will be introduced to handle and manage the entire lifecycle of `ScanJob` resources.

## Triggering Scans flow

When a `ScanJob` is created, the following sequence of actions is triggered:

1. **The `ScanJob` reconciler** fetches the referenced `Registry` resource.
2. **If the Registry is not found**, the reconciler marks the `ScanJob` as `Failed` with an appropriate message.
3. **The ScanJob reconciler** adds the serialized `Registry` resource as an annotation on the `ScanJob`. This ensures the scan uses a consistent snapshot of the registry configuration.
4. **The ScanJob reconciler** sends a message on the NATS queue to trigger the scan workflow.
5. **The ScanJob reconciler** updates the `ScanJob` status to `InProgress`.
6. **A worker** receives the message and starts the discovery process.
7. **The worker** discovers images in the registry.
8. **The worker** updates the `ScanJob` status field `ImagesCount` with the number of images found.
9. **The worker** sends a NATS message for each image to trigger SBOM generation.
10. **Workers** generate SBOMs and send messages to initiate vulnerability scans.
11. **Workers** create a `VulnerabilityReport` resource for each image with the scan results.
12. **The `VulnerabilityReport` reconciler** monitors the number of `VulnerabilityReport` resources and, once it matches `ImagesCount`, marks the `ScanJob` as `Complete`.

This design simplifies the architecture by retaining only the `ScanJob` and `VulnerabilityReport` reconcilers.
Unlike the previous model, where the `Image` and `SBOM` reconcilers coordinated different stages of the scan, the worker now directly publishes follow-up jobs (e.g., SBOM generation, vulnerability scan) to the queue.
This reduces the number of Kubernetes API interactions and streamlines the scanning workflow.

## Error handling

- Transient errors encountered during the scan process (such as network problems or registry downtime) will be automatically retried by both reconcilers and workers. Workers will use exponential backoff for these retries. If the scan continues to fail after multiple attempts, the `ScanJob` will be marked as `Failed` with a relevant error message.
- For non-transient errors (like an invalid registry configuration), the `ScanJob` will be marked as `Failed` immediately, accompanied by a clear error message.

## Registry deletion

A finalizer will be added to the `Registry` resource to guarantee that deletion only proceeds once any ongoing `ScanJob` has either completed or failed.
This ensures that scans are not interrupted mid-process, preserving the integrity of scan results and preventing orphaned resources.

## Garbage collection

A maximum of X ScanJob resources per registry will be retained in the system for auditing and historical purposes, with X being a configurable value.
This logic could be effectively implemented within either the `ValidatingWebhook` or the `ScanJob` reconciler.

## Scheduled scans

The scan frequency is set in the `Registry` resource via the `spec.scanInterval` field.
A new [`Runnable`](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/manager#Runnable) will be implemented to regularly trigger scans for all registries.
Using a ticker, the runnable will periodically examine each `Registry`’s `spec.scanInterval` and create a `ScanJob` if the time since the last scan exceeds the configured interval.
This allows us to use the same resource and reonciliation logic for both manual and scheduled scans, simplifying the architecture.

# Drawbacks

[drawbacks]: #drawbacks

<!---
Why should we **not** do this?

  * obscure corner cases
  * will it impact performance?
  * what other parts of the product will be affected?
  * will the solution be hard to maintain in the future?
--->

# Alternatives

[alternatives]: #alternatives

<!---
- What other designs/options have been considered?
- What is the impact of not doing this?
--->

# Unresolved questions

[unresolved]: #unresolved-questions

<!---
- What are the unknowns?
- What can happen if Murphy's law holds true?
-
