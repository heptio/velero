# Copyright 2016 The Kubernetes Authors.
#
# Modifications Copyright 2020 the Velero contributors.
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

# The binary to build (just the basename).
BIN ?= velero

# This repo's root import path (under GOPATH).
PKG := github.com/vmware-tanzu/velero

# Where to push the docker image.
REGISTRY ?= velero

# Image name
IMAGE ?= $(REGISTRY)/$(BIN)

# We allow the Dockerfile to be configurable to enable the use of custom Dockerfiles
# that pull base images from different registries.
VELERO_DOCKERFILE ?= Dockerfile
BUILDER_IMAGE_DOCKERFILE ?= hack/build-image/Dockerfile

# Calculate the realpath of the build-image Dockerfile as we `cd` into the hack/build
# directory before this Dockerfile is used and any relative path will not be valid.
BUILDER_IMAGE_DOCKERFILE_REALPATH := $(shell realpath $(BUILDER_IMAGE_DOCKERFILE))

# Build image handling. We push a build image for every changed version of
# /hack/build-image/Dockerfile. We tag the dockerfile with the short commit hash
# of the commit that changed it. When determining if there is a build image in
# the registry to use we look for one that matches the current "commit" for the
# Dockerfile else we make one.
# In the case where the Dockerfile for the build image has been overridden using
# the BUILDER_IMAGE_DOCKERFILE variable, we always force a build.

ifneq "$(origin BUILDER_IMAGE_DOCKERFILE)" "file"
	BUILDER_IMAGE_TAG := "custom"
else
	BUILDER_IMAGE_TAG := $(shell git log -1 --pretty=%h $(BUILDER_IMAGE_DOCKERFILE))
endif

BUILDER_IMAGE := $(REGISTRY)/build-image:$(BUILDER_IMAGE_TAG)
BUILDER_IMAGE_CACHED := $(shell docker images -q ${BUILDER_IMAGE} 2>/dev/null )

HUGO_IMAGE := hugo-builder

# Which architecture to build - see $(ALL_ARCH) for options.
# if the 'local' rule is being run, detect the ARCH from 'go env'
# if it wasn't specified by the caller.
local : ARCH ?= $(shell go env GOOS)-$(shell go env GOARCH)
ARCH ?= linux-amd64

VERSION ?= main

TAG_LATEST ?= false

ifeq ($(TAG_LATEST), true)
	IMAGE_TAGS ?= $(IMAGE):$(VERSION) $(IMAGE):latest
else
	IMAGE_TAGS ?= $(IMAGE):$(VERSION)
endif

ifeq ($(shell docker buildx inspect 2>/dev/null | awk '/Status/ { print $$2 }'), running)
	BUILDX_ENABLED ?= true
else
	BUILDX_ENABLED ?= false
endif

define BUILDX_ERROR
buildx not enabled, refusing to run this recipe
see: https://velero.io/docs/main/build-from-source/#making-images-and-updating-velero for more info
endef

# The version of restic binary to be downloaded for power architecture
RESTIC_VERSION ?= 0.12.0

CLI_PLATFORMS ?= linux-amd64 linux-arm linux-arm64 darwin-amd64 windows-amd64 linux-ppc64le
BUILDX_PLATFORMS ?= $(subst -,/,$(ARCH))
BUILDX_OUTPUT_TYPE ?= docker

# set git sha and tree state
GIT_SHA = $(shell git rev-parse HEAD)
ifneq ($(shell git status --porcelain 2> /dev/null),)
	GIT_TREE_STATE ?= dirty
else
	GIT_TREE_STATE ?= clean
endif

# The default linters used by lint and local-lint
LINTERS ?= "gosec,goconst,gofmt,goimports,unparam"

###
### These variables should not need tweaking.
###

platform_temp = $(subst -, ,$(ARCH))
GOOS = $(word 1, $(platform_temp))
GOARCH = $(word 2, $(platform_temp))
GOPROXY ?= https://proxy.golang.org

# If you want to build all binaries, see the 'all-build' rule.
# If you want to build all containers, see the 'all-containers' rule.
all:
	@$(MAKE) build
	@$(MAKE) build BIN=velero-restic-restore-helper

build-%:
	@$(MAKE) --no-print-directory ARCH=$* build
	@$(MAKE) --no-print-directory ARCH=$* build BIN=velero-restic-restore-helper

all-build: $(addprefix build-, $(CLI_PLATFORMS))

all-containers: container-builder-env
	@$(MAKE) --no-print-directory container
	@$(MAKE) --no-print-directory container BIN=velero-restic-restore-helper

local: build-dirs
	GOOS=$(GOOS) \
	GOARCH=$(GOARCH) \
	VERSION=$(VERSION) \
	REGISTRY=$(REGISTRY) \
	PKG=$(PKG) \
	BIN=$(BIN) \
	GIT_SHA=$(GIT_SHA) \
	GIT_TREE_STATE=$(GIT_TREE_STATE) \
	OUTPUT_DIR=$$(pwd)/_output/bin/$(GOOS)/$(GOARCH) \
	./hack/build.sh

