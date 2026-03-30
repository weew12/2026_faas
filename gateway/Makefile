export DOCKER_CLI_EXPERIMENTAL=enabled
export DOCKER_BUILDKIT=1

IMG_NAME ?= gateway

TAG ?= v1
OWNER ?= weew12
SERVER ?= 172.16.2.106:5000

VERSION := $(shell git describe --tags --dirty --always)
GIT_COMMIT := $(shell git rev-parse HEAD)

BUILDER_NAME ?= multiarch
NODE_NAME ?= $(BUILDER_NAME)
BUILDKIT_CONFIG ?= ./docker_buildx_config/buildkitd.toml
PLATFORMS ?= linux/amd64,linux/arm64

# ANSI colors
RESET   := \033[0m
BOLD    := \033[1m
RED     := \033[31m
GREEN   := \033[32m
YELLOW  := \033[33m
BLUE    := \033[34m
MAGENTA := \033[35m
CYAN    := \033[36m

.PHONY: buildx-prepare
buildx-prepare:
	@printf "$(BLUE)$(BOLD)==> step 1/5: install qemu/binfmt for cross-platform builds$(RESET)\n"
	@docker run --privileged --rm tonistiigi/binfmt --install all
	@printf "$(BLUE)$(BOLD)==> step 2/5: remove old builder if exists$(RESET)\n"
	@docker buildx rm $(BUILDER_NAME) 2>/dev/null || true
	@printf "$(BLUE)$(BOLD)==> step 3/5: create buildx builder$(RESET) $(CYAN)$(BUILDER_NAME)$(RESET)\n"
	@printf "$(YELLOW)    node: $(CYAN)$(NODE_NAME)$(RESET)\n"
	@printf "$(YELLOW)    config: $(CYAN)$(BUILDKIT_CONFIG)$(RESET)\n"
	@docker buildx create \
		--name $(BUILDER_NAME) \
		--node $(NODE_NAME) \
		--driver docker-container \
		--buildkitd-config $(BUILDKIT_CONFIG) \
		--use
	@printf "$(BLUE)$(BOLD)==> step 4/5: bootstrap builder$(RESET) $(CYAN)$(BUILDER_NAME)$(RESET)\n"
	@docker buildx inspect $(BUILDER_NAME) --bootstrap
	@printf "$(BLUE)$(BOLD)==> step 5/5: supported platforms for $(CYAN)$(BUILDER_NAME)$(RESET)\n"
	@docker buildx inspect $(BUILDER_NAME) --bootstrap | sed -n '/Platforms:/p'
	@printf "$(GREEN)$(BOLD)==> builder ready: $(CYAN)$(BUILDER_NAME)$(RESET)\n"

.PHONY: buildx-push-all
buildx-push-all: buildx-prepare
	@printf "$(MAGENTA)$(BOLD)==> publish multi-arch image$(RESET)\n"
	@printf "$(YELLOW)    image: $(CYAN)$(SERVER)/$(OWNER)/$(IMG_NAME):$(TAG)$(RESET)\n"
	@printf "$(YELLOW)    platforms: $(CYAN)$(PLATFORMS)$(RESET)\n"
	@printf "$(YELLOW)    version: $(CYAN)$(VERSION)$(RESET)\n"
	@printf "$(YELLOW)    git commit: $(CYAN)$(GIT_COMMIT)$(RESET)\n"
	@docker buildx build \
		--builder $(BUILDER_NAME) \
		--progress=plain \
		--platform $(PLATFORMS) \
		--output "type=image,push=true" \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg VERSION=$(VERSION) \
		--tag $(SERVER)/$(OWNER)/$(IMG_NAME):$(TAG) \
		.
	@printf "$(GREEN)$(BOLD)==> publish done: $(CYAN)$(SERVER)/$(OWNER)/$(IMG_NAME):$(TAG)$(RESET)\n"