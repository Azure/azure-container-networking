# Azure CNS metrics
azure-cns exposes metrics via Prometheus on `:10092/metrics`

## Scraping 
Prometheus can be configured using these examples: 
- a [podMonitor](podMonitor.yaml), if using promotheus-operator or kube-prometheus
- manually via this equivalent [scrape_config](scrape_config.yaml)

## Monitoring
To view all available CNS metrics once Prometheus is correctly configured to scrape:
```promql
count ({job="kube-system/azure-cns"}) by (__name__)
```

CNS exposes standard Go and Prom metrics such as `go_goroutines`, `go_gc*`, `up`, and more.

Metrics designed to be customer-facing are generally prefixed with `cx_` and can be listed similarly:
```promql
count ({__name__=~"cx.*",job="kube-system/azure-cns"}) by (__name__)
```
At time of writing, the following cx metrics are exposed (key metrics in **bold**):
- **cx_ipam_available_ips** (IPs reserved by the Node but not assigned to Pods yet)
- cx_ipam_batch_size
- cx_ipam_current_available_ips
- cx_ipam_expect_available_ips
- **cx_ipam_max_ips** (maximum IPs the Node can reserve from the Subnet)
- cx_ipam_pending_programming_ips
- cx_ipam_pending_release_ips
- **cx_ipam_pod_allocated_ips** (IPs assigned to Pods on the Node)
- cx_ipam_requested_ips
- **cx_ipam_total_ips** (IPs reserved by the Node from the Subnet)

These metrics may be used to gain insight in to the current state of the cluster's IPAM. 

For example, to view the current IP count requested by each node:
```promql
sum (cx_ipam_requested_ips{job="kube-system/azure-cns"}) by (instance)
```
To view the current IP count allocated to each node:
```promql
sum (cx_ipam_total_ips{job="kube-system/azure-cns"}) by (instance)
```
> Note: if these two values aren't converging after some time, that indicates an IP provisioning error.

To view the current IP count assigned to pods, per node:
```promql
sum (cx_ipam_pod_allocated_ips{job="kube-system/azure-cns"}) by (instance)
```

## Visualizing
A sample Grafana dashboard is included at [grafan.json](grafana.json).

Visualizations included are: 
- Per Node
    - CNS Status (Up/Down)
    - Requested IPs
    - Reserved IPs
    - Used IPs
    - Request/Reserved/Used vs Time
- Per Cluster
    - Total Reserver IPs vs Time
    - Total Used IPs vs Time
    - Reserved and Assigned vs Time
    - Cluster Subnet Utilization Percentage vs Time
    - Cluster Subnet Utilization Total vs Time
    - Node Headroom (how many additional Nodes can be added to the Cluster based on the Subnet capacity)
