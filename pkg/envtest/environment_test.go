package envtest

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	controllerruntimeenvtest "sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/versions"
)

func testK8sVersion(minor int) versions.Selector {
	return versions.PatchSelector{
		Major: 1,
		Minor: minor,
		Patch: versions.AnyPoint,
	}
}

type fakeStorageBackend struct {
	allocation  *StorageAllocation
	allocateErr error
	allocateIDs []string
	stopped     int
}

func (f *fakeStorageBackend) Start(_ context.Context) error {
	return nil
}

func (f *fakeStorageBackend) Allocate(_ context.Context, id string) (*StorageAllocation, error) {
	f.allocateIDs = append(f.allocateIDs, id)
	if f.allocateErr != nil {
		return nil, f.allocateErr
	}
	return f.allocation, nil
}

func (f *fakeStorageBackend) Stop() error {
	f.stopped++
	return nil
}

func storageBackend(t *testing.T) *fakeStorageBackend {
	t.Helper()

	endpoint, err := url.Parse("http://127.0.0.1:2379")
	require.NoError(t, err)
	return &fakeStorageBackend{
		allocation: &StorageAllocation{
			URL: endpoint,
		},
	}
}

func TestEnvironmentStartRequiresStorageBackend(t *testing.T) {
	env := newEnvironment("/tmp/envtest-assets", testK8sVersion(27))
	env.startAPIServerFn = func(_ *controllerruntimeenvtest.APIServer) error { return nil }
	env.addAdminUserFn = func(_ *controllerruntimeenvtest.ControlPlane) (*rest.Config, error) {
		return &rest.Config{Host: "https://127.0.0.1:6443"}, nil
	}

	_, err := env.Start(context.Background(), WithStorageID("default"))
	require.Error(t, err)
	assert.EqualError(t, err, "missing storage backend")
}

func TestEnvironmentStartRequiresStorageID(t *testing.T) {
	env := newEnvironment("/tmp/envtest-assets", testK8sVersion(27))
	env.startAPIServerFn = func(_ *controllerruntimeenvtest.APIServer) error { return nil }
	env.addAdminUserFn = func(_ *controllerruntimeenvtest.ControlPlane) (*rest.Config, error) {
		return &rest.Config{Host: "https://127.0.0.1:6443"}, nil
	}

	_, err := env.Start(context.Background(), WithStorageBackend(storageBackend(t)))
	require.Error(t, err)
	assert.EqualError(t, err, "missing storage id")
}

func TestEnvironmentStartUsesStorageAllocation(t *testing.T) {
	storage := storageBackend(t)
	storage.allocation.APIServerArgs = map[string]string{
		"etcd-prefix": "/registry/test",
	}

	env := newEnvironment("/tmp/envtest-assets", testK8sVersion(27))
	env.startAPIServerFn = func(_ *controllerruntimeenvtest.APIServer) error { return nil }
	env.addAdminUserFn = func(_ *controllerruntimeenvtest.ControlPlane) (*rest.Config, error) {
		return &rest.Config{Host: "https://127.0.0.1:6443"}, nil
	}

	_, err := env.Start(context.Background(),
		WithStorageBackend(storage),
		WithStorageID("test"),
	)
	require.NoError(t, err)

	apiServer := env.ControlPlane.GetAPIServer()
	require.NotNil(t, apiServer.EtcdURL)
	assert.Equal(t, storage.allocation.URL.String(), apiServer.EtcdURL.String())
	assert.Equal(t, []string{"/registry/test"}, apiServer.Configure().Get("etcd-prefix").Get(nil))
	assert.Equal(t, []string{"test"}, storage.allocateIDs)
}

func TestEnvironmentStartReturnsStorageAllocationError(t *testing.T) {
	storage := storageBackend(t)
	storage.allocateErr = errors.New("boom")

	env := newEnvironment("/tmp/envtest-assets", testK8sVersion(27))
	env.startAPIServerFn = func(_ *controllerruntimeenvtest.APIServer) error { return nil }
	env.addAdminUserFn = func(_ *controllerruntimeenvtest.ControlPlane) (*rest.Config, error) {
		return &rest.Config{Host: "https://127.0.0.1:6443"}, nil
	}

	_, err := env.Start(context.Background(),
		WithStorageBackend(storage),
		WithStorageID("default"),
	)
	require.Error(t, err)
	assert.EqualError(t, err, "failed to allocate storage: boom")
}

