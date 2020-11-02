// +build integration

package goldpinger

import "time"

type CheckAllJSON struct {
	DNSResults map[string]interface{}   `json:"dnsResults"`
	Hosts      []HostJSON               `json:"hosts"`
	Responses  map[string]PodStatusJSON `json:"responses"`
}

type HostJSON struct {
	HostIP  string `json:"hostIP"`
	PodIP   string `json:"podIP"`
	PodName string `json:"podName"`
}

type PodStatusJSON struct {
	HostIP   string       `json:"HostIP"`
	OK       bool         `json:"OK"`
	PodIP    string       `json:"PodIP"`
	Response ResponseJSON `json:"response"`
}

type ResponseJSON struct {
	DNSResults map[string]interface{}    `json:"dnsResults"`
	PodResults map[string]PingResultJSON `json:"podResults"`
}

type PingResultJSON struct {
	HostIP                   string    `json:"HostIP"`
	OK                       bool      `json:"OK"`
	PodIP                    string    `json:"PodIP"`
	PingTime                 time.Time `json:"PingTime"`
	ResponseTimeMilliseconds int       `json:"response-time-ms"`
	StatusCode               int       `json:"status-code"`
}
