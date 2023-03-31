package importer

import (
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
)

func convertCRD(
	v1beta1Extension *apiextensionsv1beta1.CustomResourceDefinition,
) (*apiextensionsv1.CustomResourceDefinition, error) {
	extension := &apiextensions.CustomResourceDefinition{}
	scheme := runtime.NewScheme()
	_ = apiextensions.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)
	_ = apiextensionsv1beta1.AddToScheme(scheme)

	err := scheme.Converter().Convert(v1beta1Extension, extension, &conversion.Meta{})
	if err != nil {
		return nil, err
	}

	v1Extension := &apiextensionsv1.CustomResourceDefinition{}
	err = scheme.Converter().Convert(extension, v1Extension, &conversion.Meta{})
	if err != nil {
		return nil, err
	}

	return v1Extension, nil
}
