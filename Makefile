CONTROLLER_TOOLS_VERSION := v0.16.1
CONTROLLER_GEN ?= go run sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: all
all: controller storage worker

.PHOHY: test
test:
	go test ./... -coverprofile cover.out

.PHOHY: vet
vet:
	go vet ./...

.PHONY: controller
controller: vet
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/controller ./cmd/controller

.PHONY: storage
storage: vet
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/storage ./cmd/storage 

.PHONY: worker
worker: vet
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/worker ./cmd/worker

.PHONY: generate
generate: generate-controller generate-storage

.PHONY: generate-controller
generate-controller: manifests  ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/v1alpha1"

.PHONY: manifests
manifests: ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./api/v1alpha1"  paths="./internal/controller" output:crd:artifacts:config=helm/templates/crd output:rbac:artifacts:config=helm/templates/controller

.PHONY: generate-storage
generate-storage: ## Generate storage  code in pkg/generated and DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	API_KNOWN_VIOLATIONS_DIR=. UPDATE_API_KNOWN_VIOLATIONS=true ./hack/update-codegen.sh
