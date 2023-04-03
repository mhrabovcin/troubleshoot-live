package importer

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/mesosphere/dkp-cli-runtime/core/output"
	"github.com/spf13/afero"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/strings/slices"

	"github.com/mhrabovcin/troubleshoot-live/pkg/bundle"
	"github.com/mhrabovcin/troubleshoot-live/pkg/cli"
	"github.com/mhrabovcin/troubleshoot-live/pkg/utils"
)

// AnnotationForOriginalValue creates annotation key for given value.
func AnnotationForOriginalValue(name string) string {
	return fmt.Sprintf("support-bundle-live/%s", name)
}

// ImportBundle creates resources in provided API server.
func ImportBundle(ctx context.Context, b bundle.Bundle, restCfg *rest.Config, out output.Output) error {
	dynamicClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		return err
	}

	cfg := &importerConfig{
		dynamicClient:   dynamicClient,
		discoveryClient: discoveryClient,
		bundle:          b,
		out:             out,
	}

	importers := []importerFn{
		importCRDs,
		importNamespaces,
		importClusterResources,
		importCMs,
		importSecrets,
	}

	for _, importerFn := range importers {
		if err := importerFn(ctx, cfg); err != nil {
			return err
		}
	}

	return nil
}

type importerConfig struct {
	dynamicClient   dynamic.Interface
	discoveryClient discovery.DiscoveryInterface
	bundle          bundle.Bundle
	out             output.Output
}

type importerFn func(context.Context, *importerConfig) error

func importNamespaces(
	ctx context.Context,
	cfg *importerConfig,
) error {
	namespacesPath := filepath.Join(cfg.bundle.Layout().ClusterResources(), "namespaces.json")
	list, err := bundle.LoadResourcesFromFile(cfg.bundle, namespacesPath)
	if err != nil {
		cli.WarnOnErrorsFilePresence(cfg.bundle, cfg.out, namespacesPath)
		return err
	}

	populateGVK(list, schema.GroupVersionKind{
		Version: "v1",
		Kind:    "Namespace",
	})

	namespaces := []string{}
	gvr, includeStatus, err := detectGVR(cfg.discoveryClient, &list.Items[0])
	if err != nil {
		return err
	}
	return list.EachListItem(func(o runtime.Object) error {
		u, _ := meta.Accessor(o)
		namespaces = append(namespaces, u.GetName())
		return importObject(ctx, cfg.dynamicClient, gvr, o, includeStatus)
	})
}

func importClusterResources(
	ctx context.Context,
	cfg *importerConfig,
) error {
	skipResources := []string{
		// crds are imported during a separate step
		"custom-resource-definitions.json",
		"pod-disruption-budgets-info.json",
		// api-resources from the discovery client
		"resources.json",
		// api-groups from the discovery client
		"groups.json",
		// namespaces are imported as first resource in a separate step
		"namespaces.json",
	}

	skipDirs := []string{
		"auth-cani-list",
		"pod-disruption-budgets",
	}

	return afero.Walk(cfg.bundle, cfg.bundle.Layout().ClusterResources(), func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			cfg.out.Warnf("Failed to read file %q from bundle: %s", path, err)
			return nil
		}

		// Do not process any resources from the directory
		if info.IsDir() && slices.Contains(skipDirs, filepath.Base(info.Name())) {
			return fs.SkipDir
		}

		if info.IsDir() {
			return nil
		}

		if slices.Contains(skipResources, filepath.Base(info.Name())) {
			return nil
		}

		// skip failed resources
		if strings.HasSuffix(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), "-errors") {
			return nil
		}

		list, err := bundle.LoadResourcesFromFile(cfg.bundle, path)
		if err != nil {
			cli.WarnOnErrorsFilePresence(cfg.bundle, cfg.out, path)
			cfg.out.Errorf(utils.MaxErrorString(err, 200), "Failed to load resources from file %q", path)
			return nil
		}

		if len(list.Items) == 0 {
			return nil
		}

		// Kind was not stored in older troubleshoot versions for non-CRDs, try to
		// figure out the kind by the filename.
		if list.Items[0].GetKind() == "" {
			relPath, err := filepath.Rel(cfg.bundle.Layout().ClusterResources(), path)
			if err != nil {
				return fmt.Errorf("failed to detect kind for path %q: %w", path, err)
			}
			if gvk, err := gvkFromFile(relPath); err == nil {
				populateGVK(list, gvk)
			}
		}

		cfg.out.V(1).Infof("Importing objects from: %s ...", path)

		gvr, includeStatus, err := detectGVR(cfg.discoveryClient, &list.Items[0])
		if err != nil {
			cfg.out.Errorf(err, "failed to detect GVR from file %q. CRD for the resource may not be imported:", path)
			return nil
		}

		_ = list.EachListItem(func(o runtime.Object) error {
			err := importObject(ctx, cfg.dynamicClient, gvr, o, includeStatus)
			if err != nil {
				u, _ := meta.Accessor(o)
				cfg.out.Warnf(
					"Failed to import %q (%s) with error: %s",
					fmt.Sprintf("%s/%s", u.GetNamespace(), u.GetName()), gvr, err,
				)
			}
			return nil
		})

		return nil
	})
}

