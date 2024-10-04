# SBOMbastic

A SBOM-centric security scanner for Kubernetes.

# Development

## Run the development environment with Tilt

Customize `tilt-settings.yaml` to your needs.

Run tilt:

```shell
tilt up
```

Run tilt with unified logs:

```shell
tilt up --stream
```

Follow controller logs:

```shell
tilt logs -f controller
```

Follow storage logs:

```shell
tilt logs -f storage
```

Follow worker logs:

```shell
tilt logs -f worker
```

Teardown the environment:

```shell
tilt down
```

## Generate code

When you make changes to the CRDs in `/api` or rbac rules annotations, you need to regenerate the code.

```shell
make generate
```
