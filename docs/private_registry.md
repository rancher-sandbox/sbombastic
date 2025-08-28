# Private Registries

SBOMbastic supports private registries to scan for images. In order to make it work, please follow the steps listed below.

## Create the Secret

SBOMbastic relies on the docker `config.json` file to manage the authentication to the registries.

The first step to setup a private registry is to create a `Secret` with the `config.json` content, having the following structure:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-auth-secret 
  namespace: default
data:
  .dockerconfigjson: ewoJImF1dGhzIjogewoJCSJkZXYtcmVnaXN0cnkuZGVmYXVsdC5zdmMuY2x1c3Rlci5sb2NhbDo1MDAwIjogewoJCQkiYXV0aCI6ICJkWE5sY2pwd1lYTnpkMjl5WkE9PSIKCQl9Cgl9Cn0KCg==
type: kubernetes.io/dockerconfigjson
```

The `.dockerconfigjson` field is a base64 value, with the `config.json` content.

Here's an example:

```json
{
    "auths": {
        "myprivateregistry.example": {
            "auth": "dXNlcjpwYXNzd29yZA=="
        }
    }
}
```

For more info, please take a look to the Kubernetes [documentation](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/).

### Tip

Save the `config.json` into a file and use the following command to save it into the `Secret` file:

```sh
cat dockerconfig.json | base64 -w 0 | xclip -sel clipboard
```

## Create the Registry

Once your `Secret` is ready, you can reference it on the `Registry` configuration, specifying the name in the `Registry` field `spec.authSecret`.

```yaml
apiVersion: sbombastic.rancher.io/v1alpha1
kind: Registry
metadata:
  name: my-first-registry
  namespace: default
spec:
  uri: dev-registry.default.svc.cluster.local:5000
  scanInterval: 1h
  authSecret: my-auth-secret
```

This will allow SBOMbastic to scan for images from private registries.

**Please, note**:

The `Secret` and the `Registry` must be defined inside of the very same `Namespace`.
