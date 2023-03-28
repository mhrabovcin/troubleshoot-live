package bundle

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/afero"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// LoadResourcesFromFile tries to k8s API resources from a given file. It supports
// resources stored as List kind or YAML array of separate resources.
func LoadResourcesFromFile(bundle afero.Fs, path string) (*unstructured.UnstructuredList, error) {
	list := &unstructured.UnstructuredList{}

	data, err := afero.ReadFile(bundle, path)
	if err != nil {
		return nil, err
	}

	if strings.HasSuffix(path, ".json") {
		err := json.Unmarshal(data, list)
		return list, err
	}

	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		items := []unstructured.Unstructured{}
		if err := yaml.Unmarshal(data, &items); err != nil {
			return nil, err
		}
		list.Items = items
		return list, nil
	}

	return nil, fmt.Errorf("unsupported data format")
}