build: _output/bin/$(GOOS)/$(GOARCH)/$(BIN)

_output/bin/$(GOOS)/$(GOARCH)/$(BIN): build-dirs
	@echo "building: $@"
	$(MAKE) shell CMD="-c '\
		GOOS=$(GOOS) \
		GOARCH=$(GOARCH) \
		VERSION=$(VERSION) \
		REGISTRY=$(REGISTRY) \
		PKG=$(PKG) \
		BIN=$(BIN) \
		GIT_SHA=$(GIT_SHA) \
		GIT_TREE_STATE=$(GIT_TREE_STATE) \
		OUTPUT_DIR=/output/$(GOOS)/$(GOARCH) \
		./hack/build.sh'"

TTY := $(shell tty -s && echo "-t")

# Example: make shell CMD="date > datefile"
shell: build-dirs build-env
	@# bind-mount the Velero root dir in at /github.com/vmware-tanzu/velero
	@# because the Kubernetes code-generator tools require the project to
	@# exist in a directory hierarchy ending like this (but *NOT* necessarily
	@# under $GOPATH).
	@docker run \
		-e GOFLAGS \
		-i $(TTY) \
		--rm \
		-u $$(id -u):$$(id -g) \
		-v "$$(pwd):/github.com/vmware-tanzu/velero:delegated" \
		-v "$$(pwd)/_output/bin:/output:delegated" \
		-v "$$(pwd)/.go/pkg:/go/pkg:delegated" \
		-v "$$(pwd)/.go/std:/go/std:delegated" \
		-v "$$(pwd)/.go/std/$(GOOS)/$(GOARCH):/usr/local/go/pkg/$(GOOS)_$(GOARCH)_static:delegated" \
		-v "$$(pwd)/.go/go-build:/.cache/go-build:delegated" \
		-v "$$(pwd)/.go/golangci-lint:/.cache/golangci-lint:delegated" \
		-w /github.com/vmware-tanzu/velero \
		$(BUILDER_IMAGE) \
		/bin/sh $(CMD)

container-builder-env:
ifneq ($(BUILDX_ENABLED), true)
	$(error $(BUILDX_ERROR))
endif
	@docker buildx build \
	--target=builder-env \
	--build-arg=GOPROXY=$(GOPROXY) \
	--build-arg=PKG=$(PKG) \
	--build-arg=VERSION=$(VERSION) \
	--build-arg=GIT_SHA=$(GIT_SHA) \
	--build-arg=GIT_TREE_STATE=$(GIT_TREE_STATE) \
	--build-arg=REGISTRY=$(REGISTRY) \
	-f $(VELERO_DOCKERFILE) .

container:
ifneq ($(BUILDX_ENABLED), true)
	$(error $(BUILDX_ERROR))
endif
	@docker buildx build --pull \
	--output=type=$(BUILDX_OUTPUT_TYPE) \
	--platform $(BUILDX_PLATFORMS) \
	$(addprefix -t , $(IMAGE_TAGS)) \
	--build-arg=PKG=$(PKG) \
	--build-arg=BIN=$(BIN) \
	--build-arg=VERSION=$(VERSION) \
	--build-arg=GIT_SHA=$(GIT_SHA) \
	--build-arg=GIT_TREE_STATE=$(GIT_TREE_STATE) \
	--build-arg=REGISTRY=$(REGISTRY) \
	--build-arg=RESTIC_VERSION=$(RESTIC_VERSION) \
	-f $(VELERO_DOCKERFILE) .
	@echo "container: $(IMAGE):$(VERSION)"

SKIP_TESTS ?=
test: build-dirs
ifneq ($(SKIP_TESTS), 1)
	@$(MAKE) shell CMD="-c 'hack/test.sh $(WHAT)'"
endif

test-local: build-dirs
ifneq ($(SKIP_TESTS), 1)
	hack/test.sh $(WHAT)
endif

verify:
ifneq ($(SKIP_TESTS), 1)
	@$(MAKE) shell CMD="-c 'hack/verify-all.sh'"
endif

lint:
ifneq ($(SKIP_TESTS), 1)
	@$(MAKE) shell CMD="-c 'hack/lint.sh $(LINTERS)'"
endif

local-lint:
ifneq ($(SKIP_TESTS), 1)
	@hack/lint.sh $(LINTERS)
endif

lint-all:
ifneq ($(SKIP_TESTS), 1)
	@$(MAKE) shell CMD="-c 'hack/lint.sh $(LINTERS) true'"
endif

local-lint-all:
ifneq ($(SKIP_TESTS), 1)
	@hack/lint.sh $(LINTERS) true
endif

update:
	@$(MAKE) shell CMD="-c 'hack/update-all.sh'"

build-dirs:
	@mkdir -p _output/bin/$(GOOS)/$(GOARCH)
	@mkdir -p .go/src/$(PKG) .go/pkg .go/bin .go/std/$(GOOS)/$(GOARCH) .go/go-build .go/golangci-lint

build-env:
	@# if we have overridden the value for the build-image Dockerfile,
	@# force a build using that Dockerfile
	@# if we detect changes in dockerfile force a new build-image
	@# else if we dont have a cached image make one
	@# finally use the cached image
