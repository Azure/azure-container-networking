package api

const (
	DefaultListeningIP = "0.0.0.0"
	DefaultHttpPort    = "10091"
	NodeMetricsPath    = "/node-metrics"
	ClusterMetricsPath = "/cluster-metrics"
	NPMMgrPath         = "/npm/v1/debug/manager"
	NPMV2CacheAPI      = "/npm/v2/debug/cache"
)

type DescribeIPSetRequest struct{}

type DescribeIPSetResponse struct{}
