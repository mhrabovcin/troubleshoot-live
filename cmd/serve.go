package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/gorilla/handlers"

	"github.com/mesosphere/dkp-cli-runtime/core/output"
	"github.com/spf13/cobra"

	"github.com/mhrabovcin/troubleshoot-live/pkg/bundle"
	"github.com/mhrabovcin/troubleshoot-live/pkg/envtest"
	"github.com/mhrabovcin/troubleshoot-live/pkg/importer"
	"github.com/mhrabovcin/troubleshoot-live/pkg/kubernetes"
	"github.com/mhrabovcin/troubleshoot-live/pkg/proxy"
	"github.com/mhrabovcin/troubleshoot-live/pkg/rewriter"
)

type serveOptions struct {
	kubeconfigPath        string
	proxyAddress          string
	envtestArch           string
	serviceClusterIPRange string
	serviceNodePortRange  string
}

// NewServeCommand serves the provided bundle.
func NewServeCommand(out output.Output) *cobra.Command {
	options := &serveOptions{
		kubeconfigPath: "./support-bundle-kubeconfig",
		proxyAddress:   "localhost:8080",
		envtestArch:    runtime.GOARCH,
	}

	cmd := &cobra.Command{
		Use:   "serve SUPPORT_BUNDLE_PATH",
		Short: "Starts a local envtest based Kubernetes API server with bundle resources",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(args[0], options, out)
		},
	}

	cmd.Flags().StringVar(
		&options.kubeconfigPath,
		"output-kubeconfig", options.kubeconfigPath,
		"where to write kubeconfig path",
	)

	cmd.Flags().StringVar(
		&options.proxyAddress, "proxy-address", options.proxyAddress,
		"value of k8s proxy server",
	)

	cmd.Flags().StringVar(
		&options.envtestArch, "envtest-arch", options.envtestArch,
		"arch value for k8s server assets",
	)

	cmd.Flags().StringVar(
		&options.serviceClusterIPRange, "service-cluster-ip-range", options.serviceClusterIPRange,
		"override k8s api server service ClusterIP range. Mask must be >= /12 range.",
	)

	cmd.Flags().StringVar(
		&options.serviceNodePortRange, "service-node-port-range", options.serviceNodePortRange,
		"override k8s api server service node port range",
	)

	return cmd
}

func runServe(bundlePath string, o *serveOptions, out output.Output) error {
	supportBundle, err := bundle.New(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to get bundle from path %q: %w", bundlePath, err)
	}

	ctx, done := signal.NotifyContext(context.Background(), syscall.SIGINT)
	defer done()

	out.StartOperation("Starting k8s server")
	testEnv, err := startK8sServer(ctx, supportBundle, out, o)
	out.EndOperation(err == nil)
	if err != nil {
		return err
	}

	defer func() {
		if err := testEnv.Stop(); err != nil {
			out.Error(err, "failed to stop k8s api server")
		}
	}()

	out.StartOperation("Importing bundle resources")
	err = importer.ImportBundle(ctx, supportBundle, testEnv.Config, out)
	out.EndOperation(err == nil)
	if err != nil {
		out.Error(err, "failed to import support bundle resources to API server")
	}

	proxyHTTPAddress := fmt.Sprintf("http://%s", o.proxyAddress)
	kubeconfigPath, err := kubernetes.WriteProxyKubeconfig(proxyHTTPAddress, o.kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to create kubeconfig: %w", err)
	}

	out.Infof("Running HTTPs proxy service on: %s", proxyHTTPAddress)
	out.Infof("KUBECONFIG=%s", kubeconfigPath)

	proxyHandler := proxy.New(testEnv.Config, supportBundle, rewriter.Default())
	loggedProxyHandler := handlers.LoggingHandler(out.InfoWriter(), proxyHandler)

	s := http.Server{
		Addr:              o.proxyAddress,
		Handler:           loggedProxyHandler,
		ReadHeaderTimeout: time.Second * 5,
	}
	go func() {
		<-ctx.Done()
		out.Info("Shutting down troubleshoot-live...")
		if err := s.Shutdown(ctx); err != nil {
			out.Error(err, "failed to shutdown http server")
		}
	}()
	return ignoreServerClosedError(s.ListenAndServe())
}

func ignoreServerClosedError(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func startK8sServer(
	ctx context.Context,
	supportBundle bundle.Bundle,
	out output.Output,
	opts *serveOptions,
) (*envtest.Environment, error) {
	testEnv, err := envtest.Prepare(ctx, supportBundle, envtest.Arch(opts.envtestArch))
	if err != nil {
		return nil, fmt.Errorf("failed to prepare k8s environment: %w", err)
	}

	testEnv.ControlPlane.GetAPIServer().Out = out.V(5).InfoWriter()
	testEnv.ControlPlane.GetAPIServer().Err = out.V(5).InfoWriter()

	serviceClusterIPRange, err := resolveServiceClusterIPRange(
		opts.serviceClusterIPRange, supportBundle, out)
	if err != nil {
		return nil, err
	}
	if serviceClusterIPRange != "" {
		testEnv.ControlPlane.GetAPIServer().Configure().Append("service-cluster-ip-range", serviceClusterIPRange)
	}

	serviceNodePortRange, err := resolveServiceNodePortRange(
		opts.serviceNodePortRange, supportBundle, out)
	if err != nil {
		return nil, err
	}
	if serviceNodePortRange != "" {
		testEnv.ControlPlane.GetAPIServer().Configure().Append("service-node-port-range", serviceNodePortRange)
	}

	_, err = testEnv.Start()
	if err != nil {
		return nil, err
	}

	return testEnv, nil
}

func resolveServiceNodePortRange(
	nodePortRangeFromFlag string,
	supportBundle bundle.Bundle,
	out output.Output,
) (string, error) {
	// Manually provided via CLI flag
	if nodePortRangeFromFlag != "" {
		return nodePortRangeFromFlag, nil
	}

	// Detected from the bundle
	nodePortRangeFromBundle, err := bundle.DetectServiceNodePortRange(supportBundle)
	if err != nil {
		return "", err
	}
	if nodePortRangeFromBundle != "" {
		out.V(1).Infof("Detected service node port range: %s", nodePortRangeFromBundle)
		return nodePortRangeFromBundle, nil
	}

	// Fallback default
	out.Warnf(
		"Service node port range could not be detected from support bundle, using default %q. Use "+
			"%q flag to override this value.",
		kubernetes.DefaultServiceNodePortRange,
		"service-node-port-range",
	)
	return kubernetes.DefaultServiceNodePortRange, nil
}

func resolveServiceClusterIPRange(
	ipRangeFromFlag string,
	supportBundle bundle.Bundle,
	out output.Output,
) (string, error) {
	// Manually provided via CLI flag
	if ipRangeFromFlag != "" {
		return ipRangeFromFlag, nil
	}

	// Detected from the bundle
	ipRangeFromBundle, err := bundle.DetectServiceSubnetRange(supportBundle)
	if err != nil {
		return "", err
	}
	if ipRangeFromBundle != "" {
		out.V(1).Infof("Detected service cluster IP range: %s", ipRangeFromBundle)
		return ipRangeFromBundle, nil
	}

	// Fallback default
	out.Warnf(
		"Service ClusterIP range could not be detected from support bundle, using default %q. Use "+
			"%q flag to override this value.",
		kubernetes.DefaultServiceClusterIPRange,
		"service-cluster-ip-range",
	)
	return kubernetes.DefaultServiceClusterIPRange, nil
}
