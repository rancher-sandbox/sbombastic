|              |                                 |
| :----------- | :------------------------------ |
| Feature Name | Vex Support                     |
| Start Date   | 16 July 2025                    |
| Category     | Architecture                    |
| RFC PR       | [fill this in after opening PR] |
| State        | **Draft**                       |

# Summary

[summary]: #summary

Support VEX (Vulnerability Exploitability Exchange) documents to filter or provide additional context to matches.

# Motivation

[motivation]: #motivation

Currently, we scan SBOMs without the use of VEX documents. VEX is a format used 
to convey information about the exploitability of vulnerabilities in software 
products and share them with scanning tools.

The use of this format can sensitively reduce the number of vulnerabilities 
in the final vulnerability report, which is more often full of false positives
entries.

To reduce the noise, VEX is the right choice if you are dealing with SBOMs and 
OCI images.

That said, we want to add support for:

* Keeping the configuration of multiple VexHub repositories

* Support private VexHub repos with credentials and auth tokens

* Use VEX documents to scan images

## Examples / User Stories

[examples]: #examples

<!---
Examples of how the feature will be used. Interactions should show the action
and the response. When appropriate, provide user stories in the form of "As a
[role], I want [feature], so [that]."
--->

### User story #1

As a user, I want to scan registries on my infrastructure filtering out the 
number of CVEs detected by SBOMbastic, removing false positives as much as possible.

### User story #2

As a user, I want to configure the registry scan with appropriate VEX files 
depending on the content of the registry (eg. test, staging, prod images), 
so that the vulnerability report will have an accurate result.

# Detailed design

[design]: #detailed-design

<!---
This is the bulk of the RFC. Explain the design in enough detail for somebody
familiar with the product to understand, and for somebody familiar with the
internals to implement.

This section should cover architecture aspects and the rationale behind
disruptive technical decisions (when applicable), as well as corner-cases and
warnings.
--->

This RFC introduces support for VEX files during registry scanning.
From now on, the Registry CRD will have an additional field to refer to the 
VexHub resources we want to use to scan it.

Suppose you are scanning the registry of a dedicated department in your company.
This registry will host a specific kind of images depending on the scope of the 
department. In order to make the scan as accurate as possible, we are going to 
provide a new clusterwide CRD called VexHub. This CRD will hold the 
configuration of the VexHub repositories, so that the Registry CRD will refer 
to these VexHub resources.

Since VexHub CRD is clusterwide, this means that you can use the same 
configuration across multiple registries.

## VexHub CRD

To configure a new VexHub, the user will need to apply a manifest as follows:

```yaml
apiVersion: sbombastic.rancher.io/v1alpha1
kind: VexHub
metadata:
  name: vendor-vexhub
  namespace: default
spec:
  url: https://vexhub.vendor.com
  credentials:
    secretRef:
      name: vendor-vexhub-secret
      key: access-token
```

As you can see, the VexHub CRD has a `secretRef` field, which means that you 
need to provide a Secret to configure the credentials (if any):

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: vexhub-auth
  namespace: security-team
type: Opaque
data:
  username: <username>
  token: <base64-encoded-token>
```

Or using `username` and `password` as in this example:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: vexhub-auth
  namespace: security-team
type: Opaque
data:
  username: <username>
  password: <password>
```

This way, the registry will be scanned with VEX files.

In addition to this, we have to consider the fact that VexHub repositories
can use HTTPS encryption. This means that user might want to use their own 
certificates to verify the connection of the repository.

## AirGap

AirGap is available by default for this feature, since the only requirement is
to provide a self-hosted vexhub repository and change the `repository_url` 
(if any) whitin the VEX files, to point to the internal registries.

This setup is well described here: https://github.com/aquasecurity/trivy/blob/main/docs/docs/advanced/air-gap.md#vex-hub

# Drawbacks

[drawbacks]: #drawbacks

<!---
Why should we **not** do this?

  * obscure corner cases
  * will it impact performance?
  * what other parts of the product will be affected?
  * will the solution be hard to maintain in the future?
--->

# Alternatives

[alternatives]: #alternatives

<!---
- What other designs/options have been considered?
- What is the impact of not doing this?
--->

The alternative approach is to allow the VexHub configuration to be directly 
into the `Registry` CRD.

This approach is easier in terms of development but has some critical issues.

* Increase the complexity of the Registry CRD and its management

* Duplication of configurations across different registries

* Lack of centralized VexHub management

# Unresolved questions

[unresolved]: #unresolved-questions

<!---
- What are the unknowns?
- What can happen if Murphy's law holds true?
--->
