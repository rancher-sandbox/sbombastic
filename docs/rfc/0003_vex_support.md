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

Currently we scan SBOMs without the use of VEX documents. VEX is a format used 
to convey information about the exploitability of vulnerabilities in software 
products and share it with scanning tools.

The use of this format can sensitively reduce the number of vulnerabilities 
in the final vulnerability report, which more often is full of false positive
entries.

In order to reduce the noise, VEX is the right choise if you are dealing with
SBOMs and OCI images.

That said, we want to add support for:

* keeping configuration of multiple VexHub repositories

* support private VexHub repos with credentials and auth tokens

* use VEX docuements to scan images

## Examples / User Stories

[examples]: #examples

<!---
Examples of how the feature will be used. Interactions should show the action
and the response. When appropriate, provide user stories in the form of "As a
[role], I want [feature], so [that]."
--->

### User story #1

As a user, I want to scan registries on my infrastracture filtering out the 
number of CVEs detected by SBOMbastic, removing false positives as much as possible.

### User story #2

As a user, I want to configure the registries scan with appropriate VEX files 
depending on the content of the registry (eg. test, staging or prod images), 
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

This RFC introduces support for VEX files when scanning registries.
From now on, the Registry CRD will have an additional field to refer the VexHub
resources we want to use to scan it.

Suppose you are scanning the registry of a dedicated department in your company.
This registry will host specific kind of images depending on the scope of the 
department. In order to make the scan as much accurate as possible, we are 
going to provide a new clusterwide CRD called VexHub. This CRD will hold 
configuration of the VexHub repositories, so that the Registry CRD will refer 
to these VexHub resources.

Since VexHub CRD is clusterwide, this means that you can use the same 
configuration across multiple registries.

## VexHub CRD

To configure a new VexHub, the user will need to apply a new manifest as follow:

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

As you can see the VexHub CRD has a `secretRef` field, which means that you 
need to provide a Secret to configure the credentials (if any):

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: vexhub-auth
  namespace: security-team
type: Opaque
data:
  token: <base64-encoded-token>

```

## Registry CRD

To configure the Registry to use the VexHub repository, the user will need to 
update the Registry manifest as follow:

```yaml
apiVersion: scanner.rancher.io/v1alpha1
kind: Registry
metadata:
  name: registry-example
  namespace: default
spec:
  url: "https://registry-1.docker.io"
  type: "docker"
  auth:
    secretName: "registry-secret"
  discoveryPeriod: "1h"
  scanPeriod: "1d"
  repositories:
    - "repo1"
    - "repo2"
  vexHubRefs:
    - name: vendor-vexhub
    - name: internal-vexhub # example of multiple VexHub configuration
```

This way the registry will be scanned with appropriate VEX files.

## AirGap

AirGap is available by default for this feature, since the only requirement is
to provide a self-hosted vexhub repository and change the `repository_url` 
(if any) whitin the VEX files, to point to the internal registries.

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

The alternative approach is to allow the VexHub configuration directly into the
`Registry` CRD as follow:

```yaml
apiVersion: scanner.rancher.io/v1alpha1
kind: Registry
metadata:
  name: registry-example
  namespace: default
spec:
  url: "https://registry-1.docker.io"
  type: "docker"
  auth:
    secretName: "registry-secret"
  discoveryPeriod: "1h"
  scanPeriod: "1d"
  repositories:
    - "repo1"
    - "repo2"
  vexhubConfig:
    - url: https://vexhub.example.com
      credentials:
        secretRef:
          name: vexhub-secret
          key: token
```

This approach is easier in terms of development but has some critical issue:

* Increase complexity of the Registry CRD and its management

* Duplication of configurations across different registries

* Lack of centralized VexHub management

# Unresolved questions

[unresolved]: #unresolved-questions

<!---
- What are the unknowns?
- What can happen if Murphy's law holds true?
--->
