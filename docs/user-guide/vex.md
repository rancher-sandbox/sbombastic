# VEX Support

VEX is a format used to convey information about the exploitability of vulnerabilities in software products and share them with scanning tools.

The use of this format can sensitively reduce the number of vulnerabilities in the final vulnerability report, which is more often full of false positives entries.

If you want to know more about VEX, take a look [here](https://github.com/openvex/spec).

## VEX Hub

VEX Hub is a place where VEX (Vulnerability Exploitability eXchange) documents from different open-source projects are collected and organized. It helps people and security tools find and use important information about software vulnerabilities more easily.

If you want to know more about VEX Hub, take a look [here](https://github.com/aquasecurity/vexhub).

## Getting Started

In order to scan `Registries` using VEX, you just have to create (one, or more) `VEXHub` resources.

Here's an example of `VEXHub` resource (you can find it under [`examples/vexhub.yaml`](https://github.com/rancher-sandbox/sbombastic/blob/main/examples/vexhub.yaml):

```yaml
apiVersion: sbombastic.rancher.io/v1alpha1
kind: VEXHub
metadata:
  name: rancher
spec:
  url: "https://github.com/rancher/vexhub"
  enabled: true
```

Apply the resource with:

```bash
kubectl apply -f examples/vexhub.yaml
```

Then, run the scan applying a [`ScanJob`](https://github.com/alegrey91/sbombastic/blob/main/examples/scanjob.yaml) configured to scan the desired registry.

SBOMbastic will automatically detect the presence of `VEXHub` resources and will include them in the scan.

Since `VEXHub` CRD is cluster-scoped, this means that you can use the same configuration across multiple registries.

### Managing Multiple VEX Hub Repositories

You can configure an arbitrary number of `VEXHub` repositories within your cluster.

If, for some reasons, you want to disable some of them, you can just patch their `spec.enabled` to `false`.

This will let SBOMbastic exclude the `VEXHub` resource when scanning the registries.

Here's the command to disable a `VEXHub` resource:

```bash
kubectl patch vexhub <vexhub-name> -p '{"spec":{"enabled":false}}'
```

## Air Gap

Air Gap is available by default for this feature, since the only requirement is to provide a self-hosted VEX Hub repository and change the `repository_url` (if any) within the VEX files, to point to the internal registries.

This setup is well described [here](https://github.com/aquasecurity/trivy/blob/main/docs/docs/advanced/air-gap.md#vex-hub).
