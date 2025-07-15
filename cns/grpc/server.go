package grpc

import (
	"fmt"
	"log"
	"net"
	"strconv"

	pb "github.com/Azure/azure-container-networking/cns/grpc/v1alpha"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// Server struct to hold the gRPC server settings and the CNS service.
type Server struct {
	Settings   ServerSettings
	CnsService pb.CNSServer
	Logger     *zap.Logger
}

// GrpcServerSettings holds the gRPC server settings.
type ServerSettings struct {
	IPAddress  string
	Port       uint16
	SocketPath string
}

// NewServer initializes a new gRPC server instance.
func NewServer(settings ServerSettings, cnsService pb.CNSServer, logger *zap.Logger) (*Server, error) {
	if cnsService == nil {
		ErrCNSServiceNotDefined := errors.New("CNS service is not defined")
		return nil, fmt.Errorf("Failed to create new gRPC server: %w", ErrCNSServiceNotDefined)
	}

	server := &Server{
		Settings:   settings,
		CnsService: cnsService,
		Logger:     logger,
	}

	return server, nil
}

// Start starts the gRPC server.
// Now this has the capability of listening over both TCP and Unix Domain Sockets.
// If both IPAddress and DomainSocketPath are provided, it will prioritize TCP.
func (s *Server) Start() error {
	if s.Settings.IPAddress != "" || s.Settings.Port != 0 {
		address := net.JoinHostPort(s.Settings.IPAddress, strconv.FormatUint(uint64(s.Settings.Port), 10))
		lis, err := net.Listen("tcp", address)
		if err != nil {
			log.Printf("[Listener] Failed to listen on gRPC endpoint: %+v", err)
			return fmt.Errorf("failed to listen on address %s: %w", address, err)
		}
		log.Printf("[Listener] Started listening on gRPC endpoint %s.", address)

		grpcServer := grpc.NewServer()
		pb.RegisterCNSServer(grpcServer, s.CnsService)

		// Register reflection service on gRPC server.
		reflection.Register(grpcServer)

		if err := grpcServer.Serve(lis); err != nil {
			return fmt.Errorf("failed to serve gRPC server: %w", err)
		}
	} else if s.Settings.SocketPath != "" {
		lis, err := net.Listen("unix", s.Settings.SocketPath)
		if err != nil {
			log.Printf("[Listener] Failed to listen on gRPC Unix Domain Socket: %+v", err)
			return fmt.Errorf("failed to listen on Unix Domain Socket %s: %w", s.Settings.SocketPath, err)
		}
		log.Printf("[Listener] Started listening on gRPC Unix Domain Socket %s.", s.Settings.SocketPath)

		grpcServer := grpc.NewServer()
		pb.RegisterCNSServer(grpcServer, s.CnsService)

		// Register reflection service on gRPC server.
		reflection.Register(grpcServer)

		if err := grpcServer.Serve(lis); err != nil {
			return fmt.Errorf("failed to serve gRPC server: %w", err)
		}
	} else {
		return fmt.Errorf("no valid gRPC server settings provided: IP address or domain socket path must be specified")
	}
	return nil
}
