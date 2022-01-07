//nolint:revive // TODO(rbtr) docstrings
package zapai

import "github.com/microsoft/ApplicationInsights-Go/appinsights"

// fieldTagMapper is a translation function from a zap field name to an appinsights tag name.
// fieldTagMappers can be used to transform nice (aka conventional) names like "version" from a zap.Field name
// in to the magic appinsights field key "ai.application.ver".
type fieldTagMapper func(trace *appinsights.TraceTelemetry, fieldValue string)

// ApplicationContextMappers are the default ApplicationContext tag mappers.
var ApplicationContextMappers = map[string]string{
	"version": "ai.application.ver",
}

// DeviceContextMappers are the default DeviceContext tag mappers.
var DeviceContextMappers = map[string]string{
	"device_id":   "ai.device.id",
	"locale":      "ai.device.locale",
	"model":       "ai.device.model",
	"oem":         "ai.device.oemName",
	"os_version":  "ai.device.osVersion",
	"device_type": "ai.device.type",
}

// LocationContextMappers are the default LocationContext tag mappers.
var LocationContextMappers = map[string]string{
	"ip": "ai.location.ip",
}

// OperationContextMappers are the default OperationContext tag mappers.
var OperationContextMappers = map[string]string{
	"operation_id":     "ai.operation.id",
	"operation_name":   "ai.operation.name",
	"parent_id":        "ai.operation.parentId",
	"synthetic_source": "ai.operation.syntheticSource",
	"correlation_id":   "ai.operation.correlationVector",
}

// SessionContextMappers are the default SessionContext tag mappers.
var SessionContextMappers = map[string]string{
	"session_id":       "ai.session.id",
	"session_is_first": "ai.session.isFirst",
}

// UserContextMappers are the default UserContext tag mappers.
var UserContextMappers = map[string]string{
	"account":           "ai.user.accountId",
	"anonymous_user_id": "ai.user.id",
	"user_id":           "ai.user.authUserId",
}

// CloudContextMappers are the default CloudContext tag mappers.
var CloudContextMappers = map[string]string{
	"az_role":          "ai.cloud.role",
	"az_role_instance": "ai.cloud.roleInstance",
}

// InternalContextMappers are the default InternalContext tag mappers.
var InternalContextMappers = map[string]string{
	"ai_sdk_version":   "ai.internal.sdkVersion",
	"ai_agent_version": "ai.internal.agentVersion",
	"node_name":        "ai.internal.nodeName",
}

// allMappers is a convenience var which is a slice of all hardcoded mappers.
var allMappers = []map[string]string{
	ApplicationContextMappers,
	DeviceContextMappers,
	LocationContextMappers,
	OperationContextMappers,
	SessionContextMappers,
	UserContextMappers,
	CloudContextMappers,
	InternalContextMappers,
}

// DefaultMappers is a convenience var which is a combined tagMapper of all hardcoded mappers.
var DefaultMappers = func() map[string]string {
	m := map[string]string{}
	for _, mm := range allMappers {
		for k, v := range mm {
			m[k] = v
		}
	}
	return m
}()
