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

The `ScanJob` resource includes a status field to track the scan's lifecycle and progress. The status contains:

- `conditions`: Four condition types that track the job's state progression:
  - `Scheduled`: Indicates the job has been accepted and scheduled for execution
  - `InProgress`: Shows the job is actively running
  - `Complete`: Indicates successful completion
  - `Failed`: Indicates the job encountered an error and failed
- `imagesCount`: Total number of images discovered in the target registry
- `scannedImagesCount`: Number of images that have been successfully scanned
- `startTime`: Timestamp when the job began processing (set when transitioning to InProgress)
- `completionTime`: Timestamp when the job finished (either successfully or with failure)

Please refer to the [Kubernetes API conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties) for more information about status conditions.

#### Example status progression

Job scheduled but not yet started:

```yaml
status:
  imagesCount: 0
  scannedImagesCount: 0
  conditions:
    - type: Scheduled
      status: "True"
      reason: "Scheduled"
      message: "ScanJob is scheduled"
    - type: InProgress
      status: "False"
      reason: "Scheduled"
      message: "ScanJob is scheduled"
    - type: Complete
      status: "False"
      reason: "Scheduled"
      message: "ScanJob is scheduled"
    - type: Failed
      status: "False"
      reason: "Scheduled"
      message: "ScanJob is scheduled"
```

Job creating catalog:

```yaml
status:
  imagesCount: 0
  scannedImagesCount: 0
  startTime: "2024-01-15T10:30:00Z"
  conditions:
    - type: Scheduled
      status: "False"
      reason: "InProgress"
      message: "ScanJob is in progress"
    - type: InProgress
      status: "True"
      reason: "CatalogCreationInProgress"
      message: "Catalog creation in progress"
    - type: Complete
      status: "False"
      reason: "InProgress"
      message: "ScanJob is in progress"
    - type: Failed
      status: "False"
      reason: "InProgress"
      message: "ScanJob is in progress"
```

Job generating SBOMs

```yaml
status:
  imagesCount: 100
  scannedImagesCount: 0
  startTime: "2024-01-15T10:30:00Z"
  conditions:
    - type: Scheduled
      status: "False"
      reason: "InProgress"
      message: "ScanJob is in progress"
    - type: InProgress
      status: "True"
      reason: "SBOMGenerationInProgress"
      message: "SBOM generation in progress"
    - type: Complete
      status: "False"
      reason: "InProgress"
      message: "ScanJob is in progress"
    - type: Failed
      status: "False"
      reason: "InProgress"
      message: "ScanJob is in progress"
```

Job scanning images

```yaml
status:
  imagesCount: 100
  scannedImagesCount: 45
  startTime: "2024-01-15T10:30:00Z"
  conditions:
    - type: Scheduled
      status: "False"
      reason: "InProgress"
      message: "ScanJob is in progress"
    - type: InProgress
      status: "True"
      reason: "ImageScanInProgress"
      message: "Image scan in progress"
    - type: Complete
      status: "False"
      reason: "InProgress"
      message: "ScanJob is in progress"
    - type: Failed
      status: "False"
      reason: "InProgress"
      message: "ScanJob is in progress"
```

Job completed successfully:

```yaml
status:
  imagesCount: 100
  scannedImagesCount: 100
  startTime: "2024-01-15T10:30:00Z"
  completionTime: "2024-01-15T11:45:00Z"
  conditions:
    - type: Scheduled
      status: "False"
      reason: "Complete"
      message: "ScanJob completed successfully"
    - type: InProgress
      status: "False"
      reason: "Complete"
      message: "ScanJob completed successfully"
    - type: Complete
      status: "True"
      reason: "AllImagesScanned"
      message: "All images scanned successfully"
    - type: Failed
      status: "False"
      reason: "Complete"
      message: "ScanJob completed successfully"
```

Job completed with no images to scan:

