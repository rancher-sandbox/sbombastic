|              |                                                          |
| :----------- | :------------------------------------------------------- |
| Feature Name | Discovery design                                         |
| Start Date   | Mar 20th, 2025                                           |
| Category     | controller, CRD                                          |
| RFC PR       | https://github.com/rancher-sandbox/sbombastic/pull/140   |
| State        | ACCEPTED                                                 |

# Summary

[summary]: #summary

Add DiscoveryJob which represents a discovery operation that can be triggered by user or by schedule.

# Motivation

Currently when a Registry CR is reconciled by controller, the annotation `sbombastic.rancher.io/last-discovered-at` in 
the Registry CR is checked. Discovery operation is triggered when this annotation is empty. 
When controller reconciles a SBOM CR and sees the number of Image CRs is equal to the number of SBOM CRs for the Registry, 
annotation `sbombastic.rancher.io/last-discovered-at` is set as current time in the owner Registry CR.
It means no new image could be discovered anymore after a registry is discovered unless the annotation 
`sbombastic.rancher.io/last-discovered-at` is cleared in the Registry CR.

The purpose of this RFC is to design a mechanism(`DiscoveryJob`) that a discovery operation could be triggered on demand or 
by schedule.
We also need to prevent new discovery operations from being triggered when there is an discovery operation running.


## Examples / User Stories

[examples]: #examples

- As a user, I want to trigger the "discovery operation" of a Registry on demand (ex. "Discover Now" on Rancher UI)
- As a user, I want to configure the discovery schedule of a Registry so the "discovery operation" could be triggered automatically by controller.


# Detailed design

[design]: #detailed-design

## CRD

The following CRDs will be extended or added to the cluster.

### Registry

Registry represents a registry to be scanned. It contains the registry URL, the name of the secret containing auth credentials, the repositories to be scanned.
It also contains settings of the discovery and scan schedules.

```yaml
apiVersion: scanner.rancher.io/v1alpha1
kind: Registry
metadata:
  name: registry-example
  namespace: default
  annotations:
    sbombastic.rancher.io/last-discovery-type:
    sbombastic.rancher.io/last-discovery-started-at:
    sbombastic.rancher.io/last-discovered-at
    sbombastic.rancher.io/last-discoveryjob-name:
    sbombastic.rancher.io/last-discoveryjob-generation
    sbombastic.rancher.io/last-discoveryjob-uid
    sbombastic.rancher.io/running-discovery-type:
    sbombastic.rancher.io/running-discovery-started-at
    sbombastic.rancher.io/running-discoveryjob-name:
    sbombastic.rancher.io/running-discoveryjob-generation
    sbombastic.rancher.io/running-discoveryjob-uid
spec:
  uri: "https://registry-1.docker.io"
  discoverySchedule:  ## for scheduled discovery
    cronSchedule: # multiple schedules are supported
      - "0 1 * * *"
    discoveryPeriod: "1h" # discovery new images every 1 hour
  scanSchedule:  ## for scheduled scan (not covered by this doc)
    scanPeriod: "1d" # scan images every day
  dbChangedRescan: false
  repositories: # optional, if not specified, scan all repositories
    - "repo1"
    - "repo2"
```

### DiscoveryJob

A DiscoveryJob represents a discovery operation that can be triggered manually.
It tracks the status condition of the discovery operation.

```yaml
apiVersion: scanner.rancher.io/v1alpha1
kind: DiscoveryJob
metadata:
  name: discovery-job-example
  namespace: namespace-1
spec:
  registry: registry-example # registry name
status:
  conditions:
    - type: "Progressing"
      status: "True"
      reason: "Finished"
      ...
    - type: "Completed"
      status: "True"
      reason: "Succeeded"
      ...
```


1. On Rancher UI the schedule setting should be configured on the Registry page.

2. When a Registry CR is reconciled, a DiscoveryJob CR named(by default) discovery-job-{registry.name} is created for storing the scheduled discovery status.
   Before a new scheduled discovery operation is triggered, status in this DiscoveryJob CR is purged first

3. Controller caches the scheduled discovery settings of all registries.
   When a Registry CR is reconciled, controller updates this cache accordingly.
   There is a scheduler go-routine wakes up per-minute and checks the scheduled discovery settings of all registries.

4. There are 2 ways to trigger manual discovery: 
   - Users click "Discover Now" on Rancher UI registry page. Rancher UI will create/update a DiscoveryJob CR accordingly.
   - Users create a DiscoveryJob CR in k8s.
   The created DiscoveryJob CR needs to be in the same namespace as the target registry.

5. When a DiscoveryJob CR is reconciled for manual disovery operation, controller will delete all existing, except the reserved discovery-job-{registry.name}, DiscoveryJob CRs 
   in the same namespace.

