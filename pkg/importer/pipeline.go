package importer

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type importStage string

const (
	stageClusterResources importStage = "cluster-resources"
	stageConfigMaps       importStage = "configmaps"
	stageSecrets          importStage = "secrets"
)

type importTask struct {
	Stage         importStage
	SourcePath    string
	GVR           schema.GroupVersionResource
	IncludeStatus bool
	Object        *unstructured.Unstructured
}

type importResult struct {
	Stage      importStage
	SourcePath string
	GVR        schema.GroupVersionResource
	Namespace  string
	Name       string
	Created    bool
	Err        error
}
