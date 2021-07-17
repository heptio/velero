# Plugin Versioning

## Abstract
This proposal outlines an approach to support versioning of Velero's plugin APIs to enable changes to those APIs to be made that are backwards compatible through the addition of new plugin methods.
It will not support backwards incompatible changes such as method removal or method signature changes.

## Background
When changes are made to Velero’s plugin APIs, there is no mechanism for the Velero server to communicate the version of the API that is supported, or for plugins to communicate what version they implement.
This means that any modification to a plugin API is a backwards incompatible change as it requires all plugins which implement the API to update and implement the new method.

There are several components involved to use plugins within Velero.
From the perspective of the core Velero codebase, all plugin kinds (e.g. ObjectStore, BackupItemAction) are defined by a single API interface and all interactions with plugins are managed by a plugin manager which provides an implementation of the plugin API interface for Velero to use.

Velero communicates with plugins via gRPC.
The core Velero project provides a framework (using the go-plugin project) for plugin authors to use to implement their plugins which manages the creation of gRPC servers and clients.
Velero plugins import the Velero plugin library in order to use this framework.
When a change is made to a plugin API, it needs to be made to the Go interface used by the Velero codebase, and also to the rpc service definition which is compiled to form part of the framework.
As each plugin kind is defined by a single interface, when a plugin imports the latest version of the Velero framework, it will need to implement the new APIs in order to build and run successfully.
If a plugin does not use the latest version of the framework, and is used with a newer version of Velero that expects the plugin to implement those methods, this will result in a runtime error as the plugin is incompatible.

With this proposal, we aim to break this coupling and introduce plugin API versions.

## Scenarios to Support
The following describes interactions between Velero and its plugins that will be supported with the implementation of this proposal.
For the purposes of this list, we will refer to existing Velero and plugin versions as `v1` and all following versions as version `n`.

Velero client communicating with plugins:

- Version `n` Velero client will be able to communicate with Version `n` plugin
- Version `n` Velero client will be able to communicate with all previous versions of the plugin (Version `n-1` back to `v1`)
- Optional: `v1` Velero client will be able to communicate with Version `n` plugin
    - This may work but is not a requirement

Plugin client communicating with other plugins:

- Version `n` plugin client will be able to communicate with Version `n` plugin
- Version `n` plugin client will be able to communicate with all previous versions of the plugin (Version `n-1` back to `v1`)
- Optional: `v1` plugin will be able to communicate with Version `n` plugin
    - This may work but is not a requirement

Velero plugins importing Velero framework:
- `v1` plugin built against Version `n` Velero framework
    - A plugin may choose to only implement a `v1` API but it must be able to be built using Version `n` of the Velero framework


## Goals

- Allow plugin APIs to change without requiring all plugins to implement latest changes (even if they upgrade the version of Velero that is imported).
- Allow plugins to choose which plugin versions they support and enable them to support multiple versions
- Allow for backwards and forwards compatibility with plugin versions
- Establish a design process for adding new plugin API methods

## Non Goals

- Support breaking changes in the plugin APIs such as method removal or method signature changes
- Changing how plugins are managed or added

## High-Level Design

With each change to a plugin API, a new version of the plugin interface and the proto service definition will be created which incorporates the new changes plus any previous versions of that particular plugin API.
The plugin framework will be adapted to allow these new plugin versions to be registered.
Plugins can opt to implement any or all versions of an API, however Velero will always attempt to use the latest version, and the plugin management will be modified to adapt earlier versions of a plugin to be compatible with the latest API.
Under the existing plugin framework, any new plugin version will be treated as a new plugin with a new kind, however the plugin manager (which provides implementations of a plugin to Velero) will include an adapter layer which will manage the different versions and adapt any version which does not implement the latest version of the plugin API.

For now, there will be a restriction that only new methods can be added to a plugin API.
Methods can not be removed or modified.
This restriction will be in place until Velero v2 when backwards incompatible changes may be included.

