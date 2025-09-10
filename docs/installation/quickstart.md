# SBOMbastic Quick Start

Welcome to the SBOMbastic Quick Start!

This guide will walk you through the following steps:

- Deploying the SBOMbastic stack in a Kubernetes cluster
- Running an automated image scan using a `Registry` custom resource

---

## Requirements

Before deployment, you need to prepare the following:

- A Kubernetes cluster (you can simply run a [kind](https://kind.sigs.k8s.io/) cluster)
- `helm` installed locally
- `kubectl` installed locally
- `cert-manager` installed in the cluster

To install cert-manager, you can run the following commands:

```bash
helm repo add jetstack https://charts.jetstack.io

helm repo update

helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true \
  --wait
```

> For more information on configuring cert-manager, please visit the [cert-manager documentation](https://cert-manager.io/docs/installation/helm)

---

## Deploy SBOMbastic

Follow these simple steps from your local machine to get SBOMbastic up and running:

### Install the Helm chart

```bash
helm repo add sbombastic https://rancher-sandbox.github.io/sbombastic
helm repo update
helm install sbombastic sbombastic/sbombastic \
  --namespace sbombastic \
  --create-namespace \
  --wait
```

### Verify the Deployment

After installation, ensure all pods are running:

```bash
kubectl get pods -n sbombastic
```

Example output:

```bash
sbombastic           sbombastic-controller-7f568c88dc-bmjgs       1/1     Running
sbombastic           sbombastic-controller-7f568c88dc-gcgbn       1/1     Running
sbombastic           sbombastic-controller-7f568c88dc-q7hbh       1/1     Running
sbombastic           sbombastic-nats-0                            2/2     Running
sbombastic           sbombastic-nats-1                            2/2     Running
sbombastic           sbombastic-nats-2                            2/2     Running
sbombastic           sbombastic-storage-5f596cd8f8-4t7z8          1/1     Running
sbombastic           sbombastic-worker-d9d68c5c-5dtck             1/1     Running
sbombastic           sbombastic-worker-d9d68c5c-qcp7n             1/1     Running
sbombastic           sbombastic-worker-d9d68c5c-tlpgm             1/1     Running
```

### Summary

At this point, your SBOMBastic deployment is up and running successfully. You're now ready to begin scanning images and generating reports!

---

## Run a Scan

In this section, you'll learn how to create a registry source and trigger an automated scan.

### Prepare a `registry.yaml` file

Before running a scan, you need to define a `Registry` custom resource for SBOMbastic to fetch images.

```yaml
apiVersion: sbombastic.rancher.io/v1alpha1
kind: Registry
metadata:
  name: test-registry
  namespace: default
spec:
  uri: ghcr.io
  repositories:
    - rancher-sandbox/sbombastic/test-assets/golang
```

### Create the Registry CR

```bash
kubectl apply -f registry.yaml
```

### Prepare a `scan-job.yaml`

The `ScanJob` CR tells SBOMbastic which registry to scan.

```yaml
apiVersion: sbombastic.rancher.io/v1alpha1
kind: ScanJob
metadata:
  name: test-scanjob
  namespace: default
spec:
  registry: test-registry
```

### Create a ScanJob CR

```bash
kubectl apply -f registry.yaml
```

### Wait for Results

Once the scan completes, check the generated SBOMs and vulnerability reports:

```bash
kubectl get sbom -n default
kubectl get vulnerabilityreport -n default
```

You should see output like:

```bash
NAME                                                               CREATED AT
2ca3e0b033d523509544cb6f31c626af2a710d7dbcc15cb9dffced2e4634d69b   2025-06-10T10:26:38Z
...
```

### Summary

You've successfully created a real-world Registry resource and triggered an automated scan.

You can jump to the [Querying reports](../user-guide/querying-reports.md) guide to learn how to query and inspect the generated images, SBOMs, and vulnerability reports.