```yaml
status:
  imagesCount: 0
  scannedImagesCount: 0
  startTime: "2024-01-15T10:30:00Z"
  completionTime: "2024-01-15T10:35:00Z"
  conditions:
    - type: Scheduled
      status: "False"
      reason: "Complete"
      message: "ScanJob completed successfully"
    - type: InProgress
      status: "False"
      reason: "Complete"
      message: "ScanJob completed successfully"
    - type: Complete
      status: "True"
      reason: "NoImagesToScan"
      message: "No images to process"
    - type: Failed
      status: "False"
      reason: "Complete"
      message: "ScanJob completed successfully"
```

Job failed:

```yaml
status:
  imagesCount: 0
  scannedImagesCount: 0
  startTime: "2024-01-15T10:30:00Z"
  completionTime: "2024-01-15T10:32:00Z"
  conditions:
    - type: Scheduled
      status: "False"
      reason: "Failed"
      message: "ScanJob failed"
    - type: InProgress
      status: "False"
      reason: "Failed"
      message: "ScanJob failed"
    - type: Complete
      status: "False"
      reason: "Failed"
      message: "ScanJob failed"
    - type: Failed
      status: "True"
      reason: "InternalError"
      message: "Failed to create catalog: registry connection timeout"
```

### Validation

Only one `ScanJob` can run against a `Registry` at a time. If a `ScanJob` is already in progress for a `Registry`, creating another one will be rejected.
A `ValidatingWebhook` enforces this by checking existing `ScanJob` resources in the same namespace to ensure no conflicts occur.

### Reconciler

A `ScanJob` reconciler will be introduced to handle and manage the entire lifecycle of `ScanJob` resources.

## Triggering Scans flow

When a `ScanJob` is created, the following sequence of actions is triggered:

1. **The `ScanJob` reconciler** fetches the referenced `Registry` resource.
2. **If the Registry is not found**, the reconciler marks the `ScanJob` as `Failed` with reason `RegistryNotFound`.
3. **The ScanJob reconciler** adds the serialized `Registry` resource as an annotation on the `ScanJob`. This ensures the scan uses a consistent snapshot of the registry configuration.
4. **The ScanJob reconciler** marks the `ScanJob` as `Scheduled` and sends a message on the NATS queue to trigger the scan workflow.
5. **A catalog creation worker** receives the message and marks the `ScanJob` as `InProgress` with reason `CatalogCreationInProgress`.
6. **The catalog worker** discovers images in the registry and creates `Image` resources for each discovered image.
7. **The catalog worker** updates the `ScanJob` status field `ImagesCount` with the number of images found.
8. **If no images are found**, the worker marks the `ScanJob` as `Complete` with reason `NoImagesToScan`.
9. **If images are found**, the worker marks the `ScanJob` as `InProgress` with reason `SBOMGenerationInProgress` and sends a NATS message for each image to trigger SBOM generation.
10. **SBOM generation workers** process each image and send messages to initiate vulnerability scans.
11. **Vulnerability scan workers** create a `VulnerabilityReport` resource for each image with the scan results.
12. **The `VulnerabilityReport` reconciler** monitors the number of `VulnerabilityReport` resources, updates the `ScannedImagesCount` field, and marks the `ScanJob` as `InProgress` with reason `ImageScanInProgress` while scanning is ongoing.
13. **Once `ScannedImagesCount` matches `ImagesCount`**, the reconciler marks the `ScanJob` as `Complete` with reason `AllImagesScanned`.
14. **If any step fails**, the failure handler marks the `ScanJob` as `Failed` with reason `InternalError` and the specific error message.

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

## Periodic scans

The scan frequency is set in the `Registry` resource via the `spec.scanInterval` field.
A new [`Runnable`](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/manager#Runnable) will be implemented to regularly trigger scans for all registries.
Using a ticker, the runnable will periodically examine each `Registry`’s `spec.scanInterval` and create a `ScanJob` if the time since the last scan exceeds the configured interval.
This allows us to use the same resource and reonciliation logic for both manual and periodic scans, simplifying the architecture.
If `spec.scanInterval` is not set, the registry will not be scanned automatically.

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
--->
