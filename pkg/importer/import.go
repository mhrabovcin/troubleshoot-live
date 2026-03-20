package importer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/mesosphere/dkp-cli-runtime/core/output"
	"github.com/spf13/afero"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

// ImportBundle creates resources in provided API server.
func ImportBundle(ctx context.Context, b bundle.Bundle, restCfg *rest.Config, out output.Output) error {
	return ImportBundleWithOptions(ctx, b, restCfg, out, DefaultImportOptions())
}

// ImportBundleWithOptions creates resources in provided API server.
func ImportBundleWithOptions(
	ctx context.Context,
	b bundle.Bundle,
	restCfg *rest.Config,
	out output.Output,
	opts ImportOptions,
) error {
	opts = normalizeImportOptions(opts)
	importStartedAt := time.Now()

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
		options:         opts,
	}

	importers := []struct {
		name string
		fn   importerFn
	}{
		{name: "crds", fn: importCRDs},
		{name: "namespaces", fn: importNamespaces},
		{name: "cluster-resources", fn: importClusterResources},
		{name: "configmaps", fn: importCMs},
		{name: "secrets", fn: importSecrets},
	}

	var importErrors []error
	for _, importer := range importers {
		stageStartedAt := time.Now()
		err := importer.fn(ctx, cfg)
		stageDuration := time.Since(stageStartedAt).Round(time.Millisecond)
		if err != nil {
			importErrors = append(importErrors, fmt.Errorf("stage %q: %w", importer.name, err))
			out.Warnf("Import stage %q failed after %s", importer.name, stageDuration)
			continue
		}

		out.Infof("Import stage %q completed in %s", importer.name, stageDuration)
	}

	out.Infof(
		"Import stage metrics: duration=%s stages=%d failed-stages=%d concurrency=%d",
		time.Since(importStartedAt).Round(time.Millisecond), len(importers), len(importErrors), opts.Concurrency,
	)

	if len(importErrors) > 0 {
		out.Warn("\n!!! There were failures when importing the bundle data.")
		out.Warn("!!! The data in the API server are most likely incomplete\n")
		return errors.Join(importErrors...)
	}

	return nil
}

type importerConfig struct {
	dynamicClient   dynamic.Interface
	discoveryClient discovery.DiscoveryInterface
	bundle          bundle.Bundle
	out             output.Output
	options         ImportOptions
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

	if len(list.Items) == 0 {
		return nil
	}

	populateGVK(list, schema.GroupVersionKind{
		Version: "v1",
		Kind:    "Namespace",
	})

	gvr, includeStatus, err := detectGVRWithRetry(ctx, cfg.discoveryClient, &list.Items[0])
	if err != nil {
		return err
	}

	return list.EachListItem(func(o runtime.Object) error {
		return importObject(ctx, cfg.dynamicClient, gvr, o, includeStatus)
	})
}

func importClusterResources(
	ctx context.Context,
	cfg *importerConfig,
) error {
	tasks := []importTask{}

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

	err := afero.Walk(cfg.bundle, cfg.bundle.Layout().ClusterResources(), func(path string, info fs.FileInfo, err error) error {
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

		gvr, includeStatus, err := detectGVRWithRetry(ctx, cfg.discoveryClient, &list.Items[0])
		if err != nil {
			cfg.out.Errorf(err, "failed to detect GVR from file %q. CRD for the resource may not be imported:", path)
			return nil
		}

		for i := range list.Items {
			tasks = append(tasks, importTask{
				Stage:         stageClusterResources,
				SourcePath:    path,
				GVR:           gvr,
				IncludeStatus: includeStatus,
				Object:        list.Items[i].DeepCopy(),
			})
		}

		return nil
	})
	if err != nil {
		return err
	}

	return executeConcurrentStage(ctx, cfg, stageClusterResources, tasks)
}

type cmOrSecretLoadFn func(afero.Fs, string) (*unstructured.Unstructured, error)

