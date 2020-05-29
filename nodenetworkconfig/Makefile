# Dependencies for generator
GOFILES = $(wildcard api/v1/*.go)

generate: $(GOFILES) controller-gen
	controller-gen object:headerFile="boilerplate.go.txt" paths="./..."

.PHONY: controller-gen

controller-gen:
	@(cd && GO111MODULE=on go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.3.0)