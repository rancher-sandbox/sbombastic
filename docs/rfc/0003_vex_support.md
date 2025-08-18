|              |                                 |
| :----------- | :------------------------------ |
| Feature Name | VEX Support                     |
| Start Date   | 16 July 2025                    |
| Category     | Architecture                    |
| RFC PR       | [#326](https://github.com/rancher-sandbox/sbombastic/pull/326) |
| State        | **ACCEPTED**                       |

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
OCI images. VEX Hub repositories are a collection of VEX files with a defined
structure so that scanning tools can pull them locally and use them to filter
out their output.

That said, we want to add support for:

* Keeping the configuration of multiple VEX Hub repositories

* Support private VEX Hub repositories with credentials and auth tokens

* Use VEX documents to scan images

## Examples / User Stories

[examples]: #examples

<!---
Examples of how the feature will be used. Interactions should show the action
and the response. When appropriate, provide user stories in the form of "As a
[role], I want [feature], so [that]."
--->

### User story #1

As a user, I want to scan registries on my infrastructure, filtering out the 
number of CVEs detected by SBOMbastic, removing false positives as much as possible.

### User story #2

As a user, I want to configure the registry scan with appropriate VEX files 
depending on the content of the registry (e.g., test, staging, prod images), 
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

Suppose you are scanning the registry of a dedicated department in your company.
This registry will host a specific kind of images depending on the scope of the 
department. In order to make the scan as accurate as possible, we are going to 
provide a new clusterwide CRD called `VEXHub`. This CRD will hold the 
configuration of the VEX Hub repositories, so that the Registry CRD will refer 
to these `VEXHub` resources.

Since VEXHub CRD is clusterwide, this means that you can use the same 
configuration across multiple registries.

## VEXHub CRD

To configure a new `VEXHub` resource, the user will need to apply a manifest 
as follows:

```yaml
apiVersion: sbombastic.rancher.io/v1alpha1
kind: VEXHub
metadata:
  name: company-vexhub
spec:
  url: "https://vex.company.com/"
  enabled: true
  authSecret:
    name: company-vexhub-creds
    namespace: sbombastic
```

There are a few attributes to keep in mind when configuring the `VEXHub` CRD:

* `url`: is the URL of the VEX Hub repository (this is mandatory)

* `enabled`: to enable/disable the VEX Hub repo (`true` by default)

* `authSecret`: used to store auth secret to get access to the repo (optional)

A VEX Hub repository might have authentication enforced. When that happens, 
the `VEXHub` CRD has a `spec.authSecret` field, which is used to 
reference a `Secret` resource that holds the authentication details.
The `Secret` being referenced must be placed in the same 
`Namespace` where the SBOMbastic stack is deployed.

The user can use these two formats for secrets, since VEX Hub supports auth
through `username`/`password` or `token`.

Below you will find an example of `Secret` with `username` and `password`:

```yaml
# Example Secret for Basic Authentication
apiVersion: v1
kind: Secret
metadata:
  name: company-vexhub-creds
  namespace: sbombastic
type: Opaque
data:
  username: YWRtaW4=
  password: c2VjcmV0cGFzc3dvcmQ=
```

Here's how the `Secret` with `token` should look like:

```yaml
# Example Secret for Bearer Token
apiVersion: v1
kind: Secret
metadata:
  name: company-vexhub-token
  namespace: sbombastic
type: Opaque
data:
  token: ZXlKaGaU9pSktWMVFpT2c9PQ==
```

## AirGap

AirGap is available by default for this feature, since the only requirement is
to provide a self-hosted VEX Hub repository and change the `repository_url` 
(if any) within the VEX files, to point to the internal registries.

This setup is well described [here](https://github.com/aquasecurity/trivy/blob/main/docs/docs/advanced/air-gap.md#vex-hub)

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

The alternative approach is to allow the `VEXHub` configuration to be directly 
into the `Registry` CRD.

This approach is easier in terms of development but has some critical issues.

* Increase the complexity of the Registry CRD and its management

* Duplication of configurations across different registries

* Lack of centralized VEX Hub management

# Unresolved questions

[unresolved]: #unresolved-questions

<!---
- What are the unknowns?
- What can happen if Murphy's law holds true?
--->
## HTTPs

We have to consider the fact that VEX Hub repositories can use HTTPS 
encryption. This means that user might want to use their own certificates 
to verify the connection of the repository.

More info here: [aquasecurity/trivy#4194 (comment)](https://github.com/aquasecurity/trivy/discussions/4194#discussioncomment-5828069)
