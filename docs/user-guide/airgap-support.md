# Air Gap Support

SBOMbastic can be used in air-gapped environments.

To run the scans, SBOMbastic currently needs the following external sources:

* Vulnerability Database

* Java Vulnerability Database

* VEX Hub (optional)

These external sources can be self-hosted in your private infrastucture to make the whole environment air-gapped.

## Self-Hosting Vulnerability Databases

The following Vulnerability Databases are packaged as OCI images, allowing you to host them in your own registry:

* [`trivy-db`](https://github.com/aquasecurity/trivy-db/pkgs/container/trivy-db)

* [`trivy-java-db`](https://github.com/aquasecurity/trivy-java-db/pkgs/container/trivy-java-db)

Once mirrored in your own OCI registry, you can install SBOMbastic to point to them:

```shell
helm install sbombastic ./chart \
    --set worker.trivyDBRepository="yourlocalregistry.example/sbombastic/trivy-db" \
    --set worker.trivyJavaDBRepository="yourlocalregistry.example/sbombastic/trivy-java-db"
```

## Self-Hosting VEX Hub

To setup your own VEX Hub repository, please refer to this [guide](https://github.com/aquasecurity/trivy/blob/main/docs/docs/advanced/self-hosting.md#make-a-local-copy-1).

Change the `repository_url` (if any) within the VEX files, to point to the internal registries.

All you need to do is to setup an HTTP server to provide the needed files for VEX.

To configure a VEX Hub in SBOMbastic, create a `VEXHub` resource with your local repository URL and apply it:

```yaml
apiVersion: sbombastic.rancher.io/v1alpha1
kind: VEXHub
metadata:
  name: local_vexhub
spec:
  url: "https://yourlocalrepo.example/"
  enabled: true
```
