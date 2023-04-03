package importer

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/mhrabovcin/troubleshoot-live/pkg/bundle"
	"github.com/mhrabovcin/troubleshoot-live/pkg/cli"
)

func loadCRDs(b bundle.Bundle) (*unstructured.UnstructuredList, error) {
	crdsPath := filepath.Join(b.Layout().ClusterResources(), "custom-resource-definitions.json")
	list, err := bundle.LoadResourcesFromFile(b, crdsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load CRDs: %w", err)
	}

	for i := range list.Items {
		item := &list.Items[i]

		// Detect api group and version if not stored in the bundle file
		if item.GetKind() == "" {
			gv := schema.GroupVersion{
				Group:   "apiextensions.k8s.io",
				Version: "v1",
			}

			// Assume old version of CRD if this value is present
			if found, ok, _ := unstructured.NestedBool(item.Object, "spec", "preserveUnknownFields"); ok && found {
				log.Printf("CRD %s assumed to version v1beta1 based on preserveUnknownFields presence", item.GetName())
				gv.Version = "v1beta1"
			}

			item.SetAPIVersion(gv.Identifier())
			item.SetKind("CustomResourceDefinition")
		}

		// Old versions of `troubleshoot` weren't always collecting the latest
		// version of the resources, e.g. collected `v1beta1` instead of `v1`.
		// If the CRD contains conversion config the envtest API server
		// will try to execute the webhook and fail to import all the resources.
		// In order to try importing all the resources remove the conversion webhook
		// and let the validation fail for incorrect resources.
		if err := unstructured.SetNestedField(item.Object, nil, "spec", "conversion", "webhook"); err != nil {
			return nil, err
		}
		if err := unstructured.SetNestedField(
			item.Object, string(apiextensions.NoneConverter), "spec", "conversion", "strategy"); err != nil {
			return nil, err
		}
	}

	return list, nil
}

func importCRDs(
	ctx context.Context,
	cfg *importerConfig,
) error {
	list, err := loadCRDs(cfg.bundle)
	if err != nil {
		cli.WarnOnErrorsFilePresence(
			cfg.bundle, cfg.out,
			filepath.Join(cfg.bundle.Layout().ClusterResources(), "custom-resource-definitions.json"),
		)
		return err
	}

	cfg.out.Infof("Processing %d records from CRD file", len(list.Items))

	return list.EachListItem(func(in runtime.Object) error {
		o, _ := meta.Accessor(in)
		u, _ := in.(*unstructured.Unstructured)
		gvr, includeStatus, err := detectGVR(cfg.discoveryClient, u)
		cfg.out.V(5).Infof("CRD import: detected %s %q", o.GetName(), gvr)

		// Assume that k8s api server doesn't know about the old apiextensions v1beta1
		// version. Attempt to convert it to v1.
		if err != nil {
			cfg.out.V(1).Infof("Attempting to convert CRD %s from v1beta1 to v1", u.GetName())
			v1beta1Extension := &apiextensionsv1beta1.CustomResourceDefinition{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, v1beta1Extension); err != nil {
				return fmt.Errorf("failed to convert CRD unstructured to v1beta1 extension %q: %w", o.GetName(), err)
			}
			v1Extension, err := convertCRD(v1beta1Extension)
			if err != nil {
				return fmt.Errorf("failed to convert CRD from v1beta1 to v1: %w", err)
			}

			if crdHasNonStructuralSchema(v1Extension) {
				cfg.out.Warnf("CRD in bundle %s has status NonStructuralSchema set to true", u.GetName())
				v1Extension.Spec.PreserveUnknownFields = false
			}

			gvr = schema.GroupVersionResource{
				Group:    "apiextensions.k8s.io",
				Version:  "v1",
				Resource: "customresourcedefinitions",
			}
			includeStatus = true
			uMap, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(v1Extension)
			in = &unstructured.Unstructured{Object: uMap}
		}

		err = importObject(ctx, cfg.dynamicClient, gvr, in, includeStatus)
		if err != nil {
			cfg.out.Warnf(
				"Failed to import CRD %q (%s) with error: %s", o.GetName(), gvr, err,
			)
		}
		return nil
	})
}

// IsStatusConditionPresentAndEqual returns true when conditionType is present and equal to status.
func crdHasNonStructuralSchema(crd *apiextensionsv1.CustomResourceDefinition) bool {
	for _, condition := range crd.Status.Conditions {
		if condition.Type == apiextensionsv1.NonStructuralSchema {
			return condition.Status == apiextensionsv1.ConditionTrue
		}
	}
	return false
}