Although adding new rpc methods to a service is considered a backwards compatible change within gRPC, due to the way the proto definitions are compiled and included in the framework used by plugins, this will require every plugin to implement the new methods.
Instead, we are opting to treat _any_ API change as one requiring versioning.

## Detailed Design

The following areas will need to be adapted to support plugin versioning.

### Plugin Interface Definitions

To provide versioned plugins, any new method added to a plugin interface will require a new versioned interface to be created which incorporates previous versions of the interface.
Currently, all plugin interface definitions reside in `pkg/plugin/velero` in a file corresponding to their plugin kind.
These files will be rearranged to be grouped by kind and then versioned: `pkg/plugin/velero/<plugin_kind>/<version>/`.

For example, if a new method belongs to the ObjectStore API, a file would be added to `pkg/plugin/velero/objectstore/v2/` and would contain the following new API definition:

```
import "github.com/vmware-tanzu/velero/pkg/plugin/velero/objectstore/v1""

type ObjectStore interface {
	v1.ObjectStore
	NewMethod()
}
```

### Proto Service Definitions

The proto service definitions of the plugins will also be versioned and arranged by their plugin kind.
Currently, all the proto definitions reside under `pkg/plugin/proto` in a file corresponding to their plugin kind.
These files will be rearranged to be grouped by kind and then versioned: `pkg/plugin/proto/<plugin_kind>/<version>`.
The scripts to compile the proto service definitions will need to be updated to place the generated Go code under a matching directory structure.

It is not possible to import an existing proto service into a new one, so the methods will need to be duplicated across versions.
The message definitions can be shared however, so these could be extracted from the service definition files and placed in a file that can be shared across all versions of the service.

### Plugin Framework

