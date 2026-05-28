package importer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mesosphere/dkp-cli-runtime/core/output"
	"github.com/spf13/afero"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/strings/slices"

	"github.com/mhrabovcin/troubleshoot-live/pkg/bundle"
	"github.com/mhrabovcin/troubleshoot-live/pkg/cli"
	"github.com/mhrabovcin/troubleshoot-live/pkg/utils"
)

const (
	defaultImportWorkers  = 8
	defaultCRDWaitTimeout = 60 * time.Second
)

var importRetryBackoff = wait.Backoff{
	Steps:    5,
	Duration: 100 * time.Millisecond,
	Factor:   2.0,
	Jitter:   0.2,
}

type importTask struct {
	sourcePath    string
	gvr           schema.GroupVersionResource
	object        *unstructured.Unstructured
	includeStatus bool
}

type workerPool struct {
	jobs chan importTask
	wg   sync.WaitGroup
	mu   sync.Mutex
	errs []error
	once sync.Once
}

func newWorkerPool(ctx context.Context, workers int, handler func(context.Context, importTask) error) *workerPool {
	if workers < 1 {
		workers = 1
	}
	wp := &workerPool{
		jobs: make(chan importTask, workers*2), // Buffer to avoid blocking disk I/O
	}

	for i := 0; i < workers; i++ {
		wp.wg.Add(1)
		go func() {
			defer wp.wg.Done()
			for task := range wp.jobs {
				if err := handler(ctx, task); err != nil {
					wp.mu.Lock()
					wp.errs = append(wp.errs, err)
					wp.mu.Unlock()
				}
			}
		}()
	}
	return wp
}

// Add returns an error if the context is cancelled, preventing silent task loss.
func (wp *workerPool) Add(ctx context.Context, task importTask) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case wp.jobs <- task:
		return nil
	}
}

// Close ensures the channel is closed safely and idempotently.
func (wp *workerPool) Close() {
	wp.once.Do(func() {
		close(wp.jobs)
	})
}

// Wait closes the channel and blocks until all workers finish.
func (wp *workerPool) Wait() error {
	wp.Close() // Ensure the channel is closed before waiting
	wp.wg.Wait()

	wp.mu.Lock()
	defer wp.mu.Unlock()
	if len(wp.errs) == 0 {
		return nil
	}
	return errors.Join(wp.errs...)
}

func newImportWorkerPool(ctx context.Context, cfg *importerConfig) *workerPool {
	return newWorkerPool(ctx, defaultImportWorkers, func(innerCtx context.Context, task importTask) error {
		err := importObjectWithRetry(innerCtx, cfg.dynamicClient, task.gvr, task.object, task.includeStatus, cfg.objectPreparer)
		if err != nil {
			cfg.out.Warnf(
				"Failed to import %q (%s) from %q with error: %s",
				objectReference(task.object), task.gvr, task.sourcePath, err,
			)
		}
		return err
	})
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
		objectPreparer:  defaultObjectPreparer(),
		gvrResolver:     newGVRResolver(discoveryClient),
		crdWaitTimeout:  defaultCRDWaitTimeout,
	}

	var importErrors []error
	importers := []importerFn{
		importCRDs,
		importNamespaces,
		importClusterResources,
		importCMs,
		importSecrets,
	}

	for _, importerFn := range importers {
		if err := importerFn(ctx, cfg); err != nil {
			importErrors = append(importErrors, err)
		}
	}

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
	objectPreparer  ObjectPreparer
	gvrResolver     *gvrResolver
	crdWaitTimeout  time.Duration
}

type importerFn func(context.Context, *importerConfig) error

