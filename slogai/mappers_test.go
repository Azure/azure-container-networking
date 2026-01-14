package slogai

import (
	"testing"
)

func TestApplicationContextMappers(t *testing.T) {
	if ApplicationContextMappers["version"] != "ai.application.ver" {
		t.Error("expected version -> ai.application.ver")
	}
}

func TestDeviceContextMappers(t *testing.T) {
	expected := map[string]string{
		"device_id":   "ai.device.id",
		"locale":      "ai.device.locale",
		"model":       "ai.device.model",
		"oem":         "ai.device.oemName",
		"os_version":  "ai.device.osVersion",
		"device_type": "ai.device.type",
	}

	for field, tag := range expected {
		if DeviceContextMappers[field] != tag {
			t.Errorf("expected %s -> %s, got %s", field, tag, DeviceContextMappers[field])
		}
	}
}

func TestLocationContextMappers(t *testing.T) {
	if LocationContextMappers["ip"] != "ai.location.ip" {
		t.Error("expected ip -> ai.location.ip")
	}
}

func TestOperationContextMappers(t *testing.T) {
	expected := map[string]string{
		"operation_id":     "ai.operation.id",
		"operation_name":   "ai.operation.name",
		"parent_id":        "ai.operation.parentId",
		"synthetic_source": "ai.operation.syntheticSource",
		"correlation_id":   "ai.operation.correlationVector",
	}

	for field, tag := range expected {
		if OperationContextMappers[field] != tag {
			t.Errorf("expected %s -> %s, got %s", field, tag, OperationContextMappers[field])
		}
	}
}

func TestSessionContextMappers(t *testing.T) {
	expected := map[string]string{
		"session_id":       "ai.session.id",
		"session_is_first": "ai.session.isFirst",
	}

	for field, tag := range expected {
		if SessionContextMappers[field] != tag {
			t.Errorf("expected %s -> %s, got %s", field, tag, SessionContextMappers[field])
		}
	}
}

func TestUserContextMappers(t *testing.T) {
	expected := map[string]string{
		"account":           "ai.user.accountId",
		"anonymous_user_id": "ai.user.id",
		"user_id":           "ai.user.authUserId",
	}

	for field, tag := range expected {
		if UserContextMappers[field] != tag {
			t.Errorf("expected %s -> %s, got %s", field, tag, UserContextMappers[field])
		}
	}
}

func TestCloudContextMappers(t *testing.T) {
	expected := map[string]string{
		"role":          "ai.cloud.role",
		"role_instance": "ai.cloud.roleInstance",
	}

	for field, tag := range expected {
		if CloudContextMappers[field] != tag {
			t.Errorf("expected %s -> %s, got %s", field, tag, CloudContextMappers[field])
		}
	}
}

func TestInternalContextMappers(t *testing.T) {
	expected := map[string]string{
		"sdk_version":   "ai.internal.sdkVersion",
		"agent_version": "ai.internal.agentVersion",
		"node_name":     "ai.internal.nodeName",
	}

	for field, tag := range expected {
		if InternalContextMappers[field] != tag {
			t.Errorf("expected %s -> %s, got %s", field, tag, InternalContextMappers[field])
		}
	}
}

func TestDefaultMappers_ContainsAll(t *testing.T) {
	// Verify DefaultMappers contains all individual mappers
	allMappers := []map[string]string{
		ApplicationContextMappers,
		DeviceContextMappers,
		LocationContextMappers,
		OperationContextMappers,
		SessionContextMappers,
		UserContextMappers,
		CloudContextMappers,
		InternalContextMappers,
	}

	for _, mapper := range allMappers {
		for field, tag := range mapper {
			if DefaultMappers[field] != tag {
				t.Errorf("DefaultMappers missing %s -> %s", field, tag)
			}
		}
	}
}

func TestDefaultMappers_Count(t *testing.T) {
	// Count total expected entries
	expectedCount := len(ApplicationContextMappers) +
		len(DeviceContextMappers) +
		len(LocationContextMappers) +
		len(OperationContextMappers) +
		len(SessionContextMappers) +
		len(UserContextMappers) +
		len(CloudContextMappers) +
		len(InternalContextMappers)

	if len(DefaultMappers) != expectedCount {
		t.Errorf("DefaultMappers has %d entries, expected %d", len(DefaultMappers), expectedCount)
	}
}

func TestDefaultMappers_NoOverlap(t *testing.T) {
	// Verify there are no duplicate keys across mappers
	seen := make(map[string]bool)
	allMappers := []map[string]string{
		ApplicationContextMappers,
		DeviceContextMappers,
		LocationContextMappers,
		OperationContextMappers,
		SessionContextMappers,
		UserContextMappers,
		CloudContextMappers,
		InternalContextMappers,
	}

	for _, mapper := range allMappers {
		for field := range mapper {
			if seen[field] {
				t.Errorf("duplicate field key found: %s", field)
			}
			seen[field] = true
		}
	}
}

func TestMappers_AITagFormat(t *testing.T) {
	// All AI tags should start with "ai."
	for field, tag := range DefaultMappers {
		if len(tag) < 3 || tag[:3] != "ai." {
			t.Errorf("tag for %s should start with 'ai.', got %s", field, tag)
		}
	}
}
