package envtest

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"k8s.io/client-go/rest"
	controllerruntimeenvtest "sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/versions"
)

// APIServerStartConfig defines configuration passed to API server startup.
type APIServerStartConfig struct {
	StorageBackend StorageBackend
	StorageID      string
	FeatureGates   map[string]bool
}

// Environment wraps API server lifecycle for support-bundle replay.
//
// Storage lifecycle is managed outside of Environment so one storage backend can
// be shared across multiple API servers.
type Environment struct {
	BinaryAssetsDirectory string
	ControlPlane          controllerruntimeenvtest.ControlPlane
	Config                *rest.Config

	k8sVersion  versions.Selector
	startConfig APIServerStartConfig

	apiServerStarted bool

	startAPIServerFn func(*controllerruntimeenvtest.APIServer) error
	stopAPIServerFn  func(*controllerruntimeenvtest.APIServer) error
	addAdminUserFn   func(*controllerruntimeenvtest.ControlPlane) (*rest.Config, error)
}

func newEnvironment(binaryAssetsDirectory string, k8sVersion versions.Selector) *Environment {
	return &Environment{
		BinaryAssetsDirectory: binaryAssetsDirectory,
		k8sVersion:            k8sVersion,
		startConfig: APIServerStartConfig{
			FeatureGates: map[string]bool{},
		},
		startAPIServerFn: startAPIServer,
		stopAPIServerFn:  stopAPIServer,
		addAdminUserFn:   addAdminUser,
	}
}

// Start starts API server and returns admin rest config.
func (e *Environment) Start(ctx context.Context, opts ...StartOption) (*rest.Config, error) {
	if e.startAPIServerFn == nil {
		e.startAPIServerFn = startAPIServer
	}
	if e.stopAPIServerFn == nil {
		e.stopAPIServerFn = stopAPIServer
	}
	if e.addAdminUserFn == nil {
		e.addAdminUserFn = addAdminUser
	}

	apiServer := e.ControlPlane.GetAPIServer()
	if apiServer.Path == "" {
		apiServer.Path = filepath.Join(e.BinaryAssetsDirectory, "kube-apiserver")
	}

	startCfg := e.startConfig
	startCfg.FeatureGates = copyFeatureGates(startCfg.FeatureGates)
	for _, opt := range opts {
		opt(&startCfg)
	}
	defaultAPIServerFeatureGates(startCfg.FeatureGates, e.k8sVersion)

	if startCfg.StorageBackend == nil {
		return nil, errors.New("missing storage backend")
	}
	if startCfg.StorageID == "" {
		return nil, errors.New("missing storage id")
	}
	storageAllocation, err := startCfg.StorageBackend.Allocate(ctx, startCfg.StorageID)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate storage: %w", err)
	}

	if err := configureAPIServerStorage(apiServer, storageAllocation); err != nil {
		return nil, err
	}
	configureAPIServerFeatureGates(apiServer, startCfg.FeatureGates)

	if err := e.startAPIServerFn(apiServer); err != nil {
		return nil, fmt.Errorf("failed to start api server: %w", err)
	}
	e.apiServerStarted = true

	cfg, err := e.addAdminUserFn(&e.ControlPlane)
	if err != nil {
		_ = e.stopAPIServerFn(apiServer)
		e.apiServerStarted = false
		return nil, fmt.Errorf("failed to provision admin user: %w", err)
	}

	e.Config = cfg
	return cfg, nil
}

// Stop stops API server.
func (e *Environment) Stop() error {
	apiServer := e.ControlPlane.GetAPIServer()
	if e.apiServerStarted && apiServer != nil {
		err := e.stopAPIServerFn(apiServer)
		e.apiServerStarted = false
		return err
	}

	return nil
}

func configureAPIServerStorage(apiServer *controllerruntimeenvtest.APIServer, allocation *StorageAllocation) error {
	if allocation == nil || allocation.URL == nil {
		return errors.New("missing storage URL")
	}

	apiServer.EtcdURL = allocation.URL
	names := make([]string, 0, len(allocation.APIServerArgs))
	for name := range allocation.APIServerArgs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		apiServer.Configure().Set(name, allocation.APIServerArgs[name])
	}
	return nil
}

func copyFeatureGates(featureGates map[string]bool) map[string]bool {
	copied := make(map[string]bool, len(featureGates))
	for name, enabled := range featureGates {
		copied[name] = enabled
	}
	return copied
}

func defaultAPIServerFeatureGates(featureGates map[string]bool, k8sVersion versions.Selector) {
	if featureGates == nil {
		return
	}
	if _, ok := featureGates["WatchList"]; ok {
		return
	}
	if supportsWatchList(k8sVersion) {
		featureGates["WatchList"] = true
	}
}

func configureAPIServerFeatureGates(apiServer *controllerruntimeenvtest.APIServer, featureGates map[string]bool) {
	if len(featureGates) == 0 {
		return
	}

	names := make([]string, 0, len(featureGates))
	for name := range featureGates {
		names = append(names, name)
	}
	sort.Strings(names)

	values := make([]string, 0, len(names))
	for _, name := range names {
		values = append(values, name+"="+strconv.FormatBool(featureGates[name]))
	}
	apiServer.Configure().Set("feature-gates", strings.Join(values, ","))
}

func supportsWatchList(k8sVersion versions.Selector) bool {
	if k8sVersion == nil {
		return false
	}

	switch v := k8sVersion.(type) {
	case versions.Concrete:
		return v.Major > 1 || (v.Major == 1 && v.Minor >= 27)
	case versions.PatchSelector:
		return v.Major > 1 || (v.Major == 1 && v.Minor >= 27)
	default:
		concrete := k8sVersion.AsConcrete()
		return concrete != nil && (concrete.Major > 1 || (concrete.Major == 1 && concrete.Minor >= 27))
	}
}

func startAPIServer(apiServer *controllerruntimeenvtest.APIServer) error {
	return apiServer.Start()
}

func stopAPIServer(apiServer *controllerruntimeenvtest.APIServer) error {
	return apiServer.Stop()
}

func addAdminUser(cp *controllerruntimeenvtest.ControlPlane) (*rest.Config, error) {
	adminInfo := controllerruntimeenvtest.User{Name: "admin", Groups: []string{"system:masters"}}
	adminUser, err := cp.AddUser(adminInfo, &rest.Config{
		QPS:   1000.0,
		Burst: 2000.0,
	})
	if err != nil {
		return nil, err
	}

	return adminUser.Config(), nil
}