func importNamespaces(
	ctx context.Context,
	cfg *importerConfig,
) (err error) {
	namespacesPath := filepath.Join(cfg.bundle.Layout().ClusterResources(), "namespaces.json")
	list, loadErr := bundle.LoadResourcesFromFile(cfg.bundle, namespacesPath)
	if loadErr != nil {
		cli.WarnOnErrorsFilePresence(cfg.bundle, cfg.out, namespacesPath)
		return loadErr
	}

	populateGVK(list, schema.GroupVersionKind{
		Version: "v1",
		Kind:    "Namespace",
	})

	if len(list.Items) == 0 {
		return nil
	}

	gvr, includeStatus, detectErr := cfg.gvrResolver.Detect(&list.Items[0])
	if detectErr != nil {
		return detectErr
	}

	cfg.out.V(1).Infof("Importing namespaces concurrently...")
	wp := newImportWorkerPool(ctx, cfg)

	defer func() {
		if wErr := wp.Wait(); wErr != nil {
			err = errors.Join(err, wErr)
		}
	}()

	var prepareErrors []error
	for i := range list.Items {
		u, errUns := asUnstructured(&list.Items[i])
		if errUns != nil {
			prepareErrors = append(prepareErrors, errUns)
			continue
		}

		addErr := wp.Add(ctx, importTask{
			sourcePath:    namespacesPath,
			gvr:           gvr,
			object:        u.DeepCopy(),
			includeStatus: includeStatus,
		})
		if addErr != nil {
			prepareErrors = append(prepareErrors, addErr)
			break // Context cancelled
		}
	}

	return errors.Join(prepareErrors...)
}

func importClusterResources(
	ctx context.Context,
	cfg *importerConfig,
) (err error) {
	skipResources := []string{
		"custom-resource-definitions.json",
		"pod-disruption-budgets-info.json",
		"resources.json",
		"groups.json",
		"namespaces.json",
	}

	skipDirs := []string{
		"auth-cani-list",
		"pod-disruption-budgets",
	}

	cfg.out.V(1).Infof("Importing cluster resources concurrently...")
	wp := newImportWorkerPool(ctx, cfg)

	defer func() {
		if wErr := wp.Wait(); wErr != nil {
			err = errors.Join(err, wErr)
		}
	}()

	var importErrors []error
	walkErr := afero.Walk(cfg.bundle, cfg.bundle.Layout().ClusterResources(), func(path string, info fs.FileInfo, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if walkErr != nil {
			cfg.out.Warnf("Failed to read file %q from bundle: %s", path, walkErr)
			return nil
		}

		if info.IsDir() && slices.Contains(skipDirs, filepath.Base(info.Name())) {
			return fs.SkipDir
		}

		if info.IsDir() || slices.Contains(skipResources, filepath.Base(info.Name())) {
			return nil
		}

		if strings.HasSuffix(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), "-errors") {
			return nil
		}

		list, loadErr := bundle.LoadResourcesFromFile(cfg.bundle, path)
		if loadErr != nil {
			cli.WarnOnErrorsFilePresence(cfg.bundle, cfg.out, path)
			cfg.out.Errorf(utils.MaxErrorString(loadErr, 200), "Failed to load resources from file %q", path)
			importErrors = append(importErrors, loadErr)
			return nil
		}

		if len(list.Items) == 0 {
			return nil
		}

		if list.Items[0].GetKind() == "" {
			relPath, relErr := filepath.Rel(cfg.bundle.Layout().ClusterResources(), path)
			if relErr != nil {
				return fmt.Errorf("failed to detect kind for path %q: %w", path, relErr)
			}
			if gvkLocal, errFromFile := gvkFromFile(relPath); errFromFile == nil {
				populateGVK(list, gvkLocal)
			}
		}

		cfg.out.V(1).Infof("Loading objects from: %s ...", path)

		gvr, includeStatus, detectErr := cfg.gvrResolver.Detect(&list.Items[0])
		if detectErr != nil {
			cfg.out.Errorf(detectErr, "failed to detect GVR from file %q. CRD for the resource may not be imported:", path)
			importErrors = append(importErrors, detectErr)
			return nil
		}

		for i := range list.Items {
			addErr := wp.Add(ctx, importTask{
				sourcePath:    path,
				gvr:           gvr,
				object:        list.Items[i].DeepCopy(),
				includeStatus: includeStatus,
			})
			if addErr != nil {
				return addErr // Context cancelled, abort Walk
			}
		}

		return nil
	})

	if walkErr != nil {
		importErrors = append(importErrors, walkErr)
	}

	return errors.Join(importErrors...)
}

