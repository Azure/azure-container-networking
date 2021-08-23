package config

type NPMConfig struct {
	ResyncPeriodInMinutes int    `json:"ResyncPeriodInMinutes"`
	ListeningPort         int    `json:"ListeningPort"`
	ListeningAddress      string `json:"ListeningAddress"`
}
