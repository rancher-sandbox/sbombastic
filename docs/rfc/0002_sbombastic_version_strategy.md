|              |                                 |
| :----------- | :------------------------------ |
| Feature Name | 	SBOMBastic versioning          |
| Start Date   | May 20th, 2025                  |
| Category     | Versioning                      |
| RFC PR       | [fill this in after opening PR] |
| State        | **ACCEPTED**                    |

# Summary

[summary]: #summary

SBOMBastic is composed of controllers, workers, and storage that work together in order to build the final product. This is a proposal to harmonize SBOMBastic versioning schema.

# Motivation

[motivation]: #motivation

Defining a versioning strategy for SBOMBastic will allow us to:
- Align on the versioning schema across all components
- Make it easier to upgrade SBOMBastic
- Make it easier to test and validate the changes

[examples]: #examples


### User story #1

As a user, I want a unified versioning scheme for SBOMbastic so that I can easily understand and upgrade the entire system without confusion or friction.

### User story #2

As a user, when I encounter a bug in SBOMbastic, I want to be able to report it using a single, unified version that reflects all components.
I also want to upgrade to a fixed version without dealing with compatibility issues between components.

### User story #3

As a maintainer, when a user reports an issue with SBOMbastic (which includes multiple components), I want to know:
- Which version of SBOMbastic was initially installed
- What upgrade path was taken
This helps ensure reproducibility and simplifies debugging.


### User story #4

As a user upgrading SBOMBastic to a new version, I want to know if the new version introduces backward incompatible changes and behavior, so I can decide if to upgrade, and how.

For example, if upgrading the helm charts in my cluster has backwards-incompatible changes, as I may be forced to redeploy from scratch, halt workloads of do manual tasks. Or if sbombastic introduces backwards-incompatible changes, that would necessitate changes to my CI infrastructure.



# Detailed design

[design]: #detailed-design

## Many component, unified version
SBOMBastic is composed of three main components: the controller, worker and storage.

To simplify version management and improve clarity, all of these components will share a single version, following the  [Semantic Versioning](https://semver.org/) specification.

This means the SBOMbastic Controller, Worker, and Storage will always be released together using the same <Major>.<Minor>.<Patch> version number.


## Helm Charts
Helm charts have two kinds of version numbers:

- `version`: a SemVer 2 version specific to the helm chart
- `appVersion`: the version of the SBOMBastic that the chart contains

The helm charts will keep their own independence when it comes to
the `version` attribute. That is, using Semver, which is for helm charts helps users be aware of backwards-incompatible changes when upgrading.

The Helm chart `version` will receive a minor version bump whenever changes are made to the chart or when the SBOMBastic stack version is updated.
Finally, the `appVersion` attribute will always be set to the `<Major>.<Minor>.<Patch>` stick to the version of the SBOMBastic stack.

> See the official documentation about
> [`Chart.yaml`](https://helm.sh/docs/topics/charts/#the-chartyaml-file)
> for more information.

## Example
This section outlines scenarios to illustrate how the proposal would work in practice.

### A new release SBOMBastic takes place
A new version of SBOMBastic stack has to be released because new features has been introduced.
Assumptions:
- The current version of the SBOMBastic stack is `1.2.0`
- The current version of the helm chart is `v1.5.3`

Actions:
All core components (Controller, Worker, Storage) will be tagged and released as `1.2.0`.

Helm Chart Changes:
- The chart `version` attribute receives a minor bump too because the version
  of the Kubewarden stack was bumped: `v1.6.0`
- The `appVersion` attribute is set to `1.2.0`, because all of the components have the same version.


### A patch for SBOMBastic stack
A patch release is made to deliver backward-compatible bug fixes.

Assumptions:
- The current version of the SBOMBastic stack is `1.2.0`
- The current version of the helm chart is `v1.5.3`

Actions:
All core components (Controller, Worker, Storage) will be tagged and released as `1.2.1`.

Helm Chart Changes:
- The chart `version` attribute receives a minor bump too because the version
  of the Kubewarden stack was bumped: `v1.5.4`
- The `appVersion` attribute is set to `1.2.1`, because all of the components have the same version.

### A patch for helm chart
Sometimes only the Helm chart needs to be updated, such as adjusting argument defaults or template logic.

Assumptions
- The current version of the Helm chart is `v1.5.4`

A patch to upgrade Helm chart due to the argument setting.
The Helm chars will update to `v1.5.5`

### A bug found in SBOMBastic stack
When users encounter a bug, they can simply report the SBOMbastic stack version they are using (e.g., `1.2.1`).
This unified versioning approach makes it much easier for maintainers to reproduce the issue and verify the environment across components.


# Drawbacks

[drawbacks]: #drawbacks

Every pathc update triggers a full release
- Even if only a single component is updated, all components must be released together.
- This comsume unnecessary CI/CD resources and increases operational overhead.

All components must upgrade together
- Updating the version of one component requires all others to adopt the same version, even if they haven’t changed./

Version numbers may not accurately reflect code changes
- For example, if the controller is patched multiple times and its version bumps to 1.2.5, the worker and storage components must also be released as 1.2.5, despite having no actual code changes.

# Alternatives

[alternatives]: #alternatives

<!---
- What other designs/options have been considered?
- What is the impact of not doing this?
--->

# Unresolved questions

[unresolved]: #unresolved-questions

<!---
- What are the unknowns?
- What can happen if Murphy's law holds true?
--->
