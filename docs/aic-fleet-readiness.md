# AIC fleet readiness

The `AIC` status shows whether the operator's selected nodes are ready to run
workloads that request `qualcomm.com/qaic`. It evaluates the NFD rule, KMM
module, owned build ConfigMaps, image pull Secret reference, device-agent
DaemonSet, node labels, KMM readiness labels, and node allocatable resources.

Example:

```yaml
status:
  observedGeneration: 2
  state: NotReady
  readiness:
    selected: 3
    ready: 2
    degraded: 0
    unavailable: 1
  conditions:
  - type: Ready
    status: "False"
    reason: DevicePluginUnavailable
    message: check KMM device-plugin pods and their logs
  - type: Degraded
    status: "True"
    reason: DevicePluginUnavailable
    message: check KMM device-plugin pods and their logs
  - type: Reconciling
    status: "True"
    reason: OwnedResourcesSettling
    message: owned resources or node observations are still settling
  - type: PrerequisitesReady
    status: "False"
    reason: DevicePluginUnavailable
    message: check KMM device-plugin pods and their logs
  nodes:
  - name: worker-a
    state: Ready
    reason: FleetReady
  - name: worker-b
    state: Unavailable
    reason: DevicePluginUnavailable
  nextAction: check KMM device-plugin pods and their logs
```

Start with the condition reason and `nextAction`, then inspect only the
affected nodes:

```sh
oc get aic -n aic-operator-system -o yaml
oc get node <node-name> --show-labels
oc get module <aic-name> -n aic-operator-system -o yaml
oc get pods -n aic-operator-system -l kmm.node.kubernetes.io/module.name=<aic-name>
oc describe node <node-name>
```

Common checks:

- `HardwareLabelMissing`: verify NFD is running and the node has the
  `qualcomm.com/qaic=true` label.
- `SelectorDrift`: compare the AIC selector with the owned KMM Module. The
  MVP reports this condition and does not rewrite a user-selected node set.
- `VersionDrift`: compare the AIC image and version fields with the owned KMM
  Module and device-agent DaemonSet.
- `DeviceResourceMissing`: verify the device plugin is ready and the node
  allocatable list contains `qualcomm.com/qaic`.
- `MissingPrerequisite`: verify the named KMM, NFD, ConfigMap, or image pull
  Secret exists.

The status is bounded and does not include Secret data, registry credentials,
pod logs, firmware contents, or unrelated node metadata. Readiness does not
prove that an inference workload can start. Image pulls, kernel-module builds,
firmware availability, and application admission still require cluster-level
validation.
