package importer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func TestConversion(t *testing.T) {
	v1beta1Extension := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:                 "foo",
			Version:               "v1",
			PreserveUnknownFields: pointer.Bool(true),
		},
	}
	v1Extension, err := convertCRD(v1beta1Extension)
	assert.NoError(t, err)
	assert.Equal(t, v1Extension.Name, v1beta1Extension.Name)
}
