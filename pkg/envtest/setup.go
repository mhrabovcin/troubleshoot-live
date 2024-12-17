package envtest

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"runtime"

	"github.com/go-logr/logr"
	"github.com/spf13/afero"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/env"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/remote"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/store"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/versions"

	"github.com/mhrabovcin/troubleshoot-live/pkg/bundle"
)

// Environment is alias for envtest.Environment.
type Environment = envtest.Environment

// Prepare creates k8s environment for the provided bundle by detecting the
// k8s version and downloading necessary envtest assets for launching the
// detected k8s version.
func Prepare(ctx context.Context, b bundle.Bundle, opts ...Option) (*envtest.Environment, error) {
	detectedK8sVersion, err := DetectK8sVersion(b)
	if err != nil {
		return nil, fmt.Errorf("failed to detect k8s version: %s", err)
	}
	log.Printf("Detected %q k8s version", detectedK8sVersion)

	versionSpec := versions.Spec{
		Selector: detectedK8sVersion,
	}

	envConfig, err := createEnvtest(ctx, versionSpec)
	if err != nil {
		return nil, err
	}

	for _, o := range opts {
		o(envConfig)
	}

	binaryAssetsDirectory, err := setupEnvtest(ctx, envConfig)
	if err != nil {
		return nil, err
	}

	log.Printf("Using envtest binaries from directory: %s\n", binaryAssetsDirectory)
	return &envtest.Environment{
		BinaryAssetsDirectory: binaryAssetsDirectory,
	}, nil
}

func setupEnvtest(ctx context.Context, e *env.Env) (_ string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("failed setting up env: %s", r)
		}
	}()

	ctx = logr.NewContext(ctx, e.Log)

	e.CheckCoherence()
	e.EnsureBaseDirs(ctx)
	e.EnsureVersionIsSet(ctx)
	if !e.ExistsAndValid() {
		e.Fetch(ctx)
	}
	out := &bytes.Buffer{}
	e.Out = out
	e.PrintInfo(env.PrintPath)
	e.Out = nil
	return out.String(), err
}

func createEnvtest(ctx context.Context, serverVersion versions.Spec) (*env.Env, error) {
	dataDir, err := store.DefaultStoreDir()
	if err != nil {
		return nil, err
	}

	logger := logr.FromContextOrDiscard(ctx)
	return &env.Env{
		Log:     logger,
		Version: serverVersion,
		Client: &remote.HTTPClient{
			Log: logger.WithName("envtest-client"),
		},
		VerifySum:     false, // todo: expose?
		ForceDownload: false, // todo: expose?
		NoDownload:    false, // todo: expose?
		Platform: versions.PlatformItem{
			Platform: versions.Platform{
				OS:   runtime.GOOS,
				Arch: runtime.GOARCH,
			},
		},
		FS:    afero.Afero{Fs: afero.NewOsFs()},
		Store: store.NewAt(dataDir),
	}, nil
}