func importCMOrSecrets(
	ctx context.Context,
	cfg *importerConfig,
	stage importStage,
	path string,
	loadFn cmOrSecretLoadFn,
	gvr schema.GroupVersionResource,
) error {
	tasks := []importTask{}

	err := afero.Walk(cfg.bundle, path, func(path string, info fs.FileInfo, err error) error {
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

		tasks = append(tasks, importTask{
			Stage:         stage,
			SourcePath:    path,
			GVR:           gvr,
			IncludeStatus: true,
			Object:        obj,
		})

		return nil
	})
	if err != nil {
		return err
	}

	return executeConcurrentStage(ctx, cfg, stage, tasks)
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
		ctx, cfg, stageConfigMaps, cfg.bundle.Layout().ConfigMaps(), bundle.LoadConfigMap, gvr)
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
		ctx, cfg, stageSecrets, cfg.bundle.Layout().Secrets(), bundle.LoadSecret, gvr)
}

func executeConcurrentStage(
	ctx context.Context,
	cfg *importerConfig,
	stage importStage,
	tasks []importTask,
) error {
	aggregator := newOutputAggregator(cfg.out, stage)
	executor := stageExecutor{
		workers: cfg.options.Concurrency,
	}

	return executor.Run(
		ctx,
		tasks,
		func(runCtx context.Context, task importTask) importResult {
			created, err := importObjectWithResult(
				runCtx, cfg.dynamicClient, task.GVR, task.Object, task.IncludeStatus,
			)
			return importResult{
				Stage:      task.Stage,
				SourcePath: task.SourcePath,
				GVR:        task.GVR,
				Namespace:  task.Object.GetNamespace(),
				Name:       task.Object.GetName(),
				Created:    created,
				Err:        err,
			}
		},
		aggregator,
	)
}

func importObject(
	ctx context.Context,
	cl dynamic.Interface,
	gvr schema.GroupVersionResource,
	o runtime.Object,
	includeStatus bool,
) error {
	_, err := importObjectWithResult(ctx, cl, gvr, o, includeStatus)
	return err
}

func importObjectWithResult(
	ctx context.Context,
	cl dynamic.Interface,
	gvr schema.GroupVersionResource,
	o runtime.Object,
	includeStatus bool,
) (bool, error) {
	if err := prepareForImport(o); err != nil {
		return false, err
	}

	u := o.(*unstructured.Unstructured)
	nsClient := cl.Resource(gvr).Namespace(u.GetNamespace())

	created, err := createResource(ctx, u, includeStatus, nsClient)
	if err != nil {
		return created, fmt.Errorf("failed to import resource: %w", err)
	}

	return created, nil
}

func createResource(
	ctx context.Context,
	u *unstructured.Unstructured,
	includeStatus bool,
	nsClient dynamic.ResourceInterface,
) (bool, error) {
	_, err := nsClient.Create(ctx, u, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return false, nil
		}

		return false, fmt.Errorf("failed to create resource: %w", err)
	}

	// Only import status for objects with status field
	if _, ok := u.Object["status"]; !ok || !includeStatus {
		return true, nil
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
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
	if err != nil {
		return true, err
	}

	return true, nil
}

func detectGVRWithRetry(
	ctx context.Context,
	cl discovery.DiscoveryInterface,
	u *unstructured.Unstructured,
) (schema.GroupVersionResource, bool, error) {
	const maxAttempts = 5
	const baseDelay = 200 * time.Millisecond

	var (
		lastErr       error
		includeStatus bool
		gvr           schema.GroupVersionResource
	)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		gvr, includeStatus, lastErr = detectGVR(cl, u)
		if lastErr == nil {
			return gvr, includeStatus, nil
		}

		if attempt == maxAttempts {
			break
		}

		delay := time.Duration(attempt) * baseDelay
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return schema.GroupVersionResource{}, false, ctx.Err()
		case <-timer.C:
		}
	}

	return schema.GroupVersionResource{}, false, lastErr
}
