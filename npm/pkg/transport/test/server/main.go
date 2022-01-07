package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Azure/azure-container-networking/npm/pkg/transport"
	"github.com/fatih/structs"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	port = 8080
)

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		cancel()
	}()

	m := transport.NewManager(ctx, port)

	ch := m.InputChannel()

	go func() {
		for _, d := range testData() {
			ch <- d
			//nolint:gomnd //ignore for test
			time.Sleep(time.Second * 5)
		}
	}()

	if err := m.Start(); err != nil {
		panic(err)
	}
}

func testData() []*structpb.Struct {
	var output []*structpb.Struct
	for i := 0; i < 10; i++ {
		v := struct {
			Type    string
			Payload string
		}{
			Type: fmt.Sprintf("IPSET-%d", i),
			//nolint:gosec //ignore for test
			Payload: fmt.Sprintf("172.17.0.%d/%d", i, rand.Uint32()%32), //nolint:gomnd //ignore for test
		}

		m := structs.Map(v)

		data, _ := structpb.NewStruct(m)
		output = append(output, data)
	}
	return output
}
