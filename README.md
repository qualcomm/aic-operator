# Qualcomm Cloud AIC Operator(AIC Operator)

The AIC Operator enables the Qualcomm Cloud AI 100 series of accelerators on OpenShift
clusters by automating the installation and configuration of their Linux device drivers
and setting up the associated Device Plugin.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
  - [1. Install the KMM Operator](#1-install-the-kmm-operator)
  - [2. Install the NFD Operator](#2-install-the-nfd-operator)
  - [3. Configure the Cluster](#3-configure-the-cluster)
  - [4. Install the AIC Operator](#4-install-the-aic-operator)
  - [5. Create an AIC Instance](#5-create-an-aic-instance)
- [Running Inference Workloads](#running-inference-workloads)
- [Uninstallation](#uninstallation)
- [Building from Source](#building-from-source)
- [Contributing](#contributing)
- [License](#license)

## Prerequisites

- OpenShift v4.18+
- Kernel Module Management (KMM) Operator v2.0+ by Red Hat
- Node Feature Discovery (NFD) Operator v4.16+ by Red Hat
- A Qualcomm AI 100 Ultra accelerator attached to the host node
- `oc` CLI configured and logged in with cluster-admin privileges
- Access to a container registry reachable from the cluster

## Installation

### 1. Install the KMM Operator

The Kernel Module Management (KMM) Operator handles out-of-tree driver builds and
loading on cluster nodes.

1. In the OpenShift web console, navigate to **Ecosystem → Software Catalog**.
2. Search for **Kernel Module Management** and install the version provided by **Red Hat**
   (not the Hub or Community variant).
3. Use the default KMM namespace **`openshift-kmm`**.

### 2. Install the NFD Operator

The Node Feature Discovery (NFD) Operator labels nodes with hardware capabilities so the
AIC Operator can target the correct nodes.

1. In the OpenShift web console, navigate to **Ecosystem → Software Catalog**.
2. Search for **Node Feature Discovery** and install the version provided by **Red Hat**
   (not the Community variant).
3. Use the default NFD namespace **`openshift-nfd`**.
4. After installation, navigate to the NFD operator page and create the default
   **NodeFeatureDiscovery** CR by clicking **Create instance**.

### 3. Configure the Cluster

#### Verify cluster access

Log in to the cluster from a terminal. To get the login command, open the web console,
click the **kube:admin** dropdown in the top-right corner, and select
**Copy login command**.

```sh
oc get nodes
```

Expected output:

```
NAME           STATUS   ROLES                         AGE    VERSION
<node-name>    Ready    control-plane,master,worker   209d   v1.29.10+67d3387
```

#### Create the operator namespace and registry secret

```sh
oc create namespace aic-operator-system
```

The `sourceImage`, `devicePluginImage`, and `deviceAgentImage` are hosted on the public
`ghcr.io/qualcomm` registry and do not require credentials.

KMM builds a kernel-specific driver image inside the cluster and pushes it to the
repository specified by `spec.driversImage`. The registry credentials must have
both pull and push permission for that repository.

```sh
oc create secret docker-registry <pull-secret-name> \
  --docker-server=<container-registry-server> \
  --docker-username=<username> \
  --docker-password=<password-or-token> \
  --namespace=aic-operator-system
```

> Reference this secret by name in the `AIC` CR under `spec.imageRepoSecret.name`.

#### Configure KMM firmware path

The KMM worker needs to know where firmware files are stored on the host. Apply the
following ConfigMap to set the firmware host path, then restart the KMM controller pods:

> **Warning:** The `oc apply` command below replaces the entire `kmm-operator-manager-config`
> ConfigMap. If your cluster has custom KMM settings beyond the firmware path, merge them
> into the `controller_config.yaml` block before applying.

```sh
cat <<EOF | oc apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: kmm-operator-manager-config
  namespace: openshift-kmm
data:
  controller_config.yaml: |
    worker:
      firmwareHostPath: /var/lib/firmware/updates  # Ensure firmware files are placed in this directory
EOF
```

> **Note:** The path `/var/lib/firmware/updates` is the expected firmware location for
> this configuration. Verify that firmware files on your cluster nodes are placed in this
> directory. If your cluster uses a different firmware path, update this value accordingly
> before applying.

Verify the ConfigMap was updated:

```sh
oc get configmap kmm-operator-manager-config -n openshift-kmm -o=json
# Verify the output contains: "firmwareHostPath: /var/lib/firmware/updates"
```

Restart the KMM controller to apply the change:

```sh
oc delete pod -n openshift-kmm -l app.kubernetes.io/component=kmm
```

Verify the KMM controller pods are running after the restart:

```sh
oc get pods -n openshift-kmm -l app.kubernetes.io/component=kmm
# All pods should show STATUS: Running
```

For full KMM configuration options see the
[KMM documentation](https://kmm.sigs.k8s.io/documentation/configure/).

### 4. Install the AIC Operator

**Option A — via the web console:**

1. Navigate to **Ecosystem → Software Catalog**.
2. Search for **AIC Operator** and install the version provided by
   **Qualcomm Technologies, Inc.**
3. Set the **Installed Namespace** to **`aic-operator-system`**.

**Option B — via CLI using Kustomize:**

```sh
# Latest (uses the image tag committed on the main branch)
oc apply -k https://github.com/qualcomm/aic-operator/config/default

# Pin to a specific release — the kustomization.yaml at that tag already has the
# correct image version, so no substitution is needed or # latest available.
oc apply -k https://github.com/qualcomm/aic-operator/config/default?ref=v0.5.0
```

> The image tag is managed in `config/manager/kustomization.yaml` in the repository.
> Each release tag sets the correct operator image version — use `?ref=<tag>` to select
> a release (see [GitHub releases](https://github.com/qualcomm/aic-operator/releases)).
> This command creates the `aic-operator-system` namespace, registers the CRD, and
> deploys the controller in one step. When using this option:
> - Skip the `oc create namespace` command in step 3.
> - Create the pull secret **after** running the command above (the namespace must
>   exist first). Use the same `oc create secret docker-registry` command from step 3.

A successful installation (either option) starts a pod named
`aic-operator-controller-manager-*` (with 2 containers) in the
`aic-operator-system` namespace:

```sh
oc get pods -n aic-operator-system
```

### 5. Create an AIC Instance

Create an `AIC` custom resource to load the drivers and start the Device Plugin on nodes
that carry a Cloud AI 100 accelerator.

Save the following as `aic-sample.yaml`, replacing all `<placeholder>` values for your
environment. The `${KERNEL_VERSION}` token in `driversImage` is substituted
automatically by KMM at build time — leave it as-is.

```yaml
apiVersion: aic.qualcomm.com/v1
kind: AIC
metadata:
  name: aic-sample
  namespace: aic-operator-system
  labels:
    app.kubernetes.io/name: aic
    app.kubernetes.io/instance: aic-sample
    app.kubernetes.io/part-of: aic-operator
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/created-by: aic-operator
spec:
  sourceImage: "ghcr.io/qualcomm/cloud_ai_qaic_kmd_src"
  driversImage: "<your-registry-server>/<username>/cloud_ai_qaic_kmd_ko:0.4.0_1.22.8.1_${KERNEL_VERSION}"
  driversVersion: "0.4.0_1.22.8.1" # or latest available
  devicePluginImage: "ghcr.io/qualcomm/cloud_ai_k8s_device_plugin"
  devPluginVersion: "1.22.8.1"     # or latest available
  deviceAgentImage: "ghcr.io/qualcomm/cloud_ai_qaic_deviceagent"
  deviceAgentVersion: "0.4.0_1.22.8.1" # or latest available
  imageRepoSecret:
    name: <pull-secret-name>   # name of the secret created in step 3
```

Apply it:

```sh
oc create -f aic-sample.yaml
```

Verify the AIC instance is running and the device plugin pod has started:

```sh
oc get aic -n aic-operator-system
oc get pods -n aic-operator-system
```

## Running Inference Workloads

Once the AIC instance is running, nodes with Cloud AI 100 hardware expose the
`qualcomm.com/qaic` resource. The following example pod requests 4 AIC devices and
keeps a shell open for interactive use:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: qaic-inf-deployment
  namespace: aic-operator-system
  labels:
    app: qaic-inf
spec:
  restartPolicy: Always
  priority: 0
  containers:
    - name: qaic-inf
      image: ghcr.io/qualcomm/cloud_ai_inference_ubuntu24:1.22.8.1 # or latest available
      imagePullPolicy: IfNotPresent
      command: ["/bin/bash", "-ce", "tail -f /dev/null"]
      resources:
        limits:
          qualcomm.com/qaic: "4"   # number of AIC devices to allocate
        requests:
          qualcomm.com/qaic: "4"
      securityContext:
        runAsGroup: 995   # GID of the 'qaic' group inside the container image
```

Save the manifest above as `qaic-inf-pod.yaml`, then apply it:

```sh
oc create -f qaic-inf-pod.yaml
```

Open a shell inside the pod and run the sample inference script:

```sh
oc exec -it qaic-inf-deployment -n aic-operator-system -- /bin/bash
# Inside the pod:
source vllm_env/bin/activate
python /opt/qti-aic/integrations/vllm/examples/offline_inference/qaic.py
```

## Uninstallation

Delete the AIC custom resource first (this stops the device plugin and unloads the
drivers):

```sh
oc delete -f aic-sample.yaml
# or
oc delete AIC aic-sample -n aic-operator-system
```

Then uninstall the operator through the OpenShift web console:
**Ecosystem → Installed Operators → AIC Operator → Actions → Uninstall Operator**.

## Building from Source

### Requirements

- Go v1.23.0+
- Docker v17.03+

### Build and Push the Operator Image

```sh
make docker-build docker-push \
  IMG=<some-registry>/cloud_ai_openshift_operator:tag \
  VERSION=<version>
```

### Build and Push the Operator Bundle

```sh
make bundle-build bundle-push \
  IMG=<some-registry>/cloud_ai_openshift_operator:tag \
  BUNDLE_IMG=<some-registry>/cloud_ai_openshift_operator_bundle:tag \
  VERSION=<version>
```

> **Note:** The images must be published to a registry accessible from your cluster.
> Ensure you have the appropriate push and pull permissions before running these commands.

### Install CRDs and Deploy Manually

```sh
make install
make deploy IMG=<some-registry>/cloud_ai_openshift_operator:tag
```

Run `make help` for a full list of available targets.

### Uninstall a source-built deployment

If you deployed the operator manually from source, use the Makefile targets instead:

```sh
oc delete -f aic-sample.yaml
make uninstall
make undeploy
```

## Contributing

See the [CONTRIBUTING](CONTRIBUTING.md) guide for details on how to contribute to this
project. Additional background on the operator framework can be found in the
[Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html).

## License

The AIC Operator is licensed under the terms of the [LICENSE](LICENSE) file.