type cmOrSecretLoadFn func(afero.Fs, string) (*unstructured.Unstructured, error)

func importCMOrSecrets(
	ctx context.Context,
	cfg *importerConfig,
	path string,
	loadFn cmOrSecretLoadFn,
	gvr schema.GroupVersionResource,
) (err error) {
	wp := newImportWorkerPool(ctx, cfg)

	defer func() {
		if wErr := wp.Wait(); wErr != nil {
			err = errors.Join(err, wErr)
		}
	}()

	var importErrors []error
	walkErr := afero.Walk(cfg.bundle, path, func(path string, info fs.FileInfo, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if walkErr != nil {
			cfg.out.Errorf(walkErr, "Failed to read file %q from bundle", path)
			importErrors = append(importErrors, walkErr)
			return nil
		}

		if info.IsDir() {
			return nil
		}

		cfg.out.V(1).Infof("Importing %s from: %s ... ", gvr.Resource, path)

		obj, loadErr := loadFn(cfg.bundle, path)
		if loadErr != nil {
			cfg.out.Errorf(utils.MaxErrorString(loadErr, 200), "Failed to import secret from %q", path)
			importErrors = append(importErrors, loadErr)
			return nil
		}

		addErr := wp.Add(ctx, importTask{
			sourcePath:    path,
			gvr:           gvr,
			object:        obj,
			includeStatus: true,
		})
		if addErr != nil {
			return addErr // Context cancelled, abort Walk
		}

		return nil
	})

	if walkErr != nil {
		importErrors = append(importErrors, walkErr)
	}

	return errors.Join(importErrors...)
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
	o *unstructured.Unstructured,
	includeStatus bool,
	preparer ObjectPreparer,
) error {
	_, err := importObjectWithResult(ctx, cl, gvr, o, includeStatus, preparer)
	return err
}

// importObjectWithResult returns true if the object was created, or an error otherwise.
func importObjectWithResult(
	ctx context.Context,
	cl dynamic.Interface,
	gvr schema.GroupVersionResource,
	o *unstructured.Unstructured,
	includeStatus bool,
	preparer ObjectPreparer,
) (bool, error) {
	if err := preparer.Prepare(o); err != nil {
		return false, err
	}

	_, err := cl.Resource(gvr).Namespace(o.GetNamespace()).Get(ctx, o.GetName(), metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("failed to get resource: %w", err)
		}
		nsClient := cl.Resource(gvr).Namespace(o.GetNamespace())

		created, err := createResource(ctx, o, includeStatus, nsClient)
		if err != nil {
			return false, fmt.Errorf("failed to import resource: %w", err)
		}
		return created, nil
	}

	return false, nil
}

func asUnstructured(o runtime.Object) (*unstructured.Unstructured, error) {
	u, ok := o.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("expected *unstructured.Unstructured, got %T", o)
	}
	return u, nil
}

func createResource(ctx context.Context, u *unstructured.Unstructured, includeStatus bool, nsClient dynamic.ResourceInterface) (bool, error) {
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

	return true, err
}

func objectReference(u *unstructured.Unstructured) string {
	if u == nil {
		return "<nil>"
	}
	if u.GetNamespace() == "" {
		return u.GetName()
	}
	return fmt.Sprintf("%s/%s", u.GetNamespace(), u.GetName())
}

func importObjectWithRetry(
	ctx context.Context,
	cl dynamic.Interface,
	gvr schema.GroupVersionResource,
	o *unstructured.Unstructured,
	includeStatus bool,
	preparer ObjectPreparer,
) error {
	return retry.OnError(importRetryBackoff, isRetryableImportErr, func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return importObject(ctx, cl, gvr, o.DeepCopy(), includeStatus, preparer)
	})
}

func isRetryableImportErr(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	return apierrors.IsTooManyRequests(err) ||
		apierrors.IsTimeout(err) ||
		apierrors.IsServerTimeout(err) ||
		apierrors.IsServiceUnavailable(err) ||
		apierrors.IsInternalError(err) ||
		apierrors.IsNotFound(err)
}

func errorOrNil(err error) []error {
	if err == nil {
		return nil
	}
	return []error{err}
}
