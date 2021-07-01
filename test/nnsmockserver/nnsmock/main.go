package main

import (
	"fmt"
	"github.com/Azure/azure-container-networking/test/nnsmockserver"
)

const (
	port = "6668"
)

func main() {
	fmt.Printf("starting mock nns server ....\n")

	mockserver := nnsmockserver.NewNnsMockServer()
	mockserver.StartGrpcServer(port)
}
