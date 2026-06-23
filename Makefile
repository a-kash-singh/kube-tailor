IMAGE_REPO ?= ghcr.io/a-kash-singh/kube-tailor
IMAGE_TAG  ?= latest

ARCH := $(shell uname -m)
ifeq ($(ARCH),x86_64)
    GOARCH ?= amd64
else ifeq ($(ARCH),aarch64)
    GOARCH ?= arm64
else ifeq ($(ARCH),arm64)
    GOARCH ?= arm64
else
    $(error "Unsupported architecture: $(ARCH)")
endif

OS := $(shell uname -s)
ifeq ($(OS),Darwin)
    GOOS ?= darwin
else ifeq ($(OS),Linux)
    GOOS ?= linux
else
    $(error "Unsupported OS: $(OS)")
endif

.PHONY: build
build:
	@echo "\n🔧  Building kube-tailor binary..."
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o bin/kube-tailor .

.PHONY: test
test:
	go test ./...

.PHONY: docker-build
docker-build:
	@echo "\n📦 Building Docker image $(IMAGE_REPO):$(IMAGE_TAG)..."
	docker buildx build \
		--build-arg GOOS=linux \
		--build-arg GOARCH=$(GOARCH) \
		--platform linux/amd64,linux/arm64 \
		-t $(IMAGE_REPO):$(IMAGE_TAG) \
		--push .

.PHONY: docker-build-local
docker-build-local:
	@echo "\n📦 Building Docker image locally..."
	docker build \
		--build-arg GOOS=linux \
		--build-arg GOARCH=$(GOARCH) \
		-t $(IMAGE_REPO):$(IMAGE_TAG) .
