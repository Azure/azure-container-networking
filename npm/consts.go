package npm

const (
	heartbeatIntervalInMinutes = 30 //nolint:unused,deadcode,varcheck // ignore this error
	// TODO: consider increasing thread number later when logics are correct
	// threadness = 1

	NodeName CacheKey = "NodeName"

	NsMap   CacheKey = "NsMap"
	PodMap  CacheKey = "PodMap"
	ListMap CacheKey = "ListMap"
	SetMap  CacheKey = "SetMap"

	EnvNodeName = "HOSTNAME"
)
