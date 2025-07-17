package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	pb "github.com/Azure/azure-container-networking/cns/grpc/v1alpha"
	"google.golang.org/grpc"
)

const (
	// Unix socket path for CNS
	socketPath = "/var/run/cns/grpc.sock" // Linux/Unix
	// For Windows: socketPath = `\\.\pipe\cns_grpc`

	// HTTP over Unix socket
	httpSocketPath = "/var/run/cns/http.sock"
)

// Example using gRPC over Unix Domain Socket
func exampleGRPCClient() {
	log.Println("=== gRPC over Unix Domain Socket Example ===")

	// Connect to Unix Domain Socket for gRPC
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "unix://"+socketPath,
		grpc.WithInsecure(),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Fatalf("Failed to connect to gRPC server: %v", err)
	}
	defer conn.Close()

	client := pb.NewCNSClient(conn)

	// Example 1: Assign devices to pod
	log.Println("Assigning devices to pod...")
	assignReq := &pb.AssignIBDevicesToPodRequest{
		PodID:     "my-pod-my-namespace",
		DeviceIds: []string{"60:45:bd:a4:b5:7a", "7c:1e:52:07:11:36"},
	}

	assignResp, err := client.AssignIBDevicesToPod(context.Background(), assignReq)
	if err != nil {
		log.Printf("AssignIBDevicesToPod failed: %v", err)
	} else {
		log.Printf("Assignment result: code=%d, message=%s",
			assignResp.ErrorCode, assignResp.Message)
	}

	// Example 2: Get device info
	log.Println("Getting device information...")
	infoReq := &pb.GetIBDeviceInfoRequest{
		DeviceID: "60:45:bd:a4:b5:7a",
	}

	infoResp, err := client.GetIBDeviceInfo(context.Background(), infoReq)
	if err != nil {
		log.Printf("GetIBDeviceInfo failed: %v", err)
	} else {
		log.Printf("Device info: deviceID=%s, podID=%s, status=%s, errorCode=%d",
			infoResp.DeviceID, infoResp.PodID, infoResp.Status, infoResp.ErrorCode)
	}
}

// Example using HTTP over Unix Domain Socket
func exampleHTTPClient() {
	log.Println("=== HTTP over Unix Domain Socket Example ===")

	// Create HTTP client that uses Unix socket
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", httpSocketPath)
			},
		},
		Timeout: 30 * time.Second,
	}

	// Example 1: Assign devices to pod
	log.Println("Assigning devices to pod via HTTP...")
	assignReq := cns.AssignIBDevicesToPodRequest{
		PodID:     "my-pod-my-namespace",
		DeviceIDs: []string{"60:45:bd:a4:b5:7a", "7c:1e:52:07:11:36"},
	}

	reqBody, err := json.Marshal(assignReq)
	if err != nil {
		log.Printf("Failed to marshal request: %v", err)
		return
	}

	// Use dummy URL since we're going over Unix socket
	resp, err := client.Post("http://unix/ibdevices/pod/my-pod-my-namespace",
		"application/json", bytes.NewReader(reqBody))
	if err != nil {
		log.Printf("HTTP request failed: %v", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read response: %v", err)
		return
	}

	var assignResp cns.AssignIBDevicesToPodResponse
	if err := json.Unmarshal(body, &assignResp); err != nil {
		log.Printf("Failed to unmarshal response: %v", err)
		return
	}

	log.Printf("Assignment result: code=%d, message=%s",
		assignResp.Response.ReturnCode, assignResp.Response.Message)

	// Example 2: Get device info
	log.Println("Getting device information via HTTP...")
	resp, err = client.Get("http://unix/ibdevices/60:45:bd:a4:b5:7a")
	if err != nil {
		log.Printf("HTTP request failed: %v", err)
		return
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read response: %v", err)
		return
	}

	var infoResp cns.GetIBDeviceInfoResponse
	if err := json.Unmarshal(body, &infoResp); err != nil {
		log.Printf("Failed to unmarshal response: %v", err)
		return
	}

	log.Printf("Device info: deviceID=%s, podID=%s, status=%s, errorCode=%d",
		infoResp.DeviceID, infoResp.PodID, infoResp.Status, infoResp.ErrorCode)
}

// Example configuration for enabling gRPC/UDS in CNS
func printExampleConfig() {
	log.Println("=== Example CNS Configuration ===")
	config := `
# CNS Configuration Example
cnsconfig:
  grpcSettings:
    enable: true
    address: "0.0.0.0"
    port: 10091
    servicePort: 10091
    socketPath: "/var/run/cns/grpc.sock"

  udsSettings:
    enable: true
    socketPath: "/var/run/cns/http.sock"

  # Other CNS settings...
  managedSettings:
    nodeID: "k8s-node-1"
    orchestratorType: "Kubernetes"
`
	fmt.Println(config)
}

func main() {
	log.Println("CNS IBDevice API Client Examples")
	log.Println("===============================")

	// Print example configuration
	printExampleConfig()

	// Note: These examples assume CNS is running with the appropriate configuration
	// and the Unix sockets are available. In a real scenario, you would handle
	// connection errors and implement proper retry logic.

	// Example 1: gRPC over Unix Domain Socket
	// Uncomment to test when CNS is running:
	// exampleGRPCClient()

	// Example 2: HTTP over Unix Domain Socket
	// Uncomment to test when CNS is running:
	// exampleHTTPClient()

	log.Println("Examples completed. Uncomment the function calls to test with a running CNS instance.")
}
