$filepath = "logs_$((Get-Date).ToString('MM-dd-yyyy'))"
Write-Output "gathering logs and writing to $filepath/"

kubectl get pod -A -o wide --show-labels > ( New-Item -Path ./$filepath/allpods.out -Force )
kubectl get netpol -A -o yaml > ( New-Item -Path ./$filepath/all-netpol-yamls.out -Force )
kubectl describe netpol -A >  ( New-Item -Path ./$filepath/all-netpol-descriptions.out -Force )

$npmpod = kubectl get pod -n kube-system -owide --output=custom-columns='Name:.metadata.name,Node:spec.nodeName' | Select-String "npm-win"
$rows = @()
foreach ($row in (-split $npmpod)) {
    $rows += $row
}

for ($i = 0; $i -lt $rows.Length; $i += 2) {
    $npm = $rows[$i]
    $node = $rows[$i + 1]

    Write-Output "Gathering logs. npm pod: $npm. node: $node"
    kubectl logs -n kube-system $npm > $filepath/logs_$npm.out

    $ip = kubectl get pod -A -owide --output=custom-columns='IP:.status.podIP,Node:spec.nodeName' | Select-String "$node"
    $ip = (-split $ip)
    [string] $ips = ""
    for ($j = 0; $j -lt $ip.Length; $j += 2) {
        if($j -ne $ip.Length-2){
            $ips += $ip[$j] + " "}
            else{
                $ips += $ip[$j]
            }
    }
    echo "node $node has IPs: $ips"

    Write-Output "copying ps1 file into $npm"
    kubectl cp ./pod_exec.ps1 kube-system/"$npm":execw.ps1

    Write-Output echo "executing ps1 file on $npm"
    kubectl exec -it -n kube-system $npm -- powershell.exe -Command  .\execw.ps1 "'$ips'"

    Write-Output "copying logs.zip from $npm"
    kubectl cp kube-system/"$npm":npm-exec-logs.zip ./$filepath/npm-exec-logs_$node.zip
}

Write-Output "finished capturing all logs. written to $filepath/"
