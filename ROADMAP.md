## Velero Roadmap

### About this document
This document provides a link to the [Velero Project boards](https://github.com/vmware-tanzu/velero/projects) that serves as the up to date description of items that are in the release pipeline. The release boards have separate swim lanes based on prioritization. Most items are gathered from the community or include a feedback loop with the community. This should serve as a reference point for Velero users and contributors to understand where the project is heading, and help determine if a contribution could be conflicting with a longer term plan. 

### How to help?
Discussion on the roadmap can take place in threads under [Issues](https://github.com/vmware-tanzu/velero/issues) or in [community meetings](https://velero.io/community/). Please open and comment on an issue if you want to provide suggestions, use cases, and feedback to an item in the roadmap. Please review the roadmap to avoid potential duplicated effort.

### How to add an item to the roadmap?
One of the most important aspects in any open source community is the concept of proposals. Large changes to the codebase and / or new features should be preceded by a [proposal](https://github.com/vmware-tanzu/velero/blob/main/GOVERNANCE.md#proposal-process) in our repo.
For smaller enhancements, you can open an issue to track that initiative or feature request.
We work with and rely on community feedback to focus our efforts to improve Velero and maintain a healthy roadmap.

### Current Roadmap
The following table includes the current roadmap for Velero. If you have any questions or would like to contribute to Velero, please attend a [community meeting](https://velero.io/community/) to discuss with our team. If you don't know where to start, we are always looking for contributors that will help us reduce technical, automation, and documentation debt.
Please take the timelines & dates as proposals and goals. Priorities and requirements change based on community feedback, roadblocks encountered, community contributions, etc. If you depend on a specific item, we encourage you to attend community meetings to get updated status information, or help us deliver that feature by contributing to Velero.

`Last Updated: July 2021`

#### 1.7.0 Roadmap (to be delivered early fall)
The release roadmap is split into Core items that are required for the release and desired items that may slip the release.

##### Core items
The top priority of 1.7 is to increase the technical health of Velero and be more efficient with Velero developer time by streamlining the release process and automating and expanding the E2E test suite.

|Issue|Description|
|---|---|
|Issue coming soon|Streamline release process|
|Issue coming soon|Automate the running of the E2E tests|
|Issue coming soon|Convert pre-release manual tests to automated E2E tests|
|[3493](https://github.com/vmware-tanzu/velero/issues/3493)|[Carvel](https://github.com/vmware-tanzu/velero/issues/3493) based installation (in addition to the existing *velero install* CLI).|
|[3531](https://github.com/vmware-tanzu/velero/issues/3531)|Test plan for Velero|
|[675](https://github.com/vmware-tanzu/velero/issues/675)|Velero command to generate debugging information.  Will integrate with [Crashd - Crash Diagnostics](https://github.com/vmware-tanzu/velero/issues/675)|
|[3285](https://github.com/vmware-tanzu/velero/issues/3285)|Support Velero plugin versioning|
|[1975](https://github.com/vmware-tanzu/velero/issues/1975)|IPV6 support|
|[3533](https://github.com/vmware-tanzu/velero/issues/3533)|Upload Progress Monitoring|
|[3500](https://github.com/vmware-tanzu/velero/issues/3500)|Use distroless containers as a base|


##### Desired items
|Issue|Description|
|---|---|

|[2922](https://github.com/vmware-tanzu/velero/issues/2922)|Plugin timeouts|

##### Items formerly in 1.7 that will likely slip due to staffing changes
|Issue|Description|
|---|---|
|[3536](https://github.com/vmware-tanzu/velero/issues/3536)|Manifest for backup/restore|
|[2066](https://github.com/vmware-tanzu/velero/issues/2066)|CSI Snapshots GA|
|[3535](https://github.com/vmware-tanzu/velero/issues/3535)|Design doc for multiple cluster support|

