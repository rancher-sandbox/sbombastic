# Collecting logs for debugging

## Install dependencies

Please follow the instructions in the [quickstart guide](../installation/quickstart.md#requirements) to ensure you have the necessary dependencies installed.

## Install or upgrade an existing installation with debug logging activated

```bash
helm repo add kubewarden https://charts.kubewarden.io
helm repo update
helm upgrade --install sbomscanner kubewarden/sbomscanner \
  --set=worker.logLevel=debug \
  --set=controller.logLevel=debug
  --namespace sbomscanner \
  --create-namespace \
  --wait
```

## Verify installation

Check the version. It should match the latest `sbomscanner-chart-*` version found in the [releases](https://github.com/kubewarden/sbomscanner/releases) page.

```bash
helm list
```

Wait for pods to be running:

```bash
kubectl get pods
```

## Collect the logs

Using [the script found in the sbomscanner repository](https://github.com/kubewarden/sbomscanner/blob/main/hack/sbomscanner-debug.sh):

```bash
./hack/sbomscanner-debug.sh collect --compress-results
```

Upload the generated tar.gz file.
