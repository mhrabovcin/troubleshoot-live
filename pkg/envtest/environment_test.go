package envtest

import (
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

func TestNewLocalEtcdDatastoreUsesBinaryAssetsDirectory(t *testing.T) {
	ds := NewLocalEtcdDatastore("/tmp/envtest-assets")
	local, ok := ds.(*localEtcdDatastore)
	require.True(t, ok)
	require.NotNil(t, local.etcd)
	assert.Equal(t, "/tmp/envtest-assets/etcd", local.etcd.Path)
}

func TestEnvironmentStartRequiresStorageEndpoint(t *testing.T) {
	env := newEnvironment("/tmp/envtest-assets", testK8sVersion(27))
	env.startAPIServerFn = func(_ *controllerruntimeenvtest.APIServer) error { return nil }
	env.addAdminUserFn = func(_ *controllerruntimeenvtest.ControlPlane) (*rest.Config, error) {
		return &rest.Config{Host: "https://127.0.0.1:6443"}, nil
	}

	_, err := env.Start()
	require.Error(t, err)
	assert.EqualError(t, err, "missing datastore endpoint")
}

func TestEnvironmentStartUsesConfiguredStorageEndpointAndPrefix(t *testing.T) {
	endpoint, err := url.Parse("http://127.0.0.1:2379")
	require.NoError(t, err)

	env := newEnvironment("/tmp/envtest-assets", testK8sVersion(27))
	env.startAPIServerFn = func(_ *controllerruntimeenvtest.APIServer) error { return nil }
	env.addAdminUserFn = func(_ *controllerruntimeenvtest.ControlPlane) (*rest.Config, error) {
		return &rest.Config{Host: "https://127.0.0.1:6443"}, nil
	}

	_, err = env.Start(
		WithDatastoreEndpoint(endpoint),
		WithDatastorePrefix("/registry/test"),
	)
	require.NoError(t, err)

	apiServer := env.ControlPlane.GetAPIServer()
	require.NotNil(t, apiServer.EtcdURL)
	assert.Equal(t, endpoint.String(), apiServer.EtcdURL.String())
	assert.Equal(t, []string{"/registry/test"}, apiServer.Configure().Get("etcd-prefix").Get(nil))
}

func TestEnvironmentStartDefaultsWatchListFeatureGateForSupportedVersion(t *testing.T) {
	endpoint, err := url.Parse("http://127.0.0.1:2379")
	require.NoError(t, err)

	env := newEnvironment("/tmp/envtest-assets", testK8sVersion(27))
	env.startAPIServerFn = func(_ *controllerruntimeenvtest.APIServer) error { return nil }
	env.addAdminUserFn = func(_ *controllerruntimeenvtest.ControlPlane) (*rest.Config, error) {
		return &rest.Config{Host: "https://127.0.0.1:6443"}, nil
	}

	_, err = env.Start(WithDatastoreEndpoint(endpoint))
	require.NoError(t, err)

	apiServer := env.ControlPlane.GetAPIServer()
	assert.Equal(t, []string{"WatchList=true"}, apiServer.Configure().Get("feature-gates").Get(nil))
}

func TestEnvironmentStartSkipsWatchListFeatureGateForUnsupportedVersion(t *testing.T) {
	endpoint, err := url.Parse("http://127.0.0.1:2379")
	require.NoError(t, err)

	env := newEnvironment("/tmp/envtest-assets", testK8sVersion(26))
	env.startAPIServerFn = func(_ *controllerruntimeenvtest.APIServer) error { return nil }
	env.addAdminUserFn = func(_ *controllerruntimeenvtest.ControlPlane) (*rest.Config, error) {
		return &rest.Config{Host: "https://127.0.0.1:6443"}, nil
	}

	_, err = env.Start(WithDatastoreEndpoint(endpoint))
	require.NoError(t, err)

	apiServer := env.ControlPlane.GetAPIServer()
	assert.Nil(t, apiServer.Configure().Get("feature-gates").Get(nil))
}

func TestEnvironmentStartUsesExplicitFeatureGates(t *testing.T) {
	endpoint, err := url.Parse("http://127.0.0.1:2379")
	require.NoError(t, err)

	env := newEnvironment("/tmp/envtest-assets", testK8sVersion(27))
	env.startAPIServerFn = func(_ *controllerruntimeenvtest.APIServer) error { return nil }
	env.addAdminUserFn = func(_ *controllerruntimeenvtest.ControlPlane) (*rest.Config, error) {
		return &rest.Config{Host: "https://127.0.0.1:6443"}, nil
	}

	_, err = env.Start(
		WithDatastoreEndpoint(endpoint),
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
	endpoint, err := url.Parse("http://127.0.0.1:2379")
	require.NoError(t, err)

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

	_, err = env.Start(WithDatastoreEndpoint(endpoint))
	require.NoError(t, err)
	require.NoError(t, env.Stop())
	assert.Equal(t, 1, stopped)
}

func TestConfigureAPIServerStorageSetsEndpointAndPrefix(t *testing.T) {
	endpoint, err := url.Parse("http://127.0.0.1:1234")
	require.NoError(t, err)

	apiServer := &controllerruntimeenvtest.APIServer{}
	err = configureAPIServerStorage(apiServer, APIServerStorageConfig{
		Endpoint: endpoint,
		Prefix:   "/registry/test",
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

func TestConfigureAPIServerStorageRequiresEndpoint(t *testing.T) {
	apiServer := &controllerruntimeenvtest.APIServer{}
	err := configureAPIServerStorage(apiServer, APIServerStorageConfig{})
	require.Error(t, err)
	assert.EqualError(t, err, "missing datastore endpoint")
}
