package imds

import (
	"context"

	"github.com/avast/retry-go/v4"
	"github.com/pkg/errors"
)

type ComputeMetadata struct {
	ResourceID string
	VMID       string
}

func (c *Client) GetComputeMetadata(ctx context.Context) (ComputeMetadata, error) {
	var result ComputeMetadata
	err := retry.Do(func() error {
		doc, err := c.getInstanceMetadata(ctx, imdsComputePath, imdsDefaultAPIVersion)
		if err != nil {
			return errors.Wrap(err, "reading compute metadata")
		}
		result.ResourceID, _ = doc["resourceId"].(string)
		result.VMID, _ = doc[vmUniqueIDProperty].(string)
		if result.ResourceID == "" || result.VMID == "" {
			return errors.New("IMDS compute metadata has no complete VM identity")
		}
		return nil
	}, retry.Context(ctx), retry.Attempts(c.config.retryAttempts), retry.DelayType(retry.BackOffDelay))
	if err != nil {
		return ComputeMetadata{}, errors.Wrap(err, "getting compute identity")
	}
	return result, nil
}