To allow plugins to register which versions of the API they implement, the plugin framework will need to be adapted to accept new versions.
Currently, the plugin manager stores a [`map[string]RestartableProcess`](https://github.com/vmware-tanzu/velero/blob/main/pkg/plugin/clientmgmt/manager.go#L69), where the string key is the binary name for the plugin process (e.g. "velero-plugin-for-aws").
Each `RestartableProcess` contains a [`map[kindAndName]interface{}`](https://github.com/vmware-tanzu/velero/blob/main/pkg/plugin/clientmgmt/restartable_process.go#L60) which represents each of the unique plugin implementations provided by that binary.
[`kindAndName`](https://github.com/vmware-tanzu/velero/blob/main/pkg/plugin/clientmgmt/registry.go#L42) is a struct which combines the plugin kind (ObjectStore, VolumeSnapshotter) and the plugin name ("velero.io/aws", "velero.io/azure").

Each plugin version registration must be unique (to allow for multiple versions to be implemented within the same plugin binary).
This will be achieved by adding a specific registration method for each version to the Server interface in the plugin framework.
For example, if adding a V2 RestoreItemAction plugin, the Server interface would be modified to add the `RegisterRestoreItemActionV2` method.
This would require [adding a new plugin Kind const](https://github.com/vmware-tanzu/velero/blob/main/pkg/plugin/framework/plugin_kinds.go#L28-L46) to represent the new plugin version, e.g. `PluginKindObjectStoreV2`.
It also requires the creation of a new implementation of the go-plugin interface ([example](https://github.com/vmware-tanzu/velero/blob/main/pkg/plugin/framework/object_store.go)) to support that version and use the generated gRPC code from the proto definition (including a client and server implementation).
The Server will also need to be adapted to recognize this new plugin Kind and to serve the new implementation.

### Plugin Manager

The plugin manager is responsible for managing the lifecycle of plugins.
It provides an interface which is used by Velero to retrieve an instance of a plugin kind with a specific name (e.g. ObjectStore with the name "velero.io/aws").
The manager contains a registry of all available plugins which is populated during the main Velero server startup.
When the plugin manager is requested to provide a particular plugin, it checks the registry for that plugin kind and name.
If it is available in the registry, the manager retrieves a `RestartableProcess` for the plugin binary, creating it if it does not already exist.
That `RestartableProcess` is then used by individual restartable implementations of a plugin kind (e.g. `restartableObjectStore`, `restartableVolumeSnapshotter`).

As new plugin versions are added, the plugin manager will be modified to retrieve the latest version of a plugin kind, adapting earlier versions of the plugin kind if possible.
This is to allow the remainder of the Velero codebase to assume that it will always interact with the latest version of a plugin.
The manager will check the registry for the plugin kind and name, and if the requested version is not found, it will attempt to fetch previous versions of the plugin kind and adapt it if possible.
It will be up to the author of new plugin versions to determine whether a previous version of a plugin can be adapted to work with the interface of the new version.

```
// GetObjectStore returns a restartableObjectStore for name.
func (m *manager) GetObjectStore(name string) (v2.ObjectStore, error) {
    name = sanitizeName(name)

    restartableProcess, err := m.getRestartableProcess(framework.PluginKindObjectStoreV2, name)
    if err != nil {
        // Check if plugin was not found
        if errors.Is(err, pluginNotFoundError) {
            // Try again but with previous version
            restartableProcess, err := m.getRestartableProcess(framework.PluginKindObjectStore, name)
            if err != nil {
                // No v1 version found, return
                return nil, err
            }

            r := newRestartableObjectStore(name, restartableProcess)
            // Adapt v1 plugin to v2
            return newAdaptedV1ObjectStore(r), nil
        } else {
            return nil, err
        }
    }
    return newRestartableObjectStoreV2(name, restartableProcess), nil
}
```

If the previous version is not available, or can not be adapted to the latest version, an error will be returned as is currently the case when a plugin implementation for a particular kind and provider can not be found.

There are situations where it may be beneficial to check at the point where a plugin API call is made whether it implements a specific version of the API.
This is something that can be addressed with future amendments to this design, however it does not seem to be necessary at this time.

### Restartable Plugin Process

As new versions of plugins are added, new restartable implementations of plugins will also need to be created.
These are currently located within "pkg/plugin/clientmgmt" but will be rearranged to be grouped by kind and version like other plugin files.

## Versioning Considerations

It should be noted that if changes are being made to a plugin's API, it will only be necessary to bump the version once within a release cycle, regardless of how many changes are made within that cycle.
This is because the changes will only impact consumers when they upgrade the versions of the Velero library that they import.
Once a new version of Velero has been released however, any further changes will need to follow the process above and use a new version.

## Alternatives Considered

### Relying on gRPC’s backwards compatibility when adding new methods

One approach for adapting the plugin APIs would have been to rely on the fact that adding methods to gRPC services is a backwards compatible change.
This approach would allow older clients to communicate with newer plugins as the existing interface would still be provided.
This was considered but ruled out as our current framework would require any plugin that recompiles using the latest version of the framework to adapt to the new version.
Also, without specific versioned interfaces, it would require checking plugin implementations at runtime for the specific methods that are supported.

## Compatibility

This design doc aims to allow plugin API changes to be made in a backwards compatible manner.
All compatibility concerns are addressed earlier in the document.

## Implementation

This design document primarily outlines an approach to allow future plugin API changes to be made.
However, there are changes to the existing code base that will be made to allow plugin authors to more easily propose and introduce changes to these APIs.

* Plugin interface definitions (currently in `pkg/plugin/velero`) will be rearranged to be grouped by kind and then versioned: `pkg/plugin/velero/<plugin_kind>/<version>/`.
* Proto definitions (currently in `pkg/plugin/proto`) will be rearranged to be grouped by kind and then versioned: `pkg/plugin/proto/<plugin_kind>/<version>`.
  * This will also require changes to the `make update` build task to correctly find the new proto location and output to the versioned directories.

It is anticipated that changes to the plugin APIs will be made as part of the 1.7 release cycle.
To assist with this work, an additional follow-up task to the ones listed above would be to prepare a V2 version of each of the existing plugins.
These new versions will not yet provide any new API methods but will provide a layout for new additions to be made

## Open Issues
