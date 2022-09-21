param([string]$podIps)
$filepath = "logs"

$podIp = @()
foreach ($r in ($podIps -split " ")) {
    $podIp += $r
}

 (Get-HnsNetwork | ? Name -Like azure).Policies >>( New-Item -Path ./$filepath/hns_state.out -Force )
 Get-HnsEndpoint | ConvertTo-Json >> $filepath/hns_state.out


 foreach ($row in $podIp) {    
    Write-Output "Gathering logs for IP $row"
    [string]$endpoint = hnsdiag list endpoints | select-string -context 2, 0 "$row"
    if($endpoint -ne $null){
        $endpointID = $endpoint.Substring($endpoint.IndexOf(":")+2,37).Trim()
        hnsdiag list endpoints | select-string -context 2, 0 "$row" >> $filepath/vfp_state_$row.out     
        vfpctrl /port $endpointID /list-tag >> $filepath/vfp_state_$row.out
        vfpctrl /port $endpointID /layer ACL_ENDPOINT_LAYER /list-rule >> $filepath/vfp_state_$row.out
    }
 }

 Compress-Archive -Path 'logs' -DestinationPath 'logs.zip' -Force

exit
