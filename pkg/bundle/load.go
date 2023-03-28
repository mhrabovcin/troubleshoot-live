package bundle

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/afero"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
)

// LoadCRDs gets CRDs stored in the bundle.
func LoadCRDs(b Bundle) ([]*apiextensionsv1.CustomResourceDefinition, error) {
	crdsPath := filepath.Join(b.Layout().ClusterResources(), "custom-resource-definitions.json")
	data, err := afero.ReadFile(b, crdsPath)
	if err != nil {
		return nil, err
	}

	decoder := scheme.Codecs.UniversalDeserializer().Decode
	crdList := &apiextensionsv1.CustomResourceDefinitionList{}
	_, _, err = decoder(data, nil, crdList)
	if err != nil {
		crdList, err = loadCRDsFromList(data)
		if err != nil {
			return nil, fmt.Errorf("failed to load CRDs: %w", err)
		}
	}

	bundleCrds := []*apiextensionsv1.CustomResourceDefinition{}
	for i := range crdList.Items {
		bundleCrds = append(bundleCrds, &crdList.Items[i])
	}
	return bundleCrds, nil
}

func loadCRDsFromList(data []byte) (*apiextensionsv1.CustomResourceDefinitionList, error) {
	crdList := &apiextensionsv1.CustomResourceDefinitionList{}

	objs := []map[string]any{}
	if err := json.Unmarshal(data, &objs); err != nil {
		return nil, fmt.Errorf("failed to detect CRDs: %w", err)
	}

	for _, obj := range objs {
		u := &unstructured.Unstructured{}
		u.Object = obj
		u.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "apiextensions",
			Version: "v1",
			Kind:    "CustomResourceDefinition",
		})
		crd := apiextensionsv1.CustomResourceDefinition{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &crd); err != nil {
			return nil, err
		}
		crdList.Items = append(crdList.Items, crd)
	}

	return crdList, nil
}
