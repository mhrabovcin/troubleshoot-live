package envtest

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"k8s.io/client-go/rest"
	controllerruntimeenvtest "sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/versions"
)

// DatastoreConnection describes datastore endpoint details for API server wiring.
type DatastoreConnection struct {
	Endpoint *url.URL
	Metadata map[string]string
}

// APIServerStorageConfig defines storage-level configuration passed to API server startup.
type APIServerStorageConfig struct {
	Endpoint *url.URL
	Prefix   string
}

// APIServerStartConfig defines configuration passed to API server startup.
type APIServerStartConfig struct {
	Storage      APIServerStorageConfig
	FeatureGates map[string]bool
}

// Datastore is a handle for datastore lifecycle and connection details.
//
// Datastore lifecycle is intentionally managed by callers outside of
// Environment so one datastore can be shared across multiple API servers.
type Datastore interface {
	Start(context.Context) (*DatastoreConnection, error)
	Stop() error
}

// NewLocalEtcdDatastore creates a local etcd datastore handle.
func NewLocalEtcdDatastore(binaryAssetsDirectory string) Datastore {
	return &localEtcdDatastore{
		etcd: &controllerruntimeenvtest.Etcd{
			Path: filepath.Join(binaryAssetsDirectory, "etcd"),
		},
	}
}

type localEtcdDatastore struct {
	etcd *controllerruntimeenvtest.Etcd
}

func (d *localEtcdDatastore) Start(_ context.Context) (*DatastoreConnection, error) {
	if err := d.etcd.Start(); err != nil {
		return nil, err
	}

	return &DatastoreConnection{
		Endpoint: d.etcd.URL,
		Metadata: map[string]string{
			"type": "local-etcd",
		},
	}, nil
}

func (d *localEtcdDatastore) Stop() error {
	return d.etcd.Stop()
}

// Environment wraps API server lifecycle for support-bundle replay.
//
// The current default runtime behavior remains unchanged: one local etcd process
// per API server instance. The additional abstraction is intentionally internal
// and lays groundwork for future shared datastore implementations.
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
func (e *Environment) Start(opts ...StartOption) (*rest.Config, error) {
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

	if err := configureAPIServerStorage(apiServer, startCfg.Storage); err != nil {
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

func configureAPIServerStorage(apiServer *controllerruntimeenvtest.APIServer, cfg APIServerStorageConfig) error {
	if cfg.Endpoint == nil {
		return errors.New("missing datastore endpoint")
	}

	apiServer.EtcdURL = cfg.Endpoint
	if cfg.Prefix != "" {
		apiServer.Configure().Set("etcd-prefix", cfg.Prefix)
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
