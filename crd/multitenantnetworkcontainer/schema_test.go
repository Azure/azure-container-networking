package multitenantnetworkcontainer_test

import (
	"encoding/json"
	"os"
	"testing"

	ncapi "github.com/Azure/azure-container-networking/crd/multitenantnetworkcontainer/api/v1alpha1"
	"github.com/stretchr/testify/require"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kube-openapi/pkg/validation/spec"
	"k8s.io/kube-openapi/pkg/validation/strfmt"
	"k8s.io/kube-openapi/pkg/validation/validate"
	"sigs.k8s.io/yaml"
)

func TestIPv6StatusSchema(t *testing.T) {
	data, err := os.ReadFile("manifests/networking.azure.com_multitenantnetworkcontainers.yaml")
	require.NoError(t, err)
	var crd apiextensionsv1.CustomResourceDefinition
	require.NoError(t, yaml.Unmarshal(data, &crd))
	require.Len(t, crd.Spec.Versions, 1)
	version := crd.Spec.Versions[0]
	require.Equal(t, "v1alpha1", version.Name)
	require.NotNil(t, version.Subresources.Status)
	current := version.Schema.OpenAPIV3Schema
	previous := current.DeepCopy()
	for _, property := range []string{"ipv6", "ipv6Prefix", "ipSubnetV6", "gatewayV6"} {
		status := current.Properties["status"]
		require.Equal(t, "string", status.Properties[property].Type)
		require.NotContains(t, status.Required, property)
		// Model the pre-upgrade schema: unknown status fields are pruned by the API server.
		delete(previous.Properties["status"].Properties, property)
	}
	ipv4 := ncapi.MultiTenantNetworkContainer{
		TypeMeta: metav1.TypeMeta{APIVersion: "networking.azure.com/v1alpha1", Kind: "MultiTenantNetworkContainer"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod", Namespace: "test",
		},
		Spec: ncapi.MultiTenantNetworkContainerSpec{UUID: "nc-id"},
		Status: ncapi.MultiTenantNetworkContainerStatus{
			State: "Initialized", IP: "10.0.0.4", IPSubnet: "10.0.0.0/24", Gateway: "10.0.0.1",
		},
	}
	ipv4Object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&ipv4)
	require.NoError(t, err)
	dualStack := ipv4.DeepCopy()
	dualStack.Status.IPv6 = "fd00:1234::4"
	dualStack.Status.IPv6Prefix = "fd00:1234::4/128"
	dualStack.Status.IPSubnetV6 = "fd00:1234::/64"
	dualStack.Status.GatewayV6 = "fe80::1"
	dualStackObject, err := runtime.DefaultUnstructuredConverter.ToUnstructured(dualStack)
	require.NoError(t, err)

	for _, tt := range []struct {
		name   string
		schema *apiextensionsv1.JSONSchemaProps
		object map[string]interface{}
		want   map[string]interface{}
	}{
		{"current/IPv4", current, ipv4Object, ipv4Object},
		{"current/dual stack", current, dualStackObject, dualStackObject},
		{"previous/IPv4", previous, ipv4Object, ipv4Object},
		{"previous/dual stack loses IPv6", previous, dualStackObject, ipv4Object},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var internal apiextensions.JSONSchemaProps
			require.NoError(t, apiextensionsv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(tt.schema, &internal, nil))
			structural, err := schema.NewStructural(&internal)
			require.NoError(t, err)
			data, err := json.Marshal(tt.schema)
			require.NoError(t, err)
			var openAPI spec.Schema
			require.NoError(t, json.Unmarshal(data, &openAPI))
			object := runtime.DeepCopyJSON(tt.object)
			pruning.Prune(object, structural, true)
			require.Empty(t, validate.NewSchemaValidator(&openAPI, nil, "", strfmt.Default).Validate(object).Errors)
			require.Equal(t, tt.want, object)
		})
	}
}
