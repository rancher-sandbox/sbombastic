# Collect logs for debugging

## Install

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.18.2/cert-manager.yaml
helm repo add sbombastic https://rancher-sandbox.github.io/sbombastic/
helm repo update
```

## Install or upgrade an existing installation with debug logging activated

```bash
helm upgrade --install sbombastic sbombastic/sbombastic \
  --set=worker.logLevel=debug \
  --set=controller.logLevel=debug
```

## Verify installation

Check the version. It should match the latest `sbombastic-chart-*` version found in the [releases](https://github.com/rancher-sandbox/sbombastic/releases) page.

```bash
helm list
```

Wait for pods to be running:

```bash
kubectl get pods
```

## Collect the logs

Using [the script found in the sbombastic repository](https://github.com/rancher-sandbox/sbombastic/blob/main/hack/sbombastic-debug.sh):

```bash
./hack/sbombastic-debug.sh collect --compress-results
```

Upload the generated tar.gz file.
