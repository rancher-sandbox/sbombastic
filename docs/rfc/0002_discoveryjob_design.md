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
  annotations:
    sbombastic.rancher.io/last-discovered-image-name: b4af50c685838e4d4c31e6dff2ad24bd8429110b71886cfe5685759ac0712fa0
    sbombastic.rancher.io/last-discovery-completed: "false"
    sbombastic.rancher.io/last-discovery-completed-at: ""
    sbombastic.rancher.io/last-discovery-started-at: "2025-05-08T15:52:20Z"
    sbombastic.rancher.io/last-job-name: registry-example-discovery-795c63c4-11
    sbombastic.rancher.io/last-job-type: discovery
spec:
  uri: "https://registry-1.docker.io"
  discoveryJob:  ## for scheduled discovery
    cron:
      dayOfWeek: # optional, 0~6
      month: # optional, 1~12
      dayOfMonth: # optional, 1~31
      hour: # optional, 0~23
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

A RegistryDiscovery represents a discovery operation that can be triggered manually or by schedule.
It tracks the status condition of the discovery operation.

```yaml
apiVersion: scanner.rancher.io/v1alpha1
kind: RegistryDiscovery
metadata:
  name: registry-example-1742868000
  namespace: default
spec:
  registry: registry-example
  registrySpec: <copy of registry.Spec>
status:
  canceled: false
  conditions:
  - lastTransitionTime: "2025-05-08T15:52:20Z"
    message: Registry discovery in progress
    reason: DiscoveryRunning
    status: "True"
    type: Discovering
  - lastTransitionTime: "2025-05-08T15:52:21Z"
    message: Registry discovery finished
    reason: DiscoveryFinished
    status: "True"
    type: Discovered
  currentStatus: Succeeded
  finishedAt: "2025-05-08T15:52:21Z"
  startedAt: "2025-05-08T15:52:20Z"
```


1. User creates a Registry object manually or thru Rancher UI. If the Registry doesn't have a discovery schedule specified, default schedule 0:xx AM everyday is applied on the fly.
   (scheduler checks every 15 minutes. So there could be 0~15 minutes delay)

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
   When a RegistryDiscovery CR is reconciled, controller will add a CreateCatalog message to the NATs queue and update the RegistryDiscovery CR's Status.CurrentStatus as "Pending". 
   The CreateCatalog message contains namespace/name of the Registry and name of the RegistryDiscovery. 

   NATS options DiscardNewPerSubject/DiscardNew/MaxMsgsPerSubject do not fit our need well for preventing duplicate jobs post to NATS queue:
   - Number of Registry objects is variant
   - It doesn't work well in our case when a new CreateCatalog message is published to an empty NATS queue while worker is still doing registry discovery (redundant discovery concurrently)
   - It doesn't work well in our case when a new CreateCatalog message is published to an empty NATS queue while worker is still doing registry discovery (redundant discovery concurrently)
   
6. Worker will pick up the CreateCatalog message and then get the Registry CR from the API server.
   
7. It's worker to make sure that for each registry no other discovery operation could be triggered before the last discovery operation is completed.
   Worker will leverage k8s lease to do this. The lease should be taken on a per-registry basis.
   (The lease name is in the format "lease-{registry namespace}-{registry name}" )
   All such k8s leases are created in {sbombastic| namespace.

8. After the leaseLock is acquired, worker will add annotations like below to the Registry CR:
    - sbombastic.rancher.io/last-discovery-completed: "false"
    - sbombastic.rancher.io/last-discovery-completed-at: ""
    - sbombastic.rancher.io/last-discovery-started-at: "2025-05-08T15:52:20Z"
    - sbombastic.rancher.io/last-job-name: registry-example-discovery-795c63c4-11
    - sbombastic.rancher.io/last-job-type: discovery

9. Controller and worker update RegistryDiscovery.Status.CurrentStatus and RegistryDiscovery.Status.Conditions to keep track of the progress of the registry discovery.
   These are the possible Status.CurrentStatus values:
    - "Pending"
	- "Running"
	- "FailStopped"
	- "Cancel"
	- "Succeeded"

   These are the possible Status.Condition.Type values:
    - "Discovering"
    - "Discovered"

   These are the possible Status.Condition.Reason values:
    - "FailedToRequestDiscovery"
	- "DiscoveryPending"
	- "DiscoveryRunning"
	- "DiscoveryFailed"
	- "DiscoveryFinished"

10. After the last Image CR is created, worker updates RegistryDiscovery.Status to "Succeeded".

11. After the last SBOM CR is reconciled(i.e. when the # of selected Image CRs equal to the # of selected SBOM CRs), controller updates Registry's annotation:
    - sbombastic.rancher.io/last-discovery-completed: "true"
    - sbombastic.rancher.io/last-discovery-completed-at: "2025-05-08T15:52:20Z"


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
1. After the SBOM CR of the last qualified image's SBOM is reconciled? (Currently this is the chosen one)
2. After the VulnerabilityReport CR of the last qualified image is created? 


