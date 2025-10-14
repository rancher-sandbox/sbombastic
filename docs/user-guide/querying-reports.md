## Querying SBOM and VulnerabilityReport Resources

In this guide, you'll learn how to query SBOMscanner resources using metadata fields.

We'll walk through three major steps:

1. Understand the supported query fields

2. Use `kubectl get --field-selector` to filter the target `Image`, `SBOM` and `VulnerabilityReport` resources

3. Use `kubectl describe` to read the full details of a specific report

### Supported `imageMetadata` Fields

`Image`, `SBOM` and `VulnerabilityReport` custom resources share a common `imageMetadata` field, which contains metadata about the target image.
These fields are useful when filtering resources with `kubectl get --field-selector`.

| Field         | Type   | Description                                                                               |
| ------------- | ------ | ----------------------------------------------------------------------------------------- |
| `registry`    | string | Name of the `Registry` object.                                                            |
| `registryURI` | string | Full URI of the registry where the image is hosted. Example: `registry-1.docker.io:5000`. |
| `repository`  | string | The image repository path. Example: `kubewarden/sbomscanner`.                                 |
| `tag`         | string | The image tag. Example: `latest`, `v1.2.3`.                                               |
| `platform`    | string | The image platform, in OS/ARCH format. Example: `linux/amd64`.                            |
| `digest`      | string | The SHA256 digest that uniquely identifies the image.                                     |

> These fields are available on both `SBOM` and `VulnerabilityReport` resources and are consistent across both kinds.

### Query Examples

Now that you know the available fields, let's walk through a few practical examples.

#### Example: Get all vulnerability reports from a specific repository and platform

Use the following command to list all `VulnerabilityReport` resources for images from the `kubewarden/sbomscanner/test-assets/golang` repository, built for the `amd64` platform:

```bash
kubectl get vulnerabilityreport --field-selector='imageMetadata.repository=kubewarden/sbomscanner/test-assets/golang,imageMetadata.platform=linux/amd64'
```

**Example output:**

```bash
NAME                                                               CREATED AT
dfe56d8371e7df15a3dde25c33a78b84b79766de2ab5a5897032019c878b5932   2025-06-23T04:35:16Z
...
```

#### Example: Get SBOMs from the same repository with a specific tag and platform

If you're looking for the all SBOMs of images tagged `1.12-alpine` and built for `amd64`, you can run:

```bash
kubectl get sboms --field-selector='imageMetadata.repository=kubewarden/sbomscanner/test-assets/golang,imageMetadata.tag=1.12-alpine,imageMetadata.platform=linux/amd64'
```

**Example output:**

```bash
NAME                                                               CREATED AT
dfe56d8371e7df15a3dde25c33a78b84b79766de2ab5a5897032019c878b5932   2025-06-23T04:34:41Z
```

### Example: Get Images from a specific registry URI

To list all `Image` resources from the `ghcr.io` registry, use:

```bash
kubectl get images --field-selector='imageMetadata.registryURI=ghcr.io'
```

### View Report/SBOM Details

Once you identify a resource name from the output above, use kubectl describe to read the full contents:

```bash
kubectl get images <name> -o yaml
kubectl get sboms <name> -o yaml
kubectl get vulnerabilityreports <name> -o yaml
```
