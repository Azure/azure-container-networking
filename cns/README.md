# Azure Container Networking Service (CNS)

Azure Container Networking Service (CNS) is a service that provides container networking capabilities for Azure environments. It manages IP address allocation, network policy enforcement, and container network configuration.

## Features

- **IP Address Management (IPAM)**: Dynamic allocation and management of IP addresses for containers
- **Network Container Management**: Creation and management of network containers
- **Multi-tenancy Support**: Isolation and management of network resources across multiple tenants
- **Kubernetes Integration**: Native integration with Kubernetes for CNI plugin support
- **Windows Service Support**: Run CNS as a Windows service for automatic startup and recovery

## Running CNS

### Linux

On Linux systems, CNS is typically run as a daemon or managed by systemd:

```bash
./azure-cns [OPTIONS]
```

### Windows

On Windows systems, CNS can be run as a standalone executable or registered as a Windows service.

#### As a Windows Service

CNS can be installed as a Windows service to enable automatic startup and recovery:

```powershell
# Install the service
azure-cns.exe --service install

# Start the service
net start azure-cns
```

See [Windows Service Documentation](doc/windows-service.md) for detailed information on Windows service features and management.

#### As a Standalone Executable

You can also run CNS directly:

```powershell
azure-cns.exe [OPTIONS]
```

## Configuration

CNS can be configured using:
- Command-line arguments
- Configuration file (JSON format)
- Environment variables

### Common Command-Line Options

```
  -e, --environment=           Set the operating environment {azure,mas,fileIpam}
  -l, --log-level=info         Set the logging level {info,debug}
  -t, --log-target=logfile     Set the logging target {syslog,stderr,logfile,stdout,stdoutfile}
  -c, --cns-url=               Set the URL for CNS to listen on
  -p, --cns-port=              Set the URL port for CNS to listen on
  -cp, --config-path=          Path to cns config file
  -v, --version                Print version information
  -s, --service=               Windows service action: install, uninstall, or run as service (Windows only)
```

For a complete list of options, run:

```bash
azure-cns --help
```

## Building CNS

To build CNS from source:

```bash
cd cns/service
go build -o azure-cns
```

Or use the Makefile from the repository root:

```bash
make azure-cns-binary
```

## Documentation

- [Windows Service Documentation](doc/windows-service.md) - Detailed guide for running CNS as a Windows service
- [Swift V2 Features](../docs/feature/swift-v2/cns.md) - Swift V2 networking features
- [Async Delete](../docs/feature/async-delete/cns.md) - Asynchronous pod deletion

## API Documentation

CNS exposes a REST API for container network management. The API documentation can be found in the [swagger.yaml](swagger.yaml) file.

## Development

### Testing

Run tests using:

```bash
cd cns
go test ./...
```

### Linting

Format and lint the code:

```bash
make fmt
make lint
```

## Support

For issues and questions:
- Create an issue in the [GitHub repository](https://github.com/Azure/azure-container-networking/issues)
- Review existing [documentation](../docs/)

## License

This project is licensed under the MIT License - see the [LICENSE](../LICENSE) file for details.