ifneq "$(origin BUILDER_IMAGE_DOCKERFILE)" "file"
	@echo "Dockerfile for builder image has been overridden to $(BUILDER_IMAGE_DOCKERFILE)"
	@echo "Preparing a new builder-image"
	$(MAKE) build-image
else ifneq ($(shell git diff --quiet HEAD -- $(BUILDER_IMAGE_DOCKERFILE); echo $$?), 0)
	@echo "Local changes detected in $(BUILDER_IMAGE_DOCKERFILE)"
	@echo "Preparing a new builder-image"
	$(MAKE) build-image
else ifneq ($(BUILDER_IMAGE_CACHED),)
	@echo "Using Cached Image: $(BUILDER_IMAGE)"
else
	@echo "Trying to pull build-image: $(BUILDER_IMAGE)"
	docker pull -q $(BUILDER_IMAGE) || $(MAKE) build-image
endif

build-image:
	@# When we build a new image we just untag the old one.
	@# This makes sure we don't leave the orphaned image behind.
	$(eval old_id=$(shell docker image inspect  --format '{{ .ID }}' ${BUILDER_IMAGE} 2>/dev/null))
ifeq ($(BUILDX_ENABLED), true)
	@cd hack/build-image && docker buildx build --build-arg=GOPROXY=$(GOPROXY) --output=type=docker --pull -t $(BUILDER_IMAGE) -f $(BUILDER_IMAGE_DOCKERFILE_REALPATH) .
else
	@cd hack/build-image && docker build --build-arg=GOPROXY=$(GOPROXY) --pull -t $(BUILDER_IMAGE) -f $(BUILDER_IMAGE_DOCKERFILE_REALPATH) .
endif
	$(eval new_id=$(shell docker image inspect  --format '{{ .ID }}' ${BUILDER_IMAGE} 2>/dev/null))
	@if [ "$(old_id)" != "" ] && [ "$(old_id)" != "$(new_id)" ]; then \
		docker rmi -f $$id || true; \
	fi

push-build-image:
	@# this target will push the build-image it assumes you already have docker
	@# credentials needed to accomplish this.
	@# Pushing will be skipped if a custom Dockerfile was used to build the image.
	ifneq "$(origin BUILDER_IMAGE_DOCKERFILE)" "file"
		@echo "Dockerfile for builder image has been overridden"
		@echo "Skipping push of custom image"
	else
		docker push $(BUILDER_IMAGE)
	endif

build-image-hugo:
	cd site && docker build --pull -t $(HUGO_IMAGE) .

clean:
# if we have a cached image then use it to run go clean --modcache
# this test checks if we there is an image id in the BUILDER_IMAGE_CACHED variable.
ifneq ($(strip $(BUILDER_IMAGE_CACHED)),)
	$(MAKE) shell CMD="-c 'go clean --modcache'"
	docker rmi -f $(BUILDER_IMAGE) || true
endif
	rm -rf .go _output
	docker rmi $(HUGO_IMAGE)


.PHONY: modules
modules:
	go mod tidy


.PHONY: verify-modules
verify-modules: modules
	@if !(git diff --quiet HEAD -- go.sum go.mod); then \
		echo "go module files are out of date, please commit the changes to go.mod and go.sum"; exit 1; \
	fi


ci: verify-modules verify all test


changelog:
	hack/release-tools/changelog.sh

# release builds a GitHub release using goreleaser within the build container.
#
# To dry-run the release, which will build the binaries/artifacts locally but
# will *not* create a GitHub release:
#		GITHUB_TOKEN=an-invalid-token-so-you-dont-accidentally-push-release \
#		RELEASE_NOTES_FILE=changelogs/CHANGELOG-1.2.md \
#		PUBLISH=false \
#		make release
#
# To run the release, which will publish a *DRAFT* GitHub release in github.com/vmware-tanzu/velero 
# (you still need to review/publish the GitHub release manually):
#		GITHUB_TOKEN=your-github-token \ 
#		RELEASE_NOTES_FILE=changelogs/CHANGELOG-1.2.md \
#		PUBLISH=true \
#		make release
release:
	$(MAKE) shell CMD="-c '\
		GITHUB_TOKEN=$(GITHUB_TOKEN) \
		RELEASE_NOTES_FILE=$(RELEASE_NOTES_FILE) \
		PUBLISH=$(PUBLISH) \
		REGISTRY=$(REGISTRY) \
		./hack/release-tools/goreleaser.sh'"

serve-docs: build-image-hugo
	docker run \
	--rm \
	-v "$$(pwd)/site:/srv/hugo" \
	-it -p 1313:1313 \
	$(HUGO_IMAGE) \
	hugo server --bind=0.0.0.0 --enableGitInfo=false
# gen-docs generates a new versioned docs directory under site/content/docs. 
# Please read the documentation in the script for instructions on how to use it.
gen-docs:
	@hack/release-tools/gen-docs.sh

.PHONY: test-e2e
test-e2e: local
	$(MAKE) -C test/e2e run
