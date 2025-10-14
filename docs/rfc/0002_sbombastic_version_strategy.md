|              |                                                                    |
| :----------- | :----------------------------------------------------------------- |
| Feature Name | SBOMscanner versioning                                              |
| Start Date   | May 20th, 2025                                                     |
| Category     | Versioning                                                         |
| RFC PR       | [PR#215]](https://github.com/kubewarden/SBOMscanner/pull/215/) |
| State        | **ACCEPTED**                                                       |

# Summary

[summary]: #summary

SBOMscanner is composed of controllers, workers, and storage that work together in order to build the final product. This is a proposal to harmonize SBOMscanner versioning schema.

# Motivation

[motivation]: #motivation

Defining a versioning strategy for SBOMscanner will allow us to:

- Align on the versioning schema across all components
- Make it easier to upgrade SBOMscanner
- Make it easier to test and validate the changes

## User Stories

[userstories]: #userstories

### User story #1

As a user, I want a unified versioning scheme for SBOMscanner so that I can easily understand and upgrade the entire system without confusion or friction.

### User story #2

As a user, when I encounter a bug in SBOMscanner, I want to be able to report it using a single, unified version that reflects all components.
I also want to upgrade to a fixed version without dealing with compatibility issues between components.

### User story #3

As a maintainer, when a user reports an issue with SBOMscanner (which includes multiple components), I want to know:

- Which version of SBOMscanner was initially installed
- What upgrade path was taken
  This helps ensure reproducibility and simplifies debugging.

### User story #4

As a user upgrading SBOMscanner to a new version, I want to know if the new version
introduces backward incompatible changes and behavior, so I can decide if to upgrade,
and how.

For example, if upgrading the helm charts in my cluster has backwards-incompatible
changes, as I may be forced to redeploy from scratch, halt workloads of do manual tasks.
Or if SBOMscanner introduces backwards-incompatible changes, that would necessitate
changes to my CI infrastructure.

# Detailed design

[design]: #detailed-design

## Many component, unified version

SBOMscanner is composed of three main components: the controller, worker and storage.

To simplify version management and improve clarity, all of these components will share a
single version, following the [Semantic Versioning](https://semver.org/) specification.

This means the SBOMscanner Controller, Worker, and Storage will always be released
together using the same `<Major>.<Minor>.<Patch>` version number.

## Helm Charts

Helm charts have two kinds of version numbers:

- `version`: a SemVer 2 version specific to the helm chart
- `appVersion`: the version of the SBOMscanner that the chart contains

The helm charts will keep their own independence when it comes to
the `version` attribute. That is, using Semver, for helm charts helps users be aware of backwards-incompatible changes when upgrading.

The Helm chart `version` will receive a minor version bump whenever changes are made to the chart or when the SBOMscanner stack version is updated.
Finally, the `appVersion` attribute will always be set to match the version of the SBOMscanner stack.

> See the official documentation about
> [`Chart.yaml`](https://helm.sh/docs/topics/charts/#the-chartyaml-file)
> for more information.

## Examples

[examples]: #examples

This section outlines scenarios to illustrate how the proposal would work in practice.

### A new release SBOMscanner takes place

A new version of SBOMscanner stack has to be released because new features has been introduced.

Assumptions:

- The current version of the SBOMscanner stack is `1.2.0`
- The current version of the helm chart is `v1.5.3`

Actions:

- All core components (Controller, Worker, Storage) will be tagged and released as `1.3.0`.

Helm Chart Changes:

- The chart `version` attribute receives a minor bump too because the version
  of the SBOMscanner stack was bumped: `v1.6.0`
- The `appVersion` attribute is set to `1.3.0`, because all of the components have the same version.

### A patch for a component of the SBOMscanner stack

A patch release is made to deliver a backward-compatible bug fix for one of the
components of the SBOMscanner stack (e.g.: storage).

Assumptions:

- The current version of the SBOMscanner stack is `1.2.0`
- The current version of the helm chart is `v1.5.3`

Actions:

- All core components (Controller, Worker, Storage) will be tagged and released as `1.2.1`.

**Note:** all the components of the stack are tagged, even the ones that might
not have changed since the `1.2.0` release.

Helm Chart Changes:

- The chart `version` attribute receives a patch bump too because this is a
  a patch release of the SBOMscanner stack: `v1.5.4`
- The `appVersion` attribute is set to `1.2.1`, because all of the components have the same version.

### A patch for helm chart

Sometimes only the Helm chart needs to be updated, such as adjusting argument defaults or template logic.

Assumptions

- The current version of the SBOMscanner stack is `1.2.0`
- The current version of the Helm chart is `v1.5.4`

Helm Chart Changes:

- The chart `version` attribute receives a minor bump: `v1.6.0`
- The `appVersion` remains `1.2.0`

### A bug found in SBOMscanner stack

When users encounter a bug, they can simply report the SBOMscanner stack version they are using (e.g., `1.2.1`).
This unified versioning approach makes it much easier for maintainers to reproduce the issue and verify the environment across components.

# Drawbacks

[drawbacks]: #drawbacks

Every patch update triggers a full release

- Even if only a single component is updated, all components must be released together.
- This consumes CI/CD resources.

All components must upgrade together. Updating the version of one component requires all others to adopt the same version, even if they haven’t changed.

Version numbers may not accurately reflect code changes.
For example, if the controller is patched multiple times and its version bumps to `1.2.5`,
the worker and storage components must also be released as `1.2.5`,
despite having no actual code changes.

However, there are high chances that all the components, including the ones that
did not receive direct code changes, will feature dependency bumps. Be them
either direct or transitive ones.

# Alternatives

[alternatives]: #alternatives

# Separate Versioning

Another possible solution would be to has separate version for each component.
All the components would share the major and minor version, but would have an
independent patch version.

Pros:

- No need to perform patch release of components that did not receive any code change
- Only patched component is rebuilt: better usage of resources inside of our build
  system and end user system
- Reduce the amount of data to be pulled by our users

Cons:

- The helm chart `appVersion` would not be updated, its `patch` value would always
  be `0` even when one of the components gets a patch update.
- It becomes a bit harder for the end user to know if they are running the fully updated
  stack. They would have to make sure they are running with the latest version of
  the helm chart, while with the proposed solution they could also look at container
  images versions or the `appVersion` attribute of the helm chart.
- The build pipeline would become more complex: instead of having 1 tag for the whole
  stack, we would have 3 tags (one per component). The automation taking care of
  updating the helm chart would become more complex.

It's worth to note that nothing prevents us from changing the proposed versioning
strategy to be this alternative one.

# Unresolved questions

[unresolved]: #unresolved-questions

<!---
- What are the unknowns?
- What can happen if Murphy's law holds true?
--->
