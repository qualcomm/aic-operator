# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= 0.0.1

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# IMAGE_REGISTRY and IMAGE_TAG_BASE define the registry namespace and image name.
# The local registry default prevents accidental publication. Set IMAGE_REGISTRY explicitly
# when building images that will be pushed to a remote registry.
#
# For example, running 'make bundle-build bundle-push catalog-build catalog-push IMAGE_REGISTRY=registry.example.com/project'
# will build and push both registry.example.com/project/cloud_ai_openshift_operator-bundle:$VERSION
# and registry.example.com/project/cloud_ai_openshift_operator-catalog:$VERSION.
IMAGE_REGISTRY ?= localhost
IMAGE_TAG_BASE ?= $(IMAGE_REGISTRY)/cloud_ai_openshift_operator

# BUNDLE_IMG defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:$(VERSION)

# BUNDLE_GEN_FLAGS are the flags passed to the operator-sdk generate bundle command
BUNDLE_GEN_FLAGS ?= -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)

# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
# You can enable this value if you would like to use SHA Based Digests
# To enable set flag to true
USE_IMAGE_DIGESTS ?= false
ifeq ($(USE_IMAGE_DIGESTS), true)
	BUNDLE_GEN_FLAGS += --use-image-digests
endif

## Pinned build tool versions
KUSTOMIZE_VERSION ?= v5.8.2
CONTROLLER_TOOLS_VERSION ?= v0.20.1
OPERATOR_SDK_VERSION ?= v1.42.3
GOLANGCI_LINT_VERSION ?= v2.14.0
ENVTEST_VERSION ?= v0.0.0-20260305142021-f9589b9f2b9d
OPM_VERSION ?= v1.74.0
OPENSHIFT_PREFLIGHT_VERSION ?= 1.21.2
GOLANGCI_LINT_VERSION_NUMBER := $(patsubst v%,%,$(GOLANGCI_LINT_VERSION))

# Kubernetes version used by the envtest control-plane assets.
ENVTEST_K8S_VERSION ?= 1.35.0

# USE_SYSTEM_TOOLS prefers the pinned build tools from PATH when their versions
# match. Missing or mismatched tools fall back to pinned copies in LOCALBIN,
# except golangci-lint, which must be provided by the development environment.
USE_SYSTEM_TOOLS ?= true

