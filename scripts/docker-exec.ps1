Param(
	[parameter(Mandatory=$true)]
	[string] $containerName,
	
	[parameter(Mandatory=$true)]
	[string] $namespace,
	
	[parameter(Mandatory=$true)]
	[string] $image,
	
	[parameter (Mandatory=$true)]
	[string] $command
)

$contid=''

if ( $command -eq 'ADD' ) {
	$contid=(docker run -d --name $containerName --net=none $image powershell Start-Sleep -m 1000000)
	$env:CNI_CONTAINERID=$contid
	$env:CNI_COMMAND='ADD'
} 
else {
	$contid=(docker inspect -f '{{ .Id }}' $containerName)
	$env:CNI_CONTAINERID=$contid
	$env:CNI_COMMAND='DEL'
}

$env:CNI_NETNS='none'
$env:CNI_PATH='C:\k\azurecni\bin'
$env:PATH="$env:CNI_PATH;"+$env:PATH
$k8sargs='IgnoreUnknown=1;K8S_POD_NAMESPACE={0};K8S_POD_NAME={1};K8S_POD_INFRA_CONTAINER_ID={2}' -f $namespace, $containerName, $contid
$env:CNI_ARGS=$k8sargs
$env:CNI_IFNAME='eth0'

$config=(jq-win64 '.plugins[0]' C:\k\azurecni\netconf\10-azure.conflist)
$name=(jq-win64 -r '.name' C:\k\azurecni\netconf\10-azure.conflist)
$config=(echo $config | jq-win64 --arg name $name '. + {name: $name}')
$cniVersion=(jq-win64 -r '.cniVersion' C:\k\azurecni\netconf\10-azure.conflist)
$config=(echo $config | jq-win64 --arg cniVersion $cniVersion '. + {cniVersion: $cniVersion}')

$res=(echo $config | azure-vnet)

echo $res
