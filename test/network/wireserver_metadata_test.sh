kubectl run wget -it --rm --image busybox --restart Never -- wget --timeout=3 --header=Metadata:true "http://168.63.129.16/machine/plugins?comp=nmagent&type=getinterfaceinfov1"
if [ $? -eq 0 ]; then
    echo "wireserver connectivity expected to fail but succeeded"
    exit 1
fi

kubectl run wget -it --rm --image busybox --restart Never -- wget --timeout=3 --header=Metadata:true "http://169.254.169.254/metadata/instance?api-version=2021-02-01"
if [ $? -ne 0 ]; then
    echo "metadata server connectivity expected to succeed but failed"
fi