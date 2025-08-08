package logger

import (
	"github.com/Azure/azure-container-networking/common"
	"go.uber.org/zap"
)

// MetadataToFields transforms Az IMDS Metadata in to zap.Field for
// attaching to a root zap core or logger instance.
// This uses the nice-names from the zapai.DefaultMappers instead of
// raw AppInsights key names.
func MetadataToFields(meta common.Metadata) []zap.Field {
	return []zap.Field{
		zap.String("account", meta.SubscriptionID),
		zap.String("anonymous_user_id", meta.VMName),
		zap.String("location", meta.Location),
		zap.String("resource_group", meta.ResourceGroupName),
		zap.String("vm_size", meta.VMSize),
		zap.String("os_version", meta.OSVersion),
		zap.String("vm_id", meta.VMID),
		zap.String("session_id", meta.VMID),
	}
}
