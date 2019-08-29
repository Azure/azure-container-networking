# Microsoft Azure Container Networking
CNI Multitenacy binaries are meant only for 1st party customers for now.

Conflist Fields Description
---------------------------
multiTenancy - To indicate CNI to use multitenancy network setup using ovs bridge. Thefollowing fields will be processed
                only if this fields is set to true

enableExactMatchForPodName - If this set to false, then CNI strips the last two hex fields added by container runtime to locate the pod.
                             For Eg: In kubernetes, if pod name is samplepod, then container runtime generates this as samplepod-3e4a-5e4a.
                             CNI would strip 3e4a-5e4a and keep it as samplepod to locate the pod in CNS.
                             If the field is set to true, CNI would take whatever container runtime provides.

enableSnatOnHost - If pod/container wants outbound connectivity, this field should be set to true. Enabling this field also enables
                   ip forwarding kernel setting in container host and adds iptable rule to allow forward traffic from snat bridge.