func TestEnvironmentStartDefaultsWatchListFeatureGateForSupportedVersion(t *testing.T) {
	env := newEnvironment("/tmp/envtest-assets", testK8sVersion(27))
	env.startAPIServerFn = func(_ *controllerruntimeenvtest.APIServer) error { return nil }
	env.addAdminUserFn = func(_ *controllerruntimeenvtest.ControlPlane) (*rest.Config, error) {
		return &rest.Config{Host: "https://127.0.0.1:6443"}, nil
	}

	_, err := env.Start(context.Background(),
		WithStorageBackend(storageBackend(t)),
		WithStorageID("default"),
	)
	require.NoError(t, err)

	apiServer := env.ControlPlane.GetAPIServer()
	assert.Equal(t, []string{"WatchList=true"}, apiServer.Configure().Get("feature-gates").Get(nil))
}

func TestEnvironmentStartSkipsWatchListFeatureGateForUnsupportedVersion(t *testing.T) {
	env := newEnvironment("/tmp/envtest-assets", testK8sVersion(26))
	env.startAPIServerFn = func(_ *controllerruntimeenvtest.APIServer) error { return nil }
	env.addAdminUserFn = func(_ *controllerruntimeenvtest.ControlPlane) (*rest.Config, error) {
		return &rest.Config{Host: "https://127.0.0.1:6443"}, nil
	}

	_, err := env.Start(context.Background(),
		WithStorageBackend(storageBackend(t)),
		WithStorageID("default"),
	)
	require.NoError(t, err)

	apiServer := env.ControlPlane.GetAPIServer()
	assert.Nil(t, apiServer.Configure().Get("feature-gates").Get(nil))
}

func TestEnvironmentStartUsesExplicitFeatureGates(t *testing.T) {
	env := newEnvironment("/tmp/envtest-assets", testK8sVersion(27))
	env.startAPIServerFn = func(_ *controllerruntimeenvtest.APIServer) error { return nil }
	env.addAdminUserFn = func(_ *controllerruntimeenvtest.ControlPlane) (*rest.Config, error) {
		return &rest.Config{Host: "https://127.0.0.1:6443"}, nil
	}

	_, err := env.Start(context.Background(),
		WithStorageBackend(storageBackend(t)),
		WithStorageID("default"),
		WithAPIServerFeatureGates(map[string]bool{
			"OtherFeature": true,
			"WatchList":    false,
		}),
	)
	require.NoError(t, err)

	apiServer := env.ControlPlane.GetAPIServer()
	assert.Equal(t, []string{"OtherFeature=true,WatchList=false"}, apiServer.Configure().Get("feature-gates").Get(nil))
}

func TestEnvironmentStopStopsOnlyAPIServer(t *testing.T) {
	storage := storageBackend(t)
	env := newEnvironment("/tmp/envtest-assets", testK8sVersion(27))

	stopped := 0
	env.startAPIServerFn = func(_ *controllerruntimeenvtest.APIServer) error { return nil }
	env.stopAPIServerFn = func(_ *controllerruntimeenvtest.APIServer) error {
		stopped++
		return nil
	}
	env.addAdminUserFn = func(_ *controllerruntimeenvtest.ControlPlane) (*rest.Config, error) {
		return &rest.Config{Host: "https://127.0.0.1:6443"}, nil
	}

	_, err := env.Start(context.Background(),
		WithStorageBackend(storage),
		WithStorageID("default"),
	)
	require.NoError(t, err)
	require.NoError(t, env.Stop())
	assert.Equal(t, 1, stopped)
	assert.Zero(t, storage.stopped)
}

func TestConfigureAPIServerStorageSetsURLAndArgs(t *testing.T) {
	endpoint, err := url.Parse("http://127.0.0.1:1234")
	require.NoError(t, err)

	apiServer := &controllerruntimeenvtest.APIServer{}
	err = configureAPIServerStorage(apiServer, &StorageAllocation{
		URL: endpoint,
		APIServerArgs: map[string]string{
			"etcd-prefix": "/registry/test",
		},
	})
	require.NoError(t, err)

	require.NotNil(t, apiServer.EtcdURL)
	assert.Equal(t, endpoint.String(), apiServer.EtcdURL.String())
	assert.Equal(
		t,
		[]string{"/registry/test"},
		apiServer.Configure().Get("etcd-prefix").Get(nil),
	)
}

func TestConfigureAPIServerStorageDoesNotSetPrefixWhenAllocationOmitsIt(t *testing.T) {
	endpoint, err := url.Parse("http://127.0.0.1:1234")
	require.NoError(t, err)

	apiServer := &controllerruntimeenvtest.APIServer{}
	err = configureAPIServerStorage(apiServer, &StorageAllocation{
		URL: endpoint,
	})
	require.NoError(t, err)

	assert.Nil(t, apiServer.Configure().Get("etcd-prefix").Get(nil))
}

func TestConfigureAPIServerStorageRequiresURL(t *testing.T) {
	apiServer := &controllerruntimeenvtest.APIServer{}
	err := configureAPIServerStorage(apiServer, &StorageAllocation{})
	require.Error(t, err)
	assert.EqualError(t, err, "missing storage URL")
}
