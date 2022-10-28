# Azure CNI Overlay Mode for AKS

Azure CNI Overlay mode is a new CNI network plugin that allocates pod IPs from an overlay network space, rather than from the virtual network IP space. This greatly reduces the IP utilization of Azure CNI as compared to the default mode. This CNI plugin functions like “kubenet” mode, but does not utilize route tables and thus is simpler to set up and much more scalable.

## Learn More
[Azure Documentation: Configure Azure CNI Overlay networking in Azure Kubernetes Service (AKS)](https://aka.ms/aks/azure-cni-overlay)