package envtest

import (
	"context"
	"errors"
	"net/url"
	"path"
	"path/filepath"

	controllerruntimeenvtest "sigs.k8s.io/controller-runtime/pkg/envtest"
)

// StorageAllocation describes storage details for one API server.
type StorageAllocation struct {
	URL           *url.URL
	APIServerArgs map[string]string
}

// StorageBackend is a handle for storage lifecycle and per-API-server allocation.
//
// StorageBackend lifecycle is intentionally managed by callers outside of
// Environment so one backend can be shared across multiple API servers.
type StorageBackend interface {
	Start(context.Context) error
	Allocate(context.Context, string) (*StorageAllocation, error)
	Stop() error
}

// LocalEtcdStorageOption configures local etcd storage.
type LocalEtcdStorageOption func(*localEtcdStorageBackend)

// WithLocalEtcdPrefixRoot configures the root used for per-bundle etcd prefixes.
func WithLocalEtcdPrefixRoot(prefixRoot string) LocalEtcdStorageOption {
	return func(b *localEtcdStorageBackend) {
		b.prefixRoot = prefixRoot
	}
}

// NewLocalEtcdStorageBackend creates a local etcd storage backend.
func NewLocalEtcdStorageBackend(binaryAssetsDirectory string, opts ...LocalEtcdStorageOption) StorageBackend {
	backend := &localEtcdStorageBackend{
		etcd: &controllerruntimeenvtest.Etcd{
			Path: filepath.Join(binaryAssetsDirectory, "etcd"),
		},
		prefixRoot:  "/registry",
		startEtcdFn: startLocalEtcd,
		stopEtcdFn:  stopLocalEtcd,
	}
	for _, opt := range opts {
		opt(backend)
	}
	return backend
}

type localEtcdStorageBackend struct {
	etcd        *controllerruntimeenvtest.Etcd
	prefixRoot  string
	started     bool
	startEtcdFn func(*controllerruntimeenvtest.Etcd) error
	stopEtcdFn  func(*controllerruntimeenvtest.Etcd) error
}

func (b *localEtcdStorageBackend) Start(_ context.Context) error {
	if b.started {
		return nil
	}
	if b.startEtcdFn == nil {
		b.startEtcdFn = startLocalEtcd
	}
	if err := b.startEtcdFn(b.etcd); err != nil {
		return err
	}
	b.started = true
	return nil
}

func (b *localEtcdStorageBackend) Allocate(_ context.Context, bundleID string) (*StorageAllocation, error) {
	if bundleID == "" {
		return nil, errors.New("missing storage id")
	}
	if !b.started || b.etcd.URL == nil {
		return nil, errors.New("storage backend is not started")
	}

	allocation := &StorageAllocation{
		URL:           b.etcd.URL,
		APIServerArgs: map[string]string{},
	}
	if b.prefixRoot != "" {
		allocation.APIServerArgs["etcd-prefix"] = path.Join(b.prefixRoot, bundleID)
	}
	return allocation, nil
}

func (b *localEtcdStorageBackend) Stop() error {
	if !b.started {
		return nil
	}
	if b.stopEtcdFn == nil {
		b.stopEtcdFn = stopLocalEtcd
	}
	if err := b.stopEtcdFn(b.etcd); err != nil {
		return err
	}
	b.started = false
	return nil
}

func startLocalEtcd(etcd *controllerruntimeenvtest.Etcd) error {
	return etcd.Start()
}

func stopLocalEtcd(etcd *controllerruntimeenvtest.Etcd) error {
	return etcd.Stop()
}
