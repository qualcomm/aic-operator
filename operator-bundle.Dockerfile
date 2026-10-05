FROM scratch
ARG VERSION=none
LABEL name="cloud_ai_openshift_operator_bundle" \
      maintainer="Qualcomm Technologies, Inc." \
      vendor="Qualcomm Technologies, Inc." \
      version="${VERSION}" \
      release="${VERSION}" \
      summary="Openshift Operator Bundle to orchestrate the Qualcomm AIC Operator" \
      description="The Openshift Operator Bundle enables the deployment of Openshift-Operator for \
Qualcomm® Cloud AI series of hardware."

# Core bundle labels.
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=aic-operator
LABEL operators.operatorframework.io.bundle.channels.v1=alpha
LABEL operators.operatorframework.io.metrics.builder=operator-sdk-v1.42.3
LABEL operators.operatorframework.io.metrics.mediatype.v1=metrics+v1
LABEL operators.operatorframework.io.metrics.project_layout=go.kubebuilder.io/v4

# Labels for testing.
LABEL operators.operatorframework.io.test.mediatype.v1=scorecard+v1
LABEL operators.operatorframework.io.test.config.v1=tests/scorecard/

# Labels for certified-operators
LABEL operatorframework.io/suggested-namespace=aic-operator-system
LABEL com.redhat.openshift.versions="v4.20-v4.22"

# Copy files to locations specified by labels.
COPY bundle/manifests /manifests/
COPY bundle/metadata /metadata/
COPY bundle/tests/scorecard /tests/scorecard/
# Include license and location information
LABEL org.opencontainers.image.source=https://github.com/qualcomm/cloud-ai-containers
COPY operands/kmd_src_img_build/CONTAINER_LICENSE.txt /licenses/