type cmOrSecretLoadFn func(afero.Fs, string) (*unstructured.Unstructured, error)

func importCMOrSecrets(
	ctx context.Context,
	cfg *importerConfig,
	path string,
	loadFn cmOrSecretLoadFn,
	gvr schema.GroupVersionResource,
) error {
	return afero.Walk(cfg.bundle, path, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			cfg.out.Errorf(err, "Failed to read file %q from bundle", path)
			return nil
		}

		if info.IsDir() {
			return nil
		}

		cfg.out.V(1).Infof("Importing %s from: %s ... ", gvr.Resource, path)

		obj, err := loadFn(cfg.bundle, path)
		if err != nil {
			cfg.out.Errorf(utils.MaxErrorString(err, 200), "Failed to import secret from %q", path)
			return nil
		}

		if err := importObject(ctx, cfg.dynamicClient, gvr, obj, true); err != nil {
			return err
		}

		return nil
	})
}

func importCMs(
	ctx context.Context,
	cfg *importerConfig,
) error {
	gvr := schema.GroupVersionResource{
		Version:  "v1",
		Resource: "configmaps",
	}
	return importCMOrSecrets(
		ctx, cfg, cfg.bundle.Layout().ConfigMaps(), bundle.LoadConfigMap, gvr)
}

func importSecrets(
	ctx context.Context,
	cfg *importerConfig,
) error {
	gvr := schema.GroupVersionResource{
		Version:  "v1",
		Resource: "secrets",
	}
	return importCMOrSecrets(
		ctx, cfg, cfg.bundle.Layout().Secrets(), bundle.LoadSecret, gvr)
}

func importObject(
	ctx context.Context,
	cl dynamic.Interface,
	gvr schema.GroupVersionResource,
	o runtime.Object,
	includeStatus bool,
) error {
	if err := prepareForImport(o); err != nil {
		return err
	}

	u := o.(*unstructured.Unstructured)

	if u.GetKind() == "Job" {
		// The .spec.selector is validated by core kube-apiserver and cannot be
		// added without specifying the `manualSelector`.
		_ = unstructured.SetNestedField(u.Object, true, "spec", "manualSelector")
		annotations := u.GetAnnotations()
		annotations[AnnotationForOriginalValue("added-spec.manualSelector")] = "true"
		u.SetAnnotations(annotations)
	}

	_, err := cl.Resource(gvr).Namespace(u.GetNamespace()).Get(ctx, u.GetName(), metav1.GetOptions{})
	if err != nil && apierrors.IsNotFound(err) {
		nsClient := cl.Resource(gvr).Namespace(u.GetNamespace())
		err := createResource(ctx, u, includeStatus, nsClient)
		if err != nil {
			return fmt.Errorf("failed to import resource: %w", err)
		}
	}

	return nil
}

func createResource(ctx context.Context, u *unstructured.Unstructured, includeStatus bool, nsClient dynamic.ResourceInterface) error {
	_, err := nsClient.Create(ctx, u, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	// Only import status for objects with status field
	if _, ok := u.Object["status"]; !ok || !includeStatus {
		return nil
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		updated, err := nsClient.Get(ctx, u.GetName(), metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to load created object: %w", err)
		}

		if err := unstructured.SetNestedField(updated.Object, u.Object["status"], "status"); err != nil {
			return fmt.Errorf("failed to set status field: %w", err)
		}

		_, err = nsClient.UpdateStatus(ctx, updated, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update status: %w", err)
		}
		return nil
	})
}