# Image URL used by image build, deployment, and bundle generation targets.
IMG ?= ${IMAGE_TAG_BASE}:${VERSION}

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$${KUBEBUILDER_ASSETS:-$$($(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)}" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: clean
clean: ## Remove generated local tools, SBOM/Preflight artifacts, and test output.
	rm -rf "$(LOCALBIN)" "$(SBOM_ARTIFACTS_DIR)" "$(PREFLIGHT_ARTIFACTS)" cover.out default.etcd
	find "$(CURDIR)" -maxdepth 1 -mindepth 1 -type d -name 'bundle_tmp[0-9]*' -exec rm -rf {} +

.PHONY: pkg-sbom
pkg-sbom: ## Build the manager and write its Syft package PURLs under sbom-artifacts/pkg-scan.
	@command -v "$(SYFT)" >/dev/null 2>&1 || { echo "Error: Syft is required on PATH; install it in the development environment."; exit 1; }
	mkdir -p "$(PKG_SCAN_DIR)"
	go build -o "$(PKG_SCAN_BINARY)" cmd/main.go
	$(SYFT) scan "file:$(PKG_SCAN_BINARY)" -o syft-json | (cd tools && go run ./cmd/extract-syft-purls) > "$(PKG_SCAN_PURL_LIST)"

.PHONY: pkg-compare
pkg-compare: pkg-sbom
	@test -n "$(APPROVED_PKG_LIST)" && test -r "$(APPROVED_PKG_LIST)" || { echo "Error: set APPROVED_PKG_LIST to a readable internal approval list."; exit 1; }
	sed -E -e 's/[[:space:]]+#.*$$//' -e 's/^[[:space:]]+//' -e 's/[[:space:]]+$$//' "$(APPROVED_PKG_LIST)" | sed '/^$$/d' | sort -u > "$(PKG_SCAN_APPROVED_LIST)"
	comm -23 "$(PKG_SCAN_PURL_LIST)" "$(PKG_SCAN_APPROVED_LIST)" > "$(PKG_SCAN_UNAPPROVED_LIST)"

.PHONY: pkg-report
pkg-report: pkg-compare ## Print packages in the manager binary that are not approved.
	@if test -s "$(PKG_SCAN_UNAPPROVED_LIST)"; then \
		echo "Unapproved packages:"; \
		cat "$(PKG_SCAN_UNAPPROVED_LIST)"; \
	else \
		echo "All manager-binary packages are approved."; \
	fi

.PHONY: pkg-check
pkg-check: pkg-compare ## Fail when the manager binary contains an unapproved package.
	@if test -s "$(PKG_SCAN_UNAPPROVED_LIST)"; then \
		echo "Unapproved packages:"; \
		cat "$(PKG_SCAN_UNAPPROVED_LIST)"; \
		exit 1; \
	fi
	@echo "All manager-binary packages are approved."

.PHONY: image-pkg-sbom
image-pkg-sbom: docker-build bundle-build ## Build the manager and bundle images and write their non-Go PURLs under sbom-artifacts.
	@command -v "$(SYFT)" >/dev/null 2>&1 || { echo "Error: Syft is required on PATH; install it in the development environment."; exit 1; }
	mkdir -p "$(IMAGE_PKG_SCAN_DIR)"
	mkdir -p "$(BUNDLE_IMAGE_PKG_SCAN_DIR)"
	$(SYFT) scan "docker:$(IMG)" -o syft-json | (cd tools && go run ./cmd/extract-syft-purls) > "$(IMAGE_PKG_SCAN_ALL_PURL_LIST)"
	awk '!/^pkg:golang\//' "$(IMAGE_PKG_SCAN_ALL_PURL_LIST)" | sed -E 's/[@?].*$$//' | sort -u > "$(IMAGE_PKG_SCAN_PURL_LIST)"
	$(SYFT) scan "docker:$(BUNDLE_IMG)" -o syft-json | (cd tools && go run ./cmd/extract-syft-purls) > "$(BUNDLE_IMAGE_PKG_SCAN_ALL_PURL_LIST)"
	awk '!/^pkg:golang\//' "$(BUNDLE_IMAGE_PKG_SCAN_ALL_PURL_LIST)" | sed -E 's/[@?].*$$//' | sort -u > "$(BUNDLE_IMAGE_PKG_SCAN_PURL_LIST)"

.PHONY: image-pkg-compare
image-pkg-compare: image-pkg-sbom
	@test -n "$(APPROVED_BASEOS_PKG_LIST)" && test -r "$(APPROVED_BASEOS_PKG_LIST)" || { echo "Error: set APPROVED_BASEOS_PKG_LIST to a readable internal approval list."; exit 1; }
	sed -E -e 's/[[:space:]]+#.*$$//' -e 's/^[[:space:]]+//' -e 's/[[:space:]]+$$//' "$(APPROVED_BASEOS_PKG_LIST)" | sed '/^$$/d' | sort -u > "$(IMAGE_PKG_SCAN_APPROVED_LIST)"
	cp "$(IMAGE_PKG_SCAN_APPROVED_LIST)" "$(BUNDLE_IMAGE_PKG_SCAN_APPROVED_LIST)"
	comm -23 "$(IMAGE_PKG_SCAN_PURL_LIST)" "$(IMAGE_PKG_SCAN_APPROVED_LIST)" > "$(IMAGE_PKG_SCAN_UNAPPROVED_LIST)"
	comm -23 "$(BUNDLE_IMAGE_PKG_SCAN_PURL_LIST)" "$(BUNDLE_IMAGE_PKG_SCAN_APPROVED_LIST)" > "$(BUNDLE_IMAGE_PKG_SCAN_UNAPPROVED_LIST)"

.PHONY: image-pkg-report
image-pkg-report: image-pkg-compare ## Print unapproved non-Go packages in the manager and bundle images.
	@if test -s "$(IMAGE_PKG_SCAN_UNAPPROVED_LIST)"; then \
		echo "Unapproved manager-image packages:"; \
		cat "$(IMAGE_PKG_SCAN_UNAPPROVED_LIST)"; \
	else \
		echo "All non-Go manager-image packages are approved."; \
	fi
	@if test -s "$(BUNDLE_IMAGE_PKG_SCAN_UNAPPROVED_LIST)"; then \
		echo "Unapproved bundle-image packages:"; \
		cat "$(BUNDLE_IMAGE_PKG_SCAN_UNAPPROVED_LIST)"; \
	else \
		echo "All non-Go bundle-image packages are approved."; \
	fi

.PHONY: image-pkg-check
image-pkg-check: image-pkg-compare ## Fail when the manager or bundle image contains an unapproved non-Go package.
	@status=0; \
	if test -s "$(IMAGE_PKG_SCAN_UNAPPROVED_LIST)"; then \
		echo "Unapproved manager-image packages:"; \
		cat "$(IMAGE_PKG_SCAN_UNAPPROVED_LIST)"; \
		status=1; \
	else \
		echo "All non-Go manager-image packages are approved."; \
	fi; \
	if test -s "$(BUNDLE_IMAGE_PKG_SCAN_UNAPPROVED_LIST)"; then \
		echo "Unapproved bundle-image packages:"; \
		cat "$(BUNDLE_IMAGE_PKG_SCAN_UNAPPROVED_LIST)"; \
		status=1; \
	else \
		echo "All non-Go bundle-image packages are approved."; \
	fi; \
	exit "$$status"

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter & yamllint
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} --build-arg VERSION=${VERSION} .

