# AIC Fleet Readiness and Drift Guard

## User problem

An `AIC` resource can be accepted while the selected nodes, NFD labels, KMM
module, firmware and device plugin are still incomplete or inconsistent. The
first visible failure is then usually an application pod that cannot schedule.
Administrators need one bounded status view that explains which prerequisite
is missing and what to check next.

## Why this belongs in the AIC operator

The operator already creates the NFD rule, KMM `Module`, build `ConfigMap`, and
device-agent `DaemonSet`. It is the component that knows the intended image
versions, node selector, pull secret, and ownership boundary. A second fleet
CRD would duplicate that relationship and make status harder to discover.

The MVP therefore extends `AIC.status` and does not add a new CRD.

## Smallest useful MVP

The controller evaluates selected Nodes, the owned KMM `Module`, NFD rule,
build ConfigMaps, image pull Secret, device-agent DaemonSet, and the
KMM-created device-plugin DaemonSet state exposed through the Module status.
It reports:

- `Ready`, `Degraded`, `Reconciling`, and `PrerequisitesReady` conditions.
- `observedGeneration` and selected, ready, degraded, and unavailable counts.
- Bounded per-node state using node name, state, reason, and next action only.
- Missing prerequisites, selector and version drift, and the
  `qualcomm.com/qaic` allocatable resource.
- A concise next action and a readiness-change log entry.

Readiness is observational. The controller only reconciles resources already
owned by the AIC operator. It does not drain or reboot nodes, unload modules,
restart unrelated workloads, change image versions, or mutate nodes.

## Objects and APIs watched

The AIC resource remains the primary watch. The controller owns and watches its
KMM `Module`, build `ConfigMap`s, NFD `NodeFeatureRule`, and device-agent
`DaemonSet`. It also reads cluster `Node`s and the referenced namespaced
`Secret`. KMM owns the device-plugin DaemonSet and publishes its aggregate
status on `Module.status`, so the AIC controller does not claim ownership of
that DaemonSet.

Node changes enqueue AIC resources whose effective selector may match the
changed node. Changes to the referenced Secret enqueue only the AIC in the
same namespace. KMM and device-agent changes are observed through owned
resource watches and the normal reconciliation interval.

## Status and condition model

All conditions use `metav1.Condition` with stable types and reasons. Messages
are short, deterministic, and free of secret values. `Ready=True` means all
selected nodes expose the expected device resource and the owned prerequisites
are present. `Degraded=True` means at least one selected node or prerequisite
is not ready. `Reconciling=True` means the owned resources or their observed
status are still settling. `PrerequisitesReady=True` means the resource and
version checks pass.

The summary counts are integers. Node details are capped to prevent status
growth on large clusters. The status never contains Secret data, registry
credentials, pod logs, full image-pull errors, or unrelated node metadata.

## Security and RBAC impact

The controller needs read-only access to Nodes and the referenced Secret, plus
the existing read access to KMM, NFD, ConfigMaps, and DaemonSets. Secret data
is only used to determine whether the named Secret exists. No new privileged
container, host path, host device, service account, or write permission is
introduced by the readiness feature.

## Test strategy

Pure readiness tests cover condition transitions, selector and version drift,
missing resources, device-resource detection, bounded node output, and
recovery. Controller tests cover fake-client status persistence, idempotent
reconciliation, and the absence of repeated status writes when observations
do not change. Generated CRD, deepcopy, RBAC, formatting, build, vet, and
repository test targets are run where the local environment supports them.
OpenShift, KMM, NFD, Qualcomm hardware, registry access, and kernel-module
build tests remain explicitly cluster-dependent.

## Explicitly not automated

The MVP does not remediate nodes, restart workloads, validate registry access by
pulling images, inspect kernel logs, probe firmware files on hosts, or claim
that a device is usable inside an application container. Those checks require
cluster or node-level evidence that this controller does not safely possess.

## Future evolution

Later work can add richer per-node evidence, metrics for fleet counts, and an
explicit opt-in remediation controller behind separate permissions. The status
contract should remain bounded and reason-based. Any future remediation must
have its own policy, audit trail, and approval boundary rather than being
hidden inside the readiness loop.
