GOLANGCI_LINT_VERSION := v1.61.0
CONTROLLER_TOOLS_VERSION := v0.16.5
ENVTEST_VERSION := release-0.19
ENVTEST_K8S_VERSION := 1.31.0

GOLANGCI_LINT ?= go run github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
CONTROLLER_GEN ?= go run sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)
ENVTEST ?= go run sigs.k8s.io/controller-runtime/tools/setup-envtest@$(ENVTEST_VERSION)

ENVTEST_DIR ?= $(shell pwd)/.envtest

.PHONY: all
all: controller storage worker

.PHONY: test
test: vet ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(ENVTEST_DIR) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: fmt
fmt:
	go fmt ./...

.PHOHY: lint
lint:
	$(GOLANGCI_LINT) run 

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
generate: generate-controller generate-storage generate-mocks

.PHONY: generate-controller
generate-controller: manifests  ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/v1alpha1"

.PHONY: manifests
manifests: ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects. We use yq to modify the generated files to match our naming and labels conventions.
	$(CONTROLLER_GEN) rbac:roleName=controller-role crd webhook paths="./api/v1alpha1"  paths="./internal/controller" output:crd:artifacts:config=helm/templates/crd output:rbac:artifacts:config=helm/templates/controller
	sed -i 's/controller-role/{{ include "sbombastic.fullname" . }}-controller/' helm/templates/controller/role.yaml
	sed -i '/metadata:/a\  labels:\n    {{ include "sbombastic.labels" (merge . (dict "Component" "controller")) | nindent 4 }}' helm/templates/controller/role.yaml

.PHONY: generate-storage-test-crd
generate-storage-test-crd: ## Generate CRD used by the controller tests to access the storage resources. This is needed since storage does not provide CRD, being an API server extension.
	$(CONTROLLER_GEN) crd paths="./api/storage/..." output:crd:artifacts:config=test/crd

.PHONY: generate-storage
generate-storage: generate-storage-test-crd ## Generate storage  code in pkg/generated and DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	API_KNOWN_VIOLATIONS_DIR=. UPDATE_API_KNOWN_VIOLATIONS=true ./hack/update-codegen.sh

.PHONY: generate-mocks
generate-mocks: ## Generate mocks for testing.
	go generate ./...
