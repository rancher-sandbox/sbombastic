# Scan registries

This guide explains how to configure and run scans on container registries using SBOMbastic:

- Setting up a `Registry` custom resource
- Triggering scans using the `ScanJob` custom resource
- Configuring periodic scans
- Checking the status of scans

# Setup a Registry

Before you can scan a registry, you need to create a `Registry` custom resource that defines the target registry and scan parameters.

Here is an example of a `Registry` manifest:

```yaml
apiVersion: sbombastic.rancher.io/v1alpha1
kind: Registry
metadata:
  name: my-registry
  namespace: default
spec:
  uri: ghcr.io
  scanInterval: 1h
  repositories:
    - rancher-sandbox/sbombastic/test-assets/golang
```

This configuration sets up a registry located at `ghcr.io` and specifies that it should scan the `rancher-sandbox/sbombastic/test-assets/golang` repository every hour.
A new scan will be triggered automatically after the resource is created.

Apply the configuration using kubectl:

```bash
kubectl apply -f registry.yaml
```

To configure authentication for private registries, refer to the [Private Registries guide](./private-registries.md).

# Trigger a scan manually

Create a ScanJob without `scanInterval` to disable periodic scans:

```yaml
apiVersion: sbombastic.rancher.io/v1alpha1
kind: Registry
metadata:
  name: my-registry
  namespace: default
spec:
  uri: ghcr.io
  repositories:
    - rancher-sandbox/sbombastic/test-assets/golang
```

To manually trigger a scan of the configured registry, create a `ScanJob` custom resource that references the `Registry`.

Here is an example of a `ScanJob` manifest:

```yaml
apiVersion: sbombastic.rancher.io/v1alpha1
kind: ScanJob
metadata:
  name: my-scanjob
  namespace: default
spec:
  registry: my-registry
```

Apply the `ScanJob` configuration:

```bash
kubectl apply -f scanjob.yaml
```

> [!NOTE]
> The `ScanJob` must be created in the same namespace as the `Registry` it references.

# Check the ScanJob status

You can monitor the status of the scan by checking the `ScanJob` resource:

```bash
kubectl get scanjob my-scanjob -n default -o yaml
```

The status section will show the progress and results of the scan, including the number of images processed:

```yaml
# ...
status:
  imagesCount: 10
  conditions:
    - type: Complete
      status: "True"
      reason: "AllImagesScanned"
      message: "Scan completed successfully"
# ...
```

# View Scan Results

Please reffer to the [Querying reports guide](./querying-reports.md) to learn how to query and view the generated images, SBOMs, and vulnerability reports.