.PHONY: docker-push
docker-push: docker-build ## Push the manager image, then run OpenShift Preflight against the remote image.
	@echo "Remote registry required: pushing manager image $(IMG). Set IMAGE_REGISTRY=<registry>/<namespace> explicitly."
	$(CONTAINER_TOOL) push ${IMG}
	$(MAKE) --no-print-directory preflight-remote

.PHONY: openshift-preflight
openshift-preflight: docker-push ## Push the manager image, then run OpenShift Preflight against the remote image.

.PHONY: preflight-remote
preflight-remote: preflight
	@echo "Remote registry required: running OpenShift Preflight against the pushed image $(IMG)."
	@docker_config="$${PFLT_DOCKERCONFIG:-$$HOME/.docker/config.json}"; \
	test -r "$$docker_config" || { echo "Error: Docker credentials are required for the private image; run docker login and set PFLT_DOCKERCONFIG if the config is not $$HOME/.docker/config.json."; exit 1; }; \
	mkdir -p "$(PREFLIGHT_ARTIFACTS)"; \
	PFLT_ARTIFACTS="$(PREFLIGHT_ARTIFACTS)" PFLT_LOGFILE="$(PREFLIGHT_LOGFILE)" PFLT_DOCKERCONFIG="$$docker_config" "$(PREFLIGHT)" check container "$(IMG)" | tee "$(PREFLIGHT_RESULT)"; \
	grep -Fq '"passed": true' "$(PREFLIGHT_RESULT)" || { echo "Error: OpenShift Preflight reported a failed check."; exit 1; }

