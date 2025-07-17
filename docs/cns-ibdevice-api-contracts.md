# CNS IBDevice API Contracts

## Overview
Two new APIs for managing InfiniBand devices in Azure Container Network Service (CNS):

1. **Assign IB Devices to Pod** - Assigns multiple InfiniBand devices to a pod
2. **Get IB Device Info** - Retrieves information about a specific InfiniBand device

These APIs support **gRPC**, **REST**, and **Unix Domain Socket** transports based on `cnsconfig.GRPCSettings.Enable` configuration.

---

## API 1: Assign InfiniBand Devices to Pod

### Endpoint
```
PUT /ibdevices/pod/{podname-podnamespace}
```

### Request Body
```json
{
  "podID": "my-pod-my-namespace",
  "deviceIds": ["60:45:bd:a4:b5:7a", "7c:1e:52:07:11:36"]
}
```

### Success Response
```json
{
  "response": {
    "errorCode": 0,
    "message": "Successfully assigned 2 devices to pod my-pod-my-namespace"
  }
}
```

### Error Response
```json
{
  "response": {
    "errorCode": 23,
    "message": "Device 60:45:bd:a4:b5:7a is already assigned to another pod"
  }
}
```

---

## API 2: Get InfiniBand Device Information

### Endpoint
```
GET /ibdevices/{mac-address-of-device}
```

### Request
No request body (MAC address provided in URL path)

### Success Response
```json
{
  "deviceID": "60:45:bd:a4:b5:7a",
  "podID": "my-pod-my-namespace", 
  "status": "assigned",
  "errorCode": 0,
  "msg": ""
}
```

### Device Not Found Response
```json
{
  "deviceID": "60:45:bd:a4:b5:7a",
  "podID": "",
  "status": "",
  "errorCode": 14,
  "msg": "Device not found"
}
```

---

## Go Data Structures
See [api.go](../cns/api.go)

- `AssignIBDevicesToPodRequest`
- `AssignIBDevicesToPodResponse`
- `GetIBDeviceInfoRequest`
- `GetIBDeviceInfoResponse`

---

## gRPC Protocol Buffers

### Service Definition
```protobuf
service CNS {
  rpc AssignIBDevicesToPod(AssignIBDevicesToPodRequest) returns (AssignIBDevicesToPodResponse);
  rpc GetIBDeviceInfo(GetIBDeviceInfoRequest) returns (GetIBDeviceInfoResponse);
}
```

### Message Definitions
```protobuf
message AssignIBDevicesToPodRequest {
  string podID = 1;                    // podname-podnamespace
  repeated string deviceIds = 2;       // MAC addresses
}

message AssignIBDevicesToPodResponse {
  int32 returnCode = 1;                // 0 for success
  string message = 2;                  // Response message
}

message GetIBDeviceInfoRequest {
  string deviceID = 1;                 // MAC address
}

message GetIBDeviceInfoResponse {
  string deviceID = 1;                 // MAC address
  string podID = 2;                    // Assigned pod
  string status = 3;                   // Device status
  int32 errorCode = 4;                 // Error code
  string msg = 5;                      // Additional message
}
```

---

## Unix Domain Socket Support

### Configuration
```yaml
cnsconfig:
  grpcSettings:
    enable: true
    socketPath: "/var/run/cns/grpc.sock"
```

### gRPC Client Example
```go
conn, err := grpc.Dial("unix:///var/run/cns/grpc.sock", grpc.WithInsecure())
client := pb.NewCNSClient(conn)

// Assign devices
resp, err := client.AssignIBDevicesToPod(ctx, &pb.AssignIBDevicesToPodRequest{
    PodID: "my-pod-my-namespace",
    DeviceIds: []string{"60:45:bd:a4:b5:7a"},
})
```

---

## Response Codes

| Code | Name              | Description                           |
|------|-------------------|---------------------------------------|
| 0    | Success           | Operation completed successfully      |
| 23    | InvalidRequest  | Invalid request parameters            |
| 14   | NotFound          | Device not found                      |
See [codes.go](../cns/types/codes.go)
- Not all of these codes are relevant, but we will take from this list

---

## Path Constants

```go
const (
    IBDevicesPodPath = "/ibdevices/pod/"  // PUT /ibdevices/pod/{podname-podnamespace}
    IBDevicesPath    = "/ibdevices/"      // GET /ibdevices/{mac-address-of-device}
)
```
