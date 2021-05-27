package api

const (
	DefaultListeningIP = "0.0.0.0"
	DefaultHttpPort    = "10091"
	NodeMetricsPath    = "/node-metrics"
	ClusterMetricsPath = "/cluster-metrics"
	NPMMgrPath         = "/npm/v1/debug/manager"
)

type DescribeIPSetRequest struct {
	ipsetname string `json:"name"`
}

type DescribeIPSetResponse struct {
}