.PHONY: release-approval-inputs
release-approval-inputs:
	@test -n "$(APPROVED_PKG_LIST)" && test -r "$(APPROVED_PKG_LIST)" || { echo "Error: set APPROVED_PKG_LIST to a readable internal approval list."; exit 1; }
	@test -n "$(APPROVED_BASEOS_PKG_LIST)" && test -r "$(APPROVED_BASEOS_PKG_LIST)" || { echo "Error: set APPROVED_BASEOS_PKG_LIST to a readable internal approval list."; exit 1; }

.PHONY: release-push
release-push: release-approval-inputs pkg-check image-pkg-check ## Validate approved packages, push the manager, run Preflight, then push the bundle release image.
	@echo "Remote registry required: pushing and validating manager image $(IMG), then pushing bundle image $(BUNDLE_IMG). Set IMAGE_REGISTRY=<registry>/<namespace> explicitly."
	$(CONTAINER_TOOL) push ${IMG}
	$(MAKE) --no-print-directory preflight-remote
	$(CONTAINER_TOOL) push $(BUNDLE_IMG)

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	@echo "Remote registry required: docker buildx will push $(IMG). Set IMAGE_REGISTRY=<registry>/<namespace> explicitly."
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name project-v3-builder
	$(CONTAINER_TOOL) buildx use project-v3-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm project-v3-builder
	rm Dockerfile.cross

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
LOCAL_KUSTOMIZE := $(LOCALBIN)/kustomize
LOCAL_CONTROLLER_GEN := $(LOCALBIN)/controller-gen
LOCAL_OPERATOR_SDK := $(LOCALBIN)/operator-sdk-$(OPERATOR_SDK_VERSION)
LOCAL_ENVTEST := $(LOCALBIN)/setup-envtest-$(ENVTEST_VERSION)
LOCAL_PREFLIGHT := $(LOCALBIN)/preflight-$(OPENSHIFT_PREFLIGHT_VERSION)
SYFT ?= syft
# Approval data is intentionally supplied from a local or internal source.
APPROVED_PKG_LIST ?=
SBOM_ARTIFACTS_DIR ?= $(CURDIR)/sbom-artifacts
PKG_SCAN_DIR ?= $(SBOM_ARTIFACTS_DIR)/pkg-scan
PKG_SCAN_BINARY ?= $(PKG_SCAN_DIR)/manager
PKG_SCAN_PURL_LIST ?= $(PKG_SCAN_DIR)/purls.txt
PKG_SCAN_APPROVED_LIST ?= $(PKG_SCAN_DIR)/approved-purls.txt
PKG_SCAN_UNAPPROVED_LIST ?= $(PKG_SCAN_DIR)/unapproved-purls.txt
APPROVED_BASEOS_PKG_LIST ?=
IMAGE_PKG_SCAN_DIR ?= $(SBOM_ARTIFACTS_DIR)/image-pkg-scan
IMAGE_PKG_SCAN_ALL_PURL_LIST ?= $(IMAGE_PKG_SCAN_DIR)/all-purls.txt
IMAGE_PKG_SCAN_PURL_LIST ?= $(IMAGE_PKG_SCAN_DIR)/purls.txt
IMAGE_PKG_SCAN_APPROVED_LIST ?= $(IMAGE_PKG_SCAN_DIR)/approved-purls.txt
IMAGE_PKG_SCAN_UNAPPROVED_LIST ?= $(IMAGE_PKG_SCAN_DIR)/unapproved-purls.txt
BUNDLE_IMAGE_PKG_SCAN_DIR ?= $(SBOM_ARTIFACTS_DIR)/bundle-image-pkg-scan
BUNDLE_IMAGE_PKG_SCAN_ALL_PURL_LIST ?= $(BUNDLE_IMAGE_PKG_SCAN_DIR)/all-purls.txt
BUNDLE_IMAGE_PKG_SCAN_PURL_LIST ?= $(BUNDLE_IMAGE_PKG_SCAN_DIR)/purls.txt
BUNDLE_IMAGE_PKG_SCAN_APPROVED_LIST ?= $(BUNDLE_IMAGE_PKG_SCAN_DIR)/approved-purls.txt
BUNDLE_IMAGE_PKG_SCAN_UNAPPROVED_LIST ?= $(BUNDLE_IMAGE_PKG_SCAN_DIR)/unapproved-purls.txt
PREFLIGHT_ARTIFACTS ?= $(CURDIR)/preflight-artifacts
PREFLIGHT_LOGFILE ?= $(PREFLIGHT_ARTIFACTS)/preflight.log
PREFLIGHT_RESULT ?= $(PREFLIGHT_ARTIFACTS)/result.json

