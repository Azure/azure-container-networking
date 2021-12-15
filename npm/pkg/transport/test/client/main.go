package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Azure/azure-container-networking/npm/pkg/transport"
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

	c, err := transport.NewDataplaneEventsClient(ctx, "podname", "nodename", "127.0.0.1:8080")
	if err != nil {
		log.Fatal(err)
	}
	if err := c.Start(ctx); err != nil {
		log.Fatal(err)
	}
}
