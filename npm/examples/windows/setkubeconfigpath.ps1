$server = Get-Content C:\k\config | ForEach-Object -Process {if($_.Contains("server:")) {$_.Trim().Split()[1]}}
$token = Get-Content $env:CONTAINER_SANDBOX_MOUNT_POINT\var\run\secrets\kubernetes.io\serviceaccount\token
$ca = Get-Content $env:CONTAINER_SANDBOX_MOUNT_POINT\var\run\secrets\kubernetes.io\serviceaccount\ca.crt
echo $token
echo $ca
echo $server
(Get-Content $env:CONTAINER_SANDBOX_MOUNT_POINT\kubeconfigtemplate.yaml).
    replace("<ca>", $ca).
    replace("<server>", $server.Trim()).
    replace("<token>", $token) | Set-Content $env:CONTAINER_SANDBOX_MOUNT_POINT\kubeconfig -Force