ifeq ($(USE_SYSTEM_TOOLS),true)
SYSTEM_KUSTOMIZE := $(shell command -v kustomize 2>/dev/null)
SYSTEM_CONTROLLER_GEN := $(shell command -v controller-gen 2>/dev/null)
SYSTEM_OPERATOR_SDK := $(shell command -v operator-sdk 2>/dev/null)
SYSTEM_GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null)
SYSTEM_ENVTEST := $(shell command -v setup-envtest 2>/dev/null)
SYSTEM_PREFLIGHT := $(shell command -v preflight 2>/dev/null)

SYSTEM_KUSTOMIZE_OK := $(shell if test -n "$(SYSTEM_KUSTOMIZE)"; then "$(SYSTEM_KUSTOMIZE)" version 2>&1 | grep -Fq "$(KUSTOMIZE_VERSION)" && echo true; fi)
SYSTEM_CONTROLLER_GEN_OK := $(shell if test -n "$(SYSTEM_CONTROLLER_GEN)"; then "$(SYSTEM_CONTROLLER_GEN)" --version 2>&1 | grep -Fq "$(CONTROLLER_TOOLS_VERSION)" && echo true; fi)
SYSTEM_OPERATOR_SDK_OK := $(shell if test -n "$(SYSTEM_OPERATOR_SDK)"; then "$(SYSTEM_OPERATOR_SDK)" version 2>&1 | grep -Fq "$(OPERATOR_SDK_VERSION)" && echo true; fi)
SYSTEM_GOLANGCI_LINT_OK := $(shell if test -n "$(SYSTEM_GOLANGCI_LINT)"; then "$(SYSTEM_GOLANGCI_LINT)" version 2>&1 | grep -Fq "$(GOLANGCI_LINT_VERSION_NUMBER)" && echo true; fi)
SYSTEM_ENVTEST_OK := $(shell if test -n "$(SYSTEM_ENVTEST)"; then "$(SYSTEM_ENVTEST)" version 2>&1 | grep -Fq "$(ENVTEST_VERSION)" && echo true; fi)
SYSTEM_PREFLIGHT_OK := $(shell if test -n "$(SYSTEM_PREFLIGHT)"; then "$(SYSTEM_PREFLIGHT)" --version 2>&1 | grep -Fq "$(OPENSHIFT_PREFLIGHT_VERSION)" && echo true; fi)
endif

ifeq ($(SYSTEM_KUSTOMIZE_OK),true)
KUSTOMIZE ?= $(SYSTEM_KUSTOMIZE)
else
KUSTOMIZE ?= $(LOCAL_KUSTOMIZE)
endif

ifeq ($(SYSTEM_CONTROLLER_GEN_OK),true)
CONTROLLER_GEN ?= $(SYSTEM_CONTROLLER_GEN)
else
CONTROLLER_GEN ?= $(LOCAL_CONTROLLER_GEN)
endif

ifeq ($(SYSTEM_OPERATOR_SDK_OK),true)
OPERATOR_SDK ?= $(SYSTEM_OPERATOR_SDK)
else
OPERATOR_SDK ?= $(LOCAL_OPERATOR_SDK)
endif

ifeq ($(SYSTEM_GOLANGCI_LINT_OK),true)
GOLANGCI_LINT ?= $(SYSTEM_GOLANGCI_LINT)
endif

