# SBOMbastic Uninstall

You can remove the resources created by uninstalling the `helm` chart as follows:

```bash
helm uninstall --namespace sbombastic sbombastic
```

Then remove the following Custom Resource Definitions, this will also delete
all the resources of these types declared inside of the cluster:

```bash
kubectl delete crd vexhubs.sbombastic.rancher.io
kubectl delete crd scanjobs.sbombastic.rancher.io
kubectl delete crd registries.sbombastic.rancher.io
```

Finally, delete the namespace where SBOMbastic was deployed:

```bash
kubectl delete ns sbombastic
```

This will remove the Persistent Volume Claims and their associated
Persistent Volumes.
