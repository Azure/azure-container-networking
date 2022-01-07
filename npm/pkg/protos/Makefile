REPO_ROOT = $(shell git rev-parse --show-toplevel)
PROTOC_INSTALL_PATH=$(HOME)/.local
PROTOC_BIN=$(PROTOC_INSTALL_PATH)/bin/protoc

.PHONY: generate

generate: $(PROTOC_BIN) ## Generate mock clients
	$(PROTOC_BIN) --proto_path=. --go_out=. --go-grpc_out=. --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative transport.proto 
