# Azure CNS Windows Service

Azure CNS (Container Networking Service) can be registered and run as a Windows service, enabling automatic startup on system boot and automatic restart on failure.

## Features

- **Automatic Startup**: CNS starts automatically when the Windows system boots
- **Automatic Recovery**: Service automatically restarts on failure (with 5-second delays)
- **Event Logging**: Service lifecycle events are logged to Windows Event Log
- **Service Management**: Easy installation and uninstallation via command-line flags

## Installation

To install Azure CNS as a Windows service, run the following command with administrator privileges:

```powershell
azure-cns.exe --service install
```

or using the short form:

```powershell
azure-cns.exe -s install
```

This will:
1. Register the service with the Windows Service Control Manager
2. Configure the service to start automatically on system boot
3. Set up automatic restart on failure
4. Create an event log source for CNS

After installation, you can start the service using:

```powershell
net start azure-cns
```

or via the Services management console (`services.msc`).

## Uninstallation

To uninstall the Azure CNS Windows service, run the following command with administrator privileges:

```powershell
azure-cns.exe --service uninstall
```

or using the short form:

```powershell
azure-cns.exe -s uninstall
```

This will:
1. Stop the service if it's running
2. Remove the service from the Windows Service Control Manager
3. Remove the event log source

## Service Configuration

The service is registered with the following configuration:

- **Service Name**: `azure-cns`
- **Display Name**: `Azure Container Networking Service`
- **Description**: `Provides container networking services for Azure`
- **Start Type**: Automatic
- **Service Account**: LocalSystem
- **Recovery Actions**:
  - First failure: Restart the service after 5 seconds
  - Second failure: Restart the service after 5 seconds
  - Subsequent failures: Restart the service after 5 seconds
  - Reset failure count after: 24 hours

## Running as a Service

Once installed, the service will automatically detect when it's being started by the Windows Service Control Manager and will run in service mode. No additional command-line flags are needed when the service is started by Windows.

For testing purposes, you can explicitly run in service mode using:

```powershell
azure-cns.exe --service run
```

## Event Logging

Service lifecycle events are logged to the Windows Event Log under the Application log with the source name `azure-cns`. You can view these logs using:

```powershell
Get-EventLog -LogName Application -Source azure-cns -Newest 20
```

or via the Event Viewer (`eventvwr.msc`).

## Troubleshooting

### Service fails to install

Ensure you are running the command prompt or PowerShell as Administrator. The service installation requires elevated privileges.

### Service fails to start

1. Check the Windows Event Log for error messages:
   ```powershell
   Get-EventLog -LogName Application -Source azure-cns -Newest 10
   ```

2. Verify that all required configuration files are present

3. Check that the executable path is correct

4. Try running the executable directly (not as a service) to identify any configuration issues:
   ```powershell
   azure-cns.exe
   ```

### Service doesn't restart on failure

The service is configured to restart automatically up to 3 times within a 24-hour period. If the service continues to fail, it will remain stopped. Check the Event Log for the root cause and fix the underlying issue before restarting the service.

## Command-Line Reference

```
  -s, --service=               Windows service action: install, uninstall, or run as service {install,uninstall,run,}
```

**Available actions:**
- `install`: Install Azure CNS as a Windows service
- `uninstall`: Uninstall the Azure CNS Windows service
- `run`: Explicitly run in service mode (typically not needed)
- ` ` (empty): Normal execution mode (auto-detects if running as service)

## Examples

### Install and start the service

```powershell
# Install the service
azure-cns.exe --service install

# Start the service
net start azure-cns

# Verify the service is running
Get-Service azure-cns
```

### Stop and uninstall the service

```powershell
# Stop the service
net stop azure-cns

# Uninstall the service
azure-cns.exe --service uninstall
```

### Check service status

```powershell
# Using PowerShell
Get-Service azure-cns

# Using sc.exe
sc query azure-cns
```

### View service logs

```powershell
# View recent events
Get-EventLog -LogName Application -Source azure-cns -Newest 20 | Format-Table -AutoSize

# View only errors
Get-EventLog -LogName Application -Source azure-cns -EntryType Error -Newest 10
```

## Notes

- This feature is only available on Windows platforms. On Linux, use systemd or other init systems to manage the service.
- The service must be installed with administrator privileges.
- The service runs under the LocalSystem account, which has high privileges. Ensure the executable and configuration files are properly secured.
- Service configuration (command-line flags, config files) should be set via the Windows Registry or by using command-line parameters in the service's "Image Path" in the service properties.
