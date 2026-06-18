package importer

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/mhrabovcin/troubleshoot-live/pkg/bundle"
	"github.com/mhrabovcin/troubleshoot-live/pkg/cli"
)

var crdGVR = schema.GroupVersionResource{
	Group:    "apiextensions.k8s.io",
	Version:  "v1",
	Resource: "customresourcedefinitions",
}

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

	tasks := make([]importTask, 0, len(list.Items))
	var prepareErrors []error
	err = list.EachListItem(func(in runtime.Object) error {
		u, err := asUnstructured(in)
		if err != nil {
			prepareErrors = append(prepareErrors, err)
			return nil
		}

		gvr, includeStatus, err := cfg.gvrResolver.Detect(u)
		cfg.out.V(5).Infof("CRD import: detected %s %q", u.GetName(), gvr)

		// Assume that k8s api server doesn't know about the old apiextensions v1beta1
		// version. Attempt to convert it to v1.
		if err != nil {
			cfg.out.V(1).Infof("Attempting to convert CRD %s from v1beta1 to v1", u.GetName())
			v1beta1Extension := &apiextensionsv1beta1.CustomResourceDefinition{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, v1beta1Extension); err != nil {
				prepareErrors = append(
					prepareErrors,
					fmt.Errorf("failed to convert CRD unstructured to v1beta1 extension %q: %w", u.GetName(), err),
				)
				return nil
			}
			v1Extension, err := convertCRD(v1beta1Extension)
			if err != nil {
				prepareErrors = append(
					prepareErrors,
					fmt.Errorf("failed to convert CRD from v1beta1 to v1 for %q: %w", u.GetName(), err),
				)
				return nil
			}

			if crdHasNonStructuralSchema(v1Extension) {
				cfg.out.Warnf("CRD in bundle %s has status NonStructuralSchema set to true", u.GetName())
				v1Extension.Spec.PreserveUnknownFields = false
			}

			gvr = crdGVR
			includeStatus = true
			uMap, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(v1Extension)
			u = &unstructured.Unstructured{Object: uMap}
		}

		tasks = append(tasks, importTask{
			sourcePath:    filepath.Join(cfg.bundle.Layout().ClusterResources(), "custom-resource-definitions.json"),
			gvr:           gvr,
			object:        u.DeepCopy(),
			includeStatus: includeStatus,
		})
		return nil
	})
	if err != nil {
		return err
	}

	importedCRDs := []string{}
	var crdsMu sync.Mutex

	// We use a custom worker pool to only track successfully imported CRDs
	wp := newWorkerPool(ctx, defaultImportWorkers, func(innerCtx context.Context, task importTask) error {
		err := importObjectWithRetry(innerCtx, cfg.dynamicClient, task.gvr, task.object, task.includeStatus, cfg.objectPreparer)
		if err != nil {
			cfg.out.Warnf(
				"Failed to import %q (%s) from %q with error: %s",
				objectReference(task.object), task.gvr, task.sourcePath, err,
			)
			return err
		}

		// Only track successfully imported CRDs to avoid 60s timeout in waitForCRDsEstablished
		crdsMu.Lock()
		importedCRDs = append(importedCRDs, task.object.GetName())
		crdsMu.Unlock()
		return nil
	})

	var allErrors []error
	allErrors = append(allErrors, prepareErrors...)

	for _, task := range tasks {
		addErr := wp.Add(ctx, task)
		if addErr != nil {
			allErrors = append(allErrors, addErr)
			break // Context cancelled
		}
	}

	waitPoolErr := wp.Wait()
	allErrors = append(allErrors, errorOrNil(waitPoolErr)...)

	if len(importedCRDs) > 0 {
		waitStart := time.Now()
		waitErr := waitForCRDsEstablished(ctx, cfg, importedCRDs)
		cfg.out.V(1).Infof("Waited %s for %d CRDs to be established", time.Since(waitStart).Round(time.Millisecond), len(importedCRDs))
		allErrors = append(allErrors, errorOrNil(waitErr)...)
	}

	return errors.Join(allErrors...)
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

func waitForCRDsEstablished(ctx context.Context, cfg *importerConfig, crdNames []string) error {
	if len(crdNames) == 0 {
		return nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, cfg.crdWaitTimeout)
	defer cancel()

	pending := map[string]struct{}{}
	for _, name := range crdNames {
		if name != "" {
			pending[name] = struct{}{}
		}
	}

	if len(pending) == 0 {
		return nil
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		for name := range pending {
			established, err := isCRDEstablished(waitCtx, cfg, name)
			if err != nil {
				if apierrors.IsNotFound(err) || isRetryableImportErr(err) {
					continue
				}
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
					break
				}
				return fmt.Errorf("failed checking CRD %q established condition: %w", name, err)
			}
			if established {
				delete(pending, name)
			}
		}

		if len(pending) == 0 {
			return nil
		}

		select {
		case <-waitCtx.Done():
			notReadyCRDs := make([]string, 0, len(pending))
			for name := range pending {
				notReadyCRDs = append(notReadyCRDs, name)
			}
			sort.Strings(notReadyCRDs)
			return fmt.Errorf(
				"timed out waiting for CRDs to be established after %s: %s",
				cfg.crdWaitTimeout,
				strings.Join(notReadyCRDs, ", "),
			)
		case <-ticker.C:
		}
	}
}

func isCRDEstablished(ctx context.Context, cfg *importerConfig, crdName string) (bool, error) {
	crd, err := cfg.dynamicClient.Resource(crdGVR).Get(ctx, crdName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	conditions, found, err := unstructured.NestedSlice(crd.Object, "status", "conditions")
	if err != nil || !found {
		return false, err
	}

	for _, conditionRaw := range conditions {
		condition, ok := conditionRaw.(map[string]any)
		if !ok {
			continue
		}

		conditionType, _ := condition["type"].(string)
		conditionStatus, _ := condition["status"].(string)
		if conditionType == "Established" && conditionStatus == "True" {
			return true, nil
		}
	}

	return false, nil
}
