# Design proposal for Velero Installation with Carvel

Currently, the Velero CLI tool has a `install` command that configures numerous major and minor aspects of Velero. This document outlines how we will change Velero to use the Carvel tooling and artifacts to drive installation and configuration.

## Goals
- Have one source of truth for all Velero configuration
- Have compatibility with gitop practices (i.e. ability to generate a full set of yaml for install that can be stored in source control)

## Definitions
### Configuration provider
The Velero developers. Authors of the configuration files, they provide the templates and artifacts to install Velero on Kubernetes clusters with default values and the necessary configuration constraints.
### Configuration consumer
Users of Velero. They rely on one source of truth from which to consume Velero templates and artifacts. They can customize the result of the templated Velero configuration to override default configuration values as well as to set and update configuration specific for their plugins.

## Requirements
### Organization
- The generated Velero configuration templates and artifacts must be kept in the Velero GitHub repository.
- The confiruation artifacts must be be laid out in a directory structure in a way that conforms with the expected standard so that they can be easily discovered by configuration consumers.

### Configuration
- The creation and configuration of Velero deployment and other non-CRD resources must be migrated from being generated using Go code to being available as artifacts that the Carvel tooling can consume to create and update these resources in Kubernetes clusters.
- Pursue an "ask once" approach to configuring the Velero deployment. In other words: given the custom configuration input from the configuration consumer, independely determine what environment variables and defaults, and other configuration settings are necessary for that setup.
- A default Backup Storage Location must be automatically configured and created based on the custom values provided by the configuration consumer.
- For compatibility with devops, Velero must continue to provide a `dry-run` option via the Carvel tooling for outputing configuration artifacts without installing them.
- Because the Carvel tooling, given proper templates, offers full functionality to do what the current Velero install command does, namelly create deployment and other resources in Kubernetes clusters, as well as update them, a path to deprecating that command could be considered and defined. An intermediate step might be to wrap the `install` command around the Carvel tooling.
- Documentation and sample templates to be used by the configuration consumers must be provided for each environment supported.

### Release packaging
- The resulting artifacts, including CRDs, must be packaged so that it contains all the appropriate set of configuration that comprises a Velero release.
- Users must be able to upgrade and downgrade Velero versions.
- The Velero/Carvel integration will be supported with the Velero v1.7 and onwards.
- Upgrades from and downgrades to previous versions of Velero must be complatible with the v1.7 way of installation.


## Supported environments
Envirornments that need to be supported: AWS, Vsphere, Azure, Kind.

## Configuration to be supported per enviroment

| Configuration                              |      AWS       |   Vsphere  |  Azure    | Kind  |
| ------------------------------------------ | -------------- | ---------- | ----------| ----- |
| Backup/restore of K8s resources            |      ✅        |     ✅     |    ✅      |   ✅  |
| Backup/restore of PVs via plugins          |      ✅        |     ✅     |    ✅      |  N/A  |
| Backup/restore of PVs with restic          |      ✅        |     ✅     |    ✅      |  N/A  |

## Future Goals
- Allow configuration for additional, non-default Backup Storage Locations.
- Allow configuration for multiple credentials.
- If Velero is running on a distro that has already been configured for a specific plugin provider, the Velero configuration should be able to intake this existing set of values instead of having to rely on the configuration consumer to to provide those condifurations again.

## Non Goals
- Introduce new configuration
- Propose changes to the configuration that go beyond the ability to install and update as currently exists.
- At this time, we are not going to make any effort to establish compatibility between the Carvel tooling and Helm artifacts.