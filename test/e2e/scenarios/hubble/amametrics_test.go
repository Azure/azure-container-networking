package hubble

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/stretchr/testify/require"
)

const (
	promHubbleJob = "hubble-pods"
)

func TestPrometheusQuery(t *testing.T) {
	client, err := api.NewClient(api.Config{
		Address: "http://localhost:9090",
	})

	require.NoError(t, err)
	promapi := promv1.NewAPI(client)
	ctx := context.Background()
	targets, err := promapi.Targets(ctx)
	require.NoError(t, err)
	require.NotNil(t, targets)

	hubbleActiveTarget := &promv1.ActiveTarget{
		ScrapePool: promHubbleJob,
		Health:     "up",
	}

	for _, target := range targets.Active {
		if target.ScrapePool == hubbleActiveTarget.ScrapePool {
			require.Equal(t, hubbleActiveTarget.Health, hubbleActiveTarget.Health)
			break
		}
	}
}