6. It's controller's responsibility to make sure no new discovery operation could be triggered before the last discovery operation is completed 
   (with timeout mitigation in case the last discovery operation never update its status correctly)
   CreateCatalog message is published to NATS server only when `sbombastic.rancher.io/running-discovery-type` is empty in Registry CR's annotation.
   If discovery operation is running when the DiscoveryJob CR is reconciled, the DiscoveryJob CR's status is updated without triggering a new discovery operation.
   
   Before publishing CreateCatalog message to NATS server, these annotations in the Registry CR are set:
   - `sbombastic.rancher.io/running-discovery-type`             : set to "manual" or "scheduled" based on what triggers this discovery operation
   - `sbombastic.rancher.io/running-discovery-started-at`       : set with current time
   - `sbombastic.rancher.io/running-discoveryjob-name`          : set with DiscoveryJob CR's .metadata.name value if it's manual discovery
   - `sbombastic.rancher.io/running-discoveryjob-generation`    : set with DiscoveryJob CR's .metadata.generation value if it's manual discovery
   - `sbombastic.rancher.io/running-discoveryjob-uid`           : set with DiscoveryJob CR's .metadata.uid value if it's manual discovery
   
   CreateCatalog message will be expanded to contain:
   - Type of the Discovery request
   - Name of the DiscoveryJob CR that requests for discovery

8. After worker picks up CreateCatalog message from NATS queue, it starts the discovery accordingly.   
   Worker updates the DiscoveryJob CR status when iterating thru the listed images.
   - If it's manual discovery, the status in the DiscoveryJob CR that triggers manual discovery is updated.
   - If it's scheduled discovery, the status in the reserved discovery-job-{registry.name} DiscoveryJob CR is updated.
   
9. When a discovery operation is completed, these annotations in the Registry CR are set:
   - `sbombastic.rancher.io/last-discovered-at`                 : set with current time.
   - `sbombastic.rancher.io/last-discovery-type`                : set with value of `sbombastic.rancher.io/running-discovery-type`
   - `sbombastic.rancher.io/last-discovery-started-at`          : set with value of `sbombastic.rancher.io/running-discovery-started-at`
   - `sbombastic.rancher.io/last-discoveryjob-name`             : set with value of `sbombastic.rancher.io/running-discovery-name`
   - `sbombastic.rancher.io/last-discoveryjob-generation`       : set with value of `sbombastic.rancher.io/running-discovery-generation`
   - `sbombastic.rancher.io/last-discoveryjob-uid`              : set with value of `sbombastic.rancher.io/running-discovery-uid`
   - `sbombastic.rancher.io/running-discovery-type`             : cleared
   - `sbombastic.rancher.io/running-discovery-started-at`       : cleared
   - `sbombastic.rancher.io/running-discoveryjob-name`          : cleared
   - `sbombastic.rancher.io/running-discoveryjob-generation`    : cleared
   - `sbombastic.rancher.io/running-discoveryjob-uid`           : cleared

10. May need to add more fields(like ObservedGeneration) to DiscoveryJob.Status for internal handling

------


# Drawbacks

[drawbacks]: #drawbacks

1. The discovery-job-{registry.name} DiscoveryJob CR in every namespace needs to be reserved for scheduled discovery status.
2. Let's say after a Registry is created by user(& the DiscoveryJob CR is created by sbombastic), user clicks "Discover Now". 
   For sbombastic to reconcile the DiscoveryJob CR for "Discover Now", DiscoveryJob.metadata.spec needs to be changed.
   However, currently DiscoveryJob.metadata.spec contains only registry name which probably never changes after a DiscoveryJob CR is created.
   But we don't want Rancher UI to delete and then create the reserved DiscoveryJob CR for submitting "Discover Now" because 
   the status of the last scheduled discovery will be lost.
   One consideration is to add DiscoveryJob.Spec.ManualGUID (string):
   - REST client(ex. Rancher UI) could specify a different DiscoveryJob.Spec.ManualGUID value in every "Discover Now" request(PATCH) to make sure
     it will be reconciled by sbombastic.
   - If we think REST client may not remember to specify a different DiscoveryJob.Spec.ManualGUID value in every "Discover Now" PATCH request,
     sbombastic could change the DiscoveryJob.Spec.ManualGUID value when it reconciles the DiscoveryJob CR so that REST client can 
     keep sending the PATCH request without caring about the DiscoveryJob.Spec.ManualGUID value.


# Alternatives

[alternatives]: #alternatives



# Unresolved questions

[unresolved]: #unresolved-questions

Q: When do we say a discovery job is "completed"? Does it mean 
1. After the SBOM CR of the last qualified image is created?
2. After the VulnerabilityReport CR of the last qualified image is created? 