ifeq ($(SYSTEM_ENVTEST_OK),true)
ENVTEST ?= $(SYSTEM_ENVTEST)
else
ENVTEST ?= $(LOCAL_ENVTEST)
endif

ifeq ($(SYSTEM_PREFLIGHT_OK),true)
PREFLIGHT ?= $(SYSTEM_PREFLIGHT)
else
PREFLIGHT ?= $(LOCAL_PREFLIGHT)
endif

.PHONY: kustomize
ifeq ($(SYSTEM_KUSTOMIZE_OK),true)
kustomize: ## Use the pinned kustomize version from PATH.
	@echo "Using system kustomize $(KUSTOMIZE_VERSION): $(KUSTOMIZE)"
else
kustomize: $(LOCALBIN) ## Install the pinned kustomize version locally when no matching system tool exists.
	@if ! test -x "$(LOCAL_KUSTOMIZE)" || ! "$(LOCAL_KUSTOMIZE)" version 2>&1 | grep -Fq "$(KUSTOMIZE_VERSION)"; then \
		echo "Installing kustomize $(KUSTOMIZE_VERSION) in $(LOCALBIN)"; \
		rm -f "$(LOCAL_KUSTOMIZE)"; \
		GOBIN="$(LOCALBIN)" go install sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION); \
	fi
endif

.PHONY: golangci-lint
ifeq ($(SYSTEM_GOLANGCI_LINT_OK),true)
golangci-lint: ## Use the pinned golangci-lint version from PATH.
	@echo "Using system golangci-lint $(GOLANGCI_LINT_VERSION): $(GOLANGCI_LINT)"
else
golangci-lint: ## Require the pinned golangci-lint version from PATH.
	@echo "Error: golangci-lint $(GOLANGCI_LINT_VERSION) must be installed on PATH; automatic installation is disabled by licensing policy."; \
	exit 1
endif

.PHONY: controller-gen
ifeq ($(SYSTEM_CONTROLLER_GEN_OK),true)
controller-gen: ## Use the pinned controller-gen version from PATH.
	@echo "Using system controller-gen $(CONTROLLER_TOOLS_VERSION): $(CONTROLLER_GEN)"
else
controller-gen: $(LOCALBIN) ## Install the pinned controller-gen version locally when no matching system tool exists.
	@if ! test -x "$(LOCAL_CONTROLLER_GEN)" || ! "$(LOCAL_CONTROLLER_GEN)" --version 2>&1 | grep -Fq "$(CONTROLLER_TOOLS_VERSION)"; then \
		echo "Installing controller-gen $(CONTROLLER_TOOLS_VERSION) in $(LOCALBIN)"; \
		GOBIN="$(LOCALBIN)" go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION); \
	fi
endif

.PHONY: envtest
ifeq ($(SYSTEM_ENVTEST_OK),true)
envtest: ## Use the pinned setup-envtest version from PATH.
	@echo "Using system setup-envtest $(ENVTEST_VERSION): $(ENVTEST)"
else
envtest: $(LOCAL_ENVTEST) ## Install the pinned setup-envtest version locally when no matching system tool exists.
$(LOCAL_ENVTEST): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(ENVTEST_VERSION)
	mv $(LOCALBIN)/setup-envtest $(LOCAL_ENVTEST)
endif

.PHONY: preflight
ifeq ($(SYSTEM_PREFLIGHT_OK),true)
preflight: ## Use the pinned OpenShift Preflight version from PATH.
	@echo "Using system OpenShift Preflight $(OPENSHIFT_PREFLIGHT_VERSION): $(PREFLIGHT)"
