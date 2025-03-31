|              |                                                          |
| :----------- | :------------------------------------------------------- |
| Feature Name | Discovery design                                         |
| Start Date   | Mar 20th, 2025                                           |
| Category     | controller, CRD                                          |
| RFC PR       | https://github.com/rancher-sandbox/sbombastic/pull/140   |
| State        | ACCEPTED                                                 |

# Summary

[summary]: #summary

Add RegistryDiscovery which represents a discovery operation that can be triggered by user or by schedule.

# Motivation

Currently when a Registry CR is reconciled by controller, the annotation `sbombastic.rancher.io/last-discovered-at` in 
the Registry CR is checked. Discovery operation is triggered when this annotation is empty. 
When controller reconciles a SBOM CR and sees the number of Image CRs is equal to the number of SBOM CRs for the Registry, 
annotation `sbombastic.rancher.io/last-discovered-at` is set as current time in the owner Registry CR.
It means no new image could be discovered anymore after a registry is discovered unless the annotation 
`sbombastic.rancher.io/last-discovered-at` is cleared in the Registry CR.

The purpose of this RFC is to design a mechanism(`RegistryDiscovery`) that a discovery operation could be triggered on demand or 
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
spec:
  uri: "https://registry-1.docker.io"
  discoveryJob:  ## for scheduled discovery
    schedule: "0 1 * * *"
    suspend: false # bool value, toggle the scheduled discovery
    failedJobsHistoryLimit: 2 # number of RegistryDiscovery objects with failed state to keep
    successfulJobsHistoryLimit: 1 # number of RegistryDiscovery objects with successful state to keep
  repositories: # optional, if not specified, scan all repositories
    - "repo1"
    - "repo2"
status:
  - lastScheduledTime: "2025-03-24T01:30:00Z"

```

### RegistryDiscovery

A RegistryDiscovery represents a discovery operation that can be triggered manually.
It tracks the status condition of the discovery operation.

```yaml
apiVersion: scanner.rancher.io/v1alpha1
kind: RegistryDiscovery
metadata:
  name: registry-example-1742868000 # <Registry name>-<unix timestamp>
  namespace: default
  labels:
    sbombastic.rancher.io/completed: "true"
    sbombastic.rancher.io/cron-registry-discovery: registry.local.lan
    sbombastic.rancher.io/phase: Succeeded # Could be "Pending", "Running", "Succeeded", "Failed", "Error" # similar to https://github.com/argoproj/argo-workflows/blob/8098a14a2f5dc134a2e21e5a6e6b23b4b76e0e21/pkg/apis/workflow/v1alpha1/workflow_phase.go#L6-L13
spec:
  registrySpec: <copy of registry.Spec>
status:
  - startedAt: "2025-03-24T01:30:00Z"
  - finishedAt: "2025-03-24T01:32:00Z"
  - conditions: # we can use conditions to provide feedback about the progress
    - status: "False"
      type: Running
    - status: "True"
      type: Completed
```


1. User creates a Registry object manually or thru Rancher UI. If the Registry doesn't have a discovery schedule specified, default schedule is applied by Registry CRD schema definition.

2. When a Registry CR is reconciled, the registry's configurations are updated in controller's cache.

3. A new reconciliation loop, running only on the leader of the controllers, will handle an internal clock and create instances of RegistryDiscovery whenever needed(by schedule) for the cached Registry instances.
   The reconciliation loop will also regularly purge the old RegistryDiscovery objects associated with each given Registry to ensure the failedJobsHistoryLimit and successfulJobsHistoryLimit limits are respected.
   RunnableFunc from controller-runtime will be used for the reconciliation loop.
   The created RegistryDiscovery CR needs to be in the same namespace as the target registry.

4. There are 2 ways to trigger manual discovery: 
   - Users click "Discover Now" on Rancher UI registry page. Rancher UI will create a RegistryDiscovery CR accordingly.
   - Users create a RegistryDiscovery CR in k8s.
   The created RegistryDiscovery CR needs to be in the same namespace as the target registry.
   
5. Each RegistryDiscovery CR is for a registry discovery occurrence.
   When a RegistryDiscovery CR is reconciled, controller will add a DiscoverRegistry job to the NATs queue. 
   The DiscoverRegistry job object can simply reference the RegistryDiscovery object instance. 

   NATS options DiscardNewPerSubject/DiscardNew/MaxMsgsPerSubject do not fit our need well for preventing duplicate jobs post to NATS queue:
   - Number of Registry objects is variant
   - It doesn't work well in our case when a new DiscoverRegistry job is published to an empty NATS queue while worker is still doing registry discovery (redundant discovery concurrently)
   
6. Worker will pick up the DiscoverRegistry job and then get the RegistryDiscovery CR from the API server and start the create catalog job.
   
7. Worker will keep the RegistryDiscovery object up-to-date to keep track of the progress of the catalog and its final outcome.

8. It's worker to make sure that for each registry no other discovery operation could be triggered before the last discovery operation is completed.
   Worker will leverage k8s lease to do this. The lease should be taken on a per-registry basis.
   However, we need to consider about the namespace these lease resource instances are in.
   - Do we want worker to have RBAC for k8s lease on all namespaces? (this might be a concern about this)
   - Or we operate all such kind of leases in {sbombastic} namespace? (notice: by design Kubernetes does not support cross-namespace owner references)
   
9. After worker starts the create catalog job, it updates the RegistryDiscovery CR status when iterating thru the listed images.
   
10. When SBOMReconciler reconciles a SBOM CR & sees len(SBOMs) == len(Images), it means the registry discovery is completed.
    Controller will update the RegistryDiscovery CR status accordingly & delete the k8s lease object for the registry.
   
11. May need to add more fields(like ObservedGeneration) to RegistryDiscovery.Status for internal handling

------


# Drawbacks

[drawbacks]: #drawbacks

1. The discovery-job-{registry.name} RegistryDiscovery CR in every namespace needs to be reserved for scheduled discovery status.
2. Let's say after a Registry is created by user(& the RegistryDiscovery CR is created by sbombastic), user clicks "Discover Now". 
   For sbombastic to reconcile the RegistryDiscovery CR for "Discover Now", RegistryDiscovery.metadata.spec needs to be changed.
   However, currently RegistryDiscovery.metadata.spec contains only registry name which probably never changes after a RegistryDiscovery CR is created.
   But we don't want Rancher UI to delete and then create the reserved RegistryDiscovery CR for submitting "Discover Now" because 
   the status of the last scheduled discovery will be lost.
   One consideration is to add RegistryDiscovery.Spec.ManualGUID (string):
   - REST client(ex. Rancher UI) could specify a different RegistryDiscovery.Spec.ManualGUID value in every "Discover Now" request(PATCH) to make sure
     it will be reconciled by sbombastic.
   - If we think REST client may not remember to specify a different RegistryDiscovery.Spec.ManualGUID value in every "Discover Now" PATCH request,
     sbombastic could change the RegistryDiscovery.Spec.ManualGUID value when it reconciles the RegistryDiscovery CR so that REST client can 
     keep sending the PATCH request without caring about the RegistryDiscovery.Spec.ManualGUID value.


# Alternatives

[alternatives]: #alternatives



# Unresolved questions

[unresolved]: #unresolved-questions

Q: When do we say a discovery job is "completed"? Does it mean 
1. After the SBOM CR of the last qualified image is created?
2. After the VulnerabilityReport CR of the last qualified image is created? 


