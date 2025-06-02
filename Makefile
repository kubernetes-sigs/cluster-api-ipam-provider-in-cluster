# Copyright 2023 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Image URL to use all building/pushing image targets
TAG ?= dev

PROD_REGISTRY ?= registry.k8s.io/capi-ipam-ic
STAGING_REGISTRY ?= gcr.io/k8s-staging-capi-ipam-ic

IMAGE_NAME ?= cluster-api-ipam-in-cluster-controller
STAGING_IMG ?= $(STAGING_REGISTRY)/$(IMAGE_NAME)

# PULL_BASE_REF is set by prow and contains the git ref for a build, e.g. branch name or tag
RELEASE_ALIAS_TAG ?= $(PULL_BASE_REF)

ARCH ?= $(shell go env GOARCH)
ALL_ARCH ?= amd64 arm64

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.29

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

HACK_BIN=$(shell pwd)/hack/bin

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
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
generate: controller-gen conversion-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
	$(CONVERSION_GEN) \
		--output-file=zz_generated.conversion.go \
		--go-header-file=./hack/boilerplate.go.txt \
		./api/v1alpha1

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out

##@ Build

.PHONY: build
build: generate fmt vet ## Build manager binary.
	go build -o bin/manager main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./main.go

.PHONY: docker-build
docker-build: licenses-report ## Build docker image with the manager.
	DOCKER_BUILDKIT=1 docker build --build-arg ARCH=$(ARCH) -t $(STAGING_IMG)-$(ARCH):$(TAG) .

.PHONY: docker-build-all
docker-build-all: $(addprefix docker-build-,$(ALL_ARCH))

docker-build-%:
	$(MAKE) ARCH=$* docker-build

.PHONY: docker-push-all
docker-push-all: $(addprefix docker-push-,$(ALL_ARCH)) docker-push-manifest

docker-push-%:
	$(MAKE) ARCH=$* docker-push

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push $(STAGING_IMG)-$(ARCH):$(TAG)

docker-push-manifest:
	docker manifest create --amend $(STAGING_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(STAGING_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} $(STAGING_IMG):$(TAG) $(STAGING_IMG)-$${arch}:$(TAG); done
	docker manifest push --purge $(STAGING_IMG):$(TAG)


##@ Release

RELEASE_TAG ?= $(shell git describe --tags --abbrev=0 2>/dev/null)

RELEASE_DIR ?= out

$(RELEASE_DIR):
	mkdir -p $(RELEASE_DIR)/

.PHONY: release
release: clean-release
	@if [ -z "${RELEASE_TAG}" ]; then echo "RELEASE_TAG is not set"; exit 1; fi
	@if ! [ -z "$$(git status --porcelain)" ]; then echo "Your local git repository contains uncommitted changes, use git clean before proceeding."; exit 1; fi
	git checkout "${RELEASE_TAG}"
	$(MAKE) manifest-modification REGISTRY=$(PROD_REGISTRY)
	$(MAKE) release-manifests
	$(MAKE) release-metadata
	$(MAKE) licenses-report
	$(MAKE) clean-release-git

.PHONY: clean-release
clean-release:
	rm -rf $(RELEASE_DIR)

.PHONY: clean-release-git
clean-release-git: ## Restores the git files usually modified during a release
	git restore ./*manager_image_patch.yaml

.PHONY: manifest-modification
manifest-modification:
	$(MAKE) set-manifest-image MANIFEST_IMG=$(REGISTRY)/$(IMAGE_NAME) MANIFEST_TAG=$(RELEASE_TAG)
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent

.PHONY: release-manifests
release-manifests: kustomize $(RELEASE_DIR)
	$(KUSTOMIZE) build config/default > $(RELEASE_DIR)/ipam-components.yaml

.PHONY: release-metadata
release-metadata:
	cp metadata.yaml $(RELEASE_DIR)/metadata.yaml

.PHONY: staging-images-release-alias-tag
staging-images-release-alias-tag: ## Add the release alias tag to the last build tag
	gcloud container images add-tag $(STAGING_IMG):$(TAG) $(STAGING_IMG):$(RELEASE_ALIAS_TAG)

.PHONY: release-staging-images
release-staging-images: docker-build-all docker-push-all staging-images-release-alias-tag

licenses-report: go-licenses
	rm -rf $(RELEASE_DIR)/licenses
	$(GO_LICENSES) save --save_path $(RELEASE_DIR)/licenses ./...
	$(GO_LICENSES) report --template hack/licenses.md.tpl ./... > $(RELEASE_DIR)/licenses/licenses.md
	(cd out/licenses && tar -czf ../licenses.tar.gz *)

##@ Release Utils

.PHONY: set-manifest-pull-policy
set-manifest-pull-policy:
	$(info Updating kustomize pull policy file for manager resources)
	sed -i'' -e 's@imagePullPolicy: .*@imagePullPolicy: '"$(PULL_POLICY)"'@' ./config/default/manager_image_patch.yaml

.PHONY: set-manifest-image
set-manifest-image:
	$(info Updating kustomize image patch file for manager resource)
	sed -i'' -e 's@image: .*@image: '"${MANIFEST_IMG}:$(MANIFEST_TAG)"'@' ./config/default/manager_image_patch.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

CONTROLLER_GEN = $(HACK_BIN)/controller-gen
.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	env GOBIN=$(HACK_BIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.17.2

KUSTOMIZE = $(HACK_BIN)/kustomize
.PHONY: kustomize
kustomize: ## Download kustomize locally if necessary.
	env GOBIN=$(HACK_BIN) go install sigs.k8s.io/kustomize/kustomize/v4@v4.5.7

ENVTEST = $(HACK_BIN)/setup-envtest
.PHONY: envtest
envtest: ## Download envtest-setup locally if necessary.
	env GOBIN=$(HACK_BIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

GO_LICENSES = $(HACK_BIN)/go-licenses
.PHONY: go-licenses
go-licenses:
	env GOBIN=$(HACK_BIN) go install github.com/google/go-licenses@latest

.PHONY: verify-boilerplate
verify-boilerplate: ## Verifies all sources have appropriate boilerplate
	./hack/verify-boilerplate.sh

CONVERSION_GEN = $(HACK_BIN)/conversion-gen
.PHONY: conversion-gen
conversion-gen: ## Download conversion-gen locally if necessary.
	env GOBIN=$(HACK_BIN) go install k8s.io/code-generator/cmd/conversion-gen@v0.30.1
