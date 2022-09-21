$filepath = "logs_$((Get-Date).ToString('MM-dd-yyyy'))"
kubectl get pod -A -o wide >> ( New-Item -Path ./$filepath/allpods.out -Force )
$npmpod = kubectl get pod -n kube-system -owide --output=custom-columns='Name:.metadata.name,Node:spec.nodeName' | Select-String "npm-win"
$rows = @()
foreach ($row in (-split $npmpod)) {
    $rows += $row
}

for ($i = 0; $i -lt $rows.Length; $i += 2) {
    $npm = $rows[$i]
    $node = $rows[$i + 1]

    Write-Output "Gathering logs for node $node"

    $ip = kubectl get pod -n kube-system -owide --output=custom-columns='IP:.status.podIP,Node:spec.nodeName' | Select-String "$node"
    $ip = (-split $ip)
    [string] $ips = ""
    for ($j = 0; $j -lt $ip.Length; $j += 2) {
        if($j -ne $ip.Length-2){
            $ips += $ip[$j] + " "}
            else{
                $ips += $ip[$j]
            }
    }  
    
    kubectl logs -n kube-system $npm >> $filepath/logs_$npm.out
    kubectl cp ./pod_exec.ps1 kube-system/"$npm":execw.ps1
    kubectl exec -it -n kube-system $npm -- powershell.exe -Command  .\execw.ps1 "'$ips'"
    kubectl cp kube-system/"$npm":logs.zip ./$filepath/logs_$node.zip
}
