$ProgressPreference = 'SilentlyContinue'

Import-Module C:\k\hns.psm1 -DisableNameChecking -ErrorAction Continue
Import-Module C:\k\debug\VFP.psm1 -DisableNameChecking -ErrorAction Continue

while ($true) {
    $snapshot = [ordered]@{
        utc = [DateTime]::UtcNow.ToString('o')
        boot = (Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToUniversalTime().ToString('o')
        adapters = @(Get-NetAdapter | Select-Object Name, InterfaceDescription, InterfaceIndex, Status, MacAddress)
        bindings = @(Get-NetAdapterBinding -ComponentID vms_pp, ms_tcpip | Select-Object Name, ComponentID, Enabled)
        ipv4 = @(Get-NetIPAddress -AddressFamily IPv4 | Select-Object InterfaceIndex, IPAddress, PrefixLength, PrefixOrigin, AddressState)
        dhcp = @(Get-NetIPInterface -AddressFamily IPv4 | Select-Object InterfaceIndex, Dhcp, ConnectionState)
        dhcpEvents = @(
            foreach ($log in 'System', 'Microsoft-Windows-Dhcp-Client/Operational') {
                Get-WinEvent -FilterHashtable @{
                    LogName = $log
                    ProviderName = 'Microsoft-Windows-Dhcp-Client'
                    StartTime = (Get-Date).AddSeconds(-60)
                } -MaxEvents 12 -ErrorAction SilentlyContinue |
                    Select-Object LogName, Id, TimeCreated, Message
            }
        )
        hns = @(Get-HnsNetwork | Select-Object Name, Type, ManagementIP, CurrentEndpointCount)
        vfp = @(
            foreach ($switch in 'azure', 'ext') {
                foreach ($port in (Get-VfpPorts -SwitchName $switch)) {
                    $portName = $port.'Port name'
                    if (-not $portName) { continue }
                    $mappings = & vfpctrl.exe /list-mapping /port $portName
                    $status = $LASTEXITCODE
                    $addresses = @($mappings | ForEach-Object {
                        if ($_ -match '^\s*Provider address:\s*(\S+)') { $Matches[1] }
                    } | Sort-Object -Unique)
                    [ordered]@{ switch = $switch; port = $portName; exitCode = $status; providerAddresses = $addresses }
                }
            }
        )
    }

    $snapshot | ConvertTo-Json -Depth 6 -Compress
    Start-Sleep -Seconds 15
}
