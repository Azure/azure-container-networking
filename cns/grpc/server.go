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
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// Server struct to hold the gRPC server settings and the CNS service.
type Server struct {
	Settings     ServerSettings
	CnsService   pb.CNSServer
	Logger       *zap.Logger
	HealthServer *health.Server
}

// GrpcServerSettings holds the gRPC server settings.
type ServerSettings struct {
	IPAddress string
	Port      uint16
}

// NewServer initializes a new gRPC server instance.
func NewServer(settings ServerSettings, cnsService pb.CNSServer, logger *zap.Logger) (*Server, error) {
	if cnsService == nil {
		ErrCNSServiceNotDefined := errors.New("CNS service is not defined")
		return nil, fmt.Errorf("Failed to create new gRPC server: %w", ErrCNSServiceNotDefined)
	}

	healthServer := health.NewServer()

	server := &Server{
		Settings:     settings,
		CnsService:   cnsService,
		Logger:       logger,
		HealthServer: healthServer,
	}

	return server, nil
}

// Start starts the gRPC server.
func (s *Server) Start() error {
	address := net.JoinHostPort(s.Settings.IPAddress, strconv.FormatUint(uint64(s.Settings.Port), 10))
	lis, err := net.Listen("tcp", address)
	if err != nil {
		log.Printf("[Listener] Failed to listen on gRPC endpoint: %+v", err)
		return fmt.Errorf("failed to listen on address %s: %w", address, err)
	}
	log.Printf("[Listener] Started listening on gRPC endpoint %s.", address)

	grpcServer := grpc.NewServer()
	pb.RegisterCNSServer(grpcServer, s.CnsService)
	healthgrpc.RegisterHealthServer(grpcServer, s.HealthServer)

	// Register reflection service on gRPC server.
	reflection.Register(grpcServer)

	// Set initial health status
	s.HealthServer.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)

	if err := grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve gRPC server: %w", err)
	}

	return nil
}

// UpdateHealthStatus updates the health status of the gRPC server.
func (s *Server) UpdateHealthStatus(status healthgrpc.HealthCheckResponse_ServingStatus) {
	s.HealthServer.SetServingStatus("", status)
}
