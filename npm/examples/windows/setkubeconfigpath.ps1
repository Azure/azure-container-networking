$cpEndpoint = Get-Content C:\k\config | ForEach-Object -Process {if($_.Contains("server:")) {$_.Trim().Split()[1]}}
$token = Get-Content -Path $env:CONTAINER_SANDBOX_MOUNT_POINT\var\run\secrets\kubernetes.io\serviceaccount\token
$ca = Get-Content -Raw -Path $env:CONTAINER_SANDBOX_MOUNT_POINT\var\run\secrets\kubernetes.io\serviceaccount\ca.crt
$caBase64 = [System.Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($ca))
$server = "server: https://$cpEndpoint"
(Get-Content $env:CONTAINER_SANDBOX_MOUNT_POINT\kubeconfigtemplate.yaml).
    replace("<ca>", $caBase64).
    replace("<server>", $server.Trim()).
    replace("<token>", $token) | Set-Content $env:CONTAINER_SANDBOX_MOUNT_POINT\kubeconfig -Force
