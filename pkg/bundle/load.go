package bundle

import (
	"path/filepath"

	"github.com/spf13/afero"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

// LoadCRDs gets CRDs stored in the bundle.
func LoadCRDs(b Bundle) ([]*apiextensionsv1.CustomResourceDefinition, error) {
	data, err := afero.ReadFile(b, filepath.Join(b.Layout().ClusterResources(), "custom-resource-definitions.json"))
	if err != nil {
		return nil, err
	}

	decoder := scheme.Codecs.UniversalDeserializer().Decode
	crdList := &apiextensionsv1.CustomResourceDefinitionList{}
	_, _, err = decoder(data, nil, crdList)
	if err != nil {
		return nil, err
	}

	bundleCrds := []*apiextensionsv1.CustomResourceDefinition{}
	for i := range crdList.Items {
		bundleCrds = append(bundleCrds, &crdList.Items[i])
	}
	return bundleCrds, nil
}
