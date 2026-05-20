package envtest

import (
	"context"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	controllerruntimeenvtest "sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestNewLocalEtcdStorageBackendUsesBinaryAssetsDirectory(t *testing.T) {
	ds := NewLocalEtcdStorageBackend("/tmp/envtest-assets")
	local, ok := ds.(*localEtcdStorageBackend)
	require.True(t, ok)
	require.NotNil(t, local.etcd)
	assert.Equal(t, "/tmp/envtest-assets/etcd", local.etcd.Path)
}

func TestLocalEtcdStorageBackendAllocateUsesPerBundlePrefixes(t *testing.T) {
	endpoint, err := url.Parse("http://127.0.0.1:2379")
	require.NoError(t, err)

	storage := NewLocalEtcdStorageBackend("/tmp/envtest-assets")
	local, ok := storage.(*localEtcdStorageBackend)
	require.True(t, ok)
	local.started = true
	local.etcd.URL = endpoint

	first, err := local.Allocate(context.Background(), "first")
	require.NoError(t, err)
	second, err := local.Allocate(context.Background(), "second")
	require.NoError(t, err)

	assert.Equal(t, endpoint.String(), first.URL.String())
	assert.Equal(t, endpoint.String(), second.URL.String())
	assert.Equal(t, "/registry/first", first.APIServerArgs["etcd-prefix"])
	assert.Equal(t, "/registry/second", second.APIServerArgs["etcd-prefix"])
}

func TestLocalEtcdStorageBackendAllocateCanDisablePrefixes(t *testing.T) {
	endpoint, err := url.Parse("http://127.0.0.1:2379")
	require.NoError(t, err)

	storage := NewLocalEtcdStorageBackend(
		"/tmp/envtest-assets",
		WithLocalEtcdPrefixRoot(""),
	)
	local, ok := storage.(*localEtcdStorageBackend)
	require.True(t, ok)
	local.started = true
	local.etcd.URL = endpoint

	allocation, err := local.Allocate(context.Background(), "default")
	require.NoError(t, err)

	assert.Empty(t, allocation.APIServerArgs)
}

func TestLocalEtcdStorageBackendAllocateRequiresStartedBackend(t *testing.T) {
	storage := NewLocalEtcdStorageBackend("/tmp/envtest-assets")

	_, err := storage.Allocate(context.Background(), "default")
	require.Error(t, err)
	assert.EqualError(t, err, "storage backend is not started")
}

func TestLocalEtcdStorageBackendStartIsIdempotent(t *testing.T) {
	storage := NewLocalEtcdStorageBackend("/tmp/envtest-assets")
	local, ok := storage.(*localEtcdStorageBackend)
	require.True(t, ok)

	started := 0
	local.startEtcdFn = func(_ *controllerruntimeenvtest.Etcd) error {
		started++
		return nil
	}

	require.NoError(t, storage.Start(context.Background()))
	require.NoError(t, storage.Start(context.Background()))

	assert.Equal(t, 1, started)
}
