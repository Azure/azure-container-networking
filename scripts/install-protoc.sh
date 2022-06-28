#!/usr/bin/env bash

PROTOBUFF_VERSION=3.19.1
PB_REL="https://github.com/protocolbuffers/protobuf/releases"
PROTOC_ZIP_NAME=protoc-${PROTOBUFF_VERSION}-linux-x86_64.zip
PROTOC_INSTALL_PATH=$HOME/.local

# install unzip on linux if not installed
if [[ "$OSTYPE" == "linux-gnu" ]]; then
	if ! type "unzip" > /dev/null; then
		sudo apt-get install unzip
	fi
fi

# install protoc and protoc-gen-go
if [ ! -f ${PROTOC_INSTALL_PATH}/bin/protoc ]; then
    echo "Installing protobuf compiler"
		curl -LO ${PB_REL}/download/v${PROTOBUFF_VERSION}/${PROTOC_ZIP_NAME}
		echo "Unzipping protoc"
		unzip -o ${PROTOC_ZIP_NAME} -d ${PROTOC_INSTALL_PATH}
		chmod +x ${PROTOC_INSTALL_PATH}/bin/protoc 
		export PATH=${PROTOC_INSTALL_PATH}/bin:$PATH
		echo "Removing protoc zip"
		rm ${PROTOC_ZIP_NAME}
else
	echo "Protobuf compiler already installed at ${PROTOC_INSTALL_PATH}/bin/protoc"
	echo "Add protoc to your PATH if you want to use it from anywhere else using the following command:"
	export PATH=${PROTOC_INSTALL_PATH}/bin:$PATH
fi

# install protoc-gen-go
if [ ! -f ${GOPATH}/bin/protoc-gen-go ]; then
	echo "Installing protoc-gen-go"
	go install github.com/golang/protobuf/protoc-gen-go@v1.26
else
	echo "protoc-gen-go already installed at ${GOPATH}/bin/protoc-gen-go"
fi

# install protoc-gen-go-grpc
if [ ! -f ${GOPATH}/bin/protoc-gen-go-grpc ]; then
	echo "Installing protoc-gen-go-grpc"
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.1
else
	echo "protoc-gen-go-grpc already installed at ${GOPATH}/bin/protoc-gen-go-grpc"
fi
