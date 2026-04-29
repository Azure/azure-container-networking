package multitenancy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilversion "k8s.io/apimachinery/pkg/util/version"
)

// crdWithSelectableFields returns a CRD with selectableFields populated on each version.
func crdWithSelectableFields() *v1.CustomResourceDefinition {
	return &v1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "test.example.com"},
		Spec: v1.CustomResourceDefinitionSpec{
			Versions: []v1.CustomResourceDefinitionVersion{
				{
					Name: "v1alpha1",
					SelectableFields: []v1.SelectableField{
						{JSONPath: ".spec.podName"},
						{JSONPath: ".status.status"},
					},
				},
			},
		},
	}
}

func TestEnsureSelectableFieldsVersionSafe_NilVersion(t *testing.T) {
	crd := crdWithSelectableFields()
	result := ensureSelectableFieldsVersionSafe(crd, nil)

	require.NotNil(t, result)
	for _, ver := range result.Spec.Versions {
		assert.Nil(t, ver.SelectableFields, "expected selectableFields to be stripped for nil k8s version")
	}
}

func TestEnsureSelectableFieldsVersionSafe_OldVersion(t *testing.T) {
	for _, v := range []string{"1.29.0", "1.30.5"} {
		k8sVersion := utilversion.MustParseGeneric(v)
		crd := crdWithSelectableFields()
		result := ensureSelectableFieldsVersionSafe(crd, k8sVersion)

		require.NotNil(t, result)
		for _, ver := range result.Spec.Versions {
			assert.Nil(t, ver.SelectableFields, "expected selectableFields to be stripped for k8s %s", v)
		}
	}
}

func TestEnsureSelectableFieldsVersionSafe_SupportedVersion(t *testing.T) {
	for _, v := range []string{"1.31.0", "1.32.0", "1.33.1"} {
		k8sVersion := utilversion.MustParseGeneric(v)
		crd := crdWithSelectableFields()
		result := ensureSelectableFieldsVersionSafe(crd, k8sVersion)

		require.NotNil(t, result)
		for _, ver := range result.Spec.Versions {
			assert.NotNil(t, ver.SelectableFields, "expected selectableFields to be preserved for k8s %s", v)
			assert.NotEmpty(t, ver.SelectableFields)
		}
	}
}

func TestEnsureSelectableFieldsVersionSafe_DoesNotMutateOriginal(t *testing.T) {
	original := crdWithSelectableFields()
	originalFields := original.Spec.Versions[0].SelectableFields

	_ = ensureSelectableFieldsVersionSafe(original, nil)

	assert.Equal(t, originalFields, original.Spec.Versions[0].SelectableFields, "original CRD must not be mutated")
}

func TestMakeCRDVersionSafe_NilVersion(t *testing.T) {
	installer := &Installer{k8sVersion: nil}
	crd := crdWithSelectableFields()
	result := installer.makeCRDVersionSafe(crd)

	for _, ver := range result.Spec.Versions {
		assert.Nil(t, ver.SelectableFields, "expected selectableFields stripped when k8sVersion is nil")
	}
}

func TestMakeCRDVersionSafe_SupportedVersion(t *testing.T) {
	installer := &Installer{k8sVersion: utilversion.MustParseGeneric("1.31.0")}
	crd := crdWithSelectableFields()
	result := installer.makeCRDVersionSafe(crd)

	for _, ver := range result.Spec.Versions {
		assert.NotNil(t, ver.SelectableFields, "expected selectableFields preserved for k8s 1.31+")
	}
}