else
preflight: $(LOCALBIN) ## Download the pinned OpenShift Preflight version locally when no matching system tool exists.
	@if ! test -x "$(LOCAL_PREFLIGHT)" || ! "$(LOCAL_PREFLIGHT)" --version 2>&1 | grep -Fq "$(OPENSHIFT_PREFLIGHT_VERSION)"; then \
		echo "Downloading OpenShift Preflight $(OPENSHIFT_PREFLIGHT_VERSION) to $(LOCALBIN)"; \
		TMP="$(LOCAL_PREFLIGHT).tmp"; \
		trap 'rm -f "$$TMP"' EXIT; \
		wget --progress=dot:mega -O "$$TMP" "https://github.com/redhat-openshift-ecosystem/openshift-preflight/releases/download/$(OPENSHIFT_PREFLIGHT_VERSION)/preflight-linux-amd64"; \
		chmod +x "$$TMP"; \
		mv "$$TMP" "$(LOCAL_PREFLIGHT)"; \
	fi
endif

# Utilize Kind or modify the e2e tests to load the image locally, enabling compatibility with other vendors.
.PHONY: test-e2e  # Run the e2e tests against a Kind k8s instance that is spun up.
test-e2e:
	go test ./test/e2e/ -v -ginkgo.v

.PHONY: operator-sdk
ifeq ($(SYSTEM_OPERATOR_SDK_OK),true)
operator-sdk: ## Use the pinned operator-sdk version from PATH.
	@echo "Using system operator-sdk $(OPERATOR_SDK_VERSION): $(OPERATOR_SDK)"
else
operator-sdk: $(LOCALBIN) ## Install the pinned operator-sdk version locally when no matching system tool exists.
	@if ! test -x "$(LOCAL_OPERATOR_SDK)" || ! "$(LOCAL_OPERATOR_SDK)" version 2>&1 | grep -Fq "$(OPERATOR_SDK_VERSION)"; then \
		echo "Installing operator-sdk $(OPERATOR_SDK_VERSION) in $(LOCALBIN)"; \
		OS="$$(go env GOOS)"; \
		ARCH="$$(go env GOARCH)"; \
		TMP="$(LOCAL_OPERATOR_SDK).tmp"; \
		trap 'rm -f "$$TMP"' EXIT; \
		curl -fsSLo "$$TMP" "https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$${OS}_$${ARCH}"; \
		chmod +x "$$TMP"; \
		mv "$$TMP" "$(LOCAL_OPERATOR_SDK)"; \
	fi
endif

.PHONY: bundle
bundle: manifests kustomize operator-sdk ## Generate bundle manifests and metadata, then validate generated files.
	$(OPERATOR_SDK) generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/default/ > aic-operator-deploy.yaml
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate bundle --extra-service-accounts aic-operator-kmm-module-loader,aic-operator-kmm-device-plugin $(BUNDLE_GEN_FLAGS)
	$(OPERATOR_SDK) bundle validate ./bundle

.PHONY: bundle-build
bundle-build: bundle ## Build the bundle image.
	$(CONTAINER_TOOL) build -f operator-bundle.Dockerfile -t $(BUNDLE_IMG) --build-arg VERSION=${VERSION} .

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	@echo "Remote registry required: pushing bundle image $(BUNDLE_IMG). Set IMAGE_REGISTRY=<registry>/<namespace> explicitly."
	$(CONTAINER_TOOL) push $(BUNDLE_IMG)

.PHONY: opm
OPM ?= $(LOCALBIN)/opm-$(OPM_VERSION)
opm: $(OPM) ## Download the pinned opm version locally if necessary.
$(OPM): $(LOCALBIN)
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/$(OPM_VERSION)/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}

# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)

# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
.PHONY: catalog-build
catalog-build: opm ## Build a catalog image.
	@echo "Remote registry required: bundle image(s) must already be pushable and pullable: $(BUNDLE_IMGS)."
	$(OPM) index add --container-tool docker --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	@echo "Remote registry required: pushing catalog image $(CATALOG_IMG). Set IMAGE_REGISTRY=<registry>/<namespace> explicitly."
	$(CONTAINER_TOOL) push $(CATALOG_IMG)
