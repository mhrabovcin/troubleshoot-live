package cmd

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/mesosphere/dkp-cli-runtime/core/output"
	"github.com/spf13/cobra"

	"github.com/mhrabovcin/troubleshoot-live/pkg/bundle"
	"github.com/mhrabovcin/troubleshoot-live/pkg/envtest"
	"github.com/mhrabovcin/troubleshoot-live/pkg/importer"
	"github.com/mhrabovcin/troubleshoot-live/pkg/kubernetes"
	"github.com/mhrabovcin/troubleshoot-live/pkg/proxy"
)

type serveOptions struct {
	kubeconfigPath string
	proxyAddress   string
}

func NewServeCommand(out output.Output) *cobra.Command {
	options := &serveOptions{
		proxyAddress: "localhost:8080",
	}

	cmd := &cobra.Command{
		Use:   "serve SUPPORT_BUNDLE_PATH",
		Short: "Starts a local envtest based k8s server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(args[0], options, out)
		},
	}

	cmd.Flags().StringVar(
		&options.kubeconfigPath,
		"output-kubeconfig", options.kubeconfigPath,
		"where to write kubeconfig path. Default: $(cwd)/support-bundle-kubeconfig if none provided",
	)

	cmd.Flags().StringVar(
		&options.proxyAddress, "proxy-address", options.proxyAddress,
		"value of k8s proxy server",
	)

	return cmd
}

func runServe(bundlePath string, o *serveOptions, out output.Output) error {
	supportBundle, err := bundle.New(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to get bundle from path %q: %w", bundlePath, err)
	}

	ctx := context.Background()

	out.StartOperation("Starting k8s server")
	testEnv, err := startK8sServer(ctx, supportBundle, out)
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
	err = importer.ImportBundle(ctx, supportBundle, testEnv.Config)
	out.EndOperation(err != nil)
	if err != nil {
		return fmt.Errorf("failed to import support bundle content to k8s api server: %w", err)
	}

	proxyHTTPAddress := fmt.Sprintf("http://%s", o.proxyAddress)
	kubeconfigPath, err := kubernetes.WriteProxyKubeconfig(proxyHTTPAddress, o.kubeconfigPath)
	if err != nil {
		log.Fatalln(err)
	}

	out.Infof("Running HTTPs proxy service on: %s", proxyHTTPAddress)
	out.Infof("Kubeconfig path: %s", kubeconfigPath)

	http.Handle("/", proxy.New(testEnv.Config, supportBundle))
	return http.ListenAndServe(o.proxyAddress, nil)
}

func startK8sServer(
	ctx context.Context,
	supportBundle bundle.Bundle,
	out output.Output,
) (*envtest.Environment, error) {
	bundleCRDs, err := bundle.LoadCRDs(supportBundle)
	if err != nil {
		return nil, err
	}
	if err := importer.PrepareSliceForImport(bundleCRDs); err != nil {
		return nil, err
	}
	out.Infof("Detected %d CRDs", len(bundleCRDs))

	testEnv, err := envtest.Prepare(ctx, supportBundle)
	if err != nil {
		return nil, err
	}
	testEnv.CRDs = bundleCRDs
	ipRange, err := bundle.DetectServiceSubnetRange(supportBundle)
	if err != nil {
		return nil, err
	}
	if ipRange != "" {
		out.V(1).Infof("Detected k8s service cluster IP range: %s", ipRange)
		testEnv.ControlPlane.GetAPIServer().Configure().Append("service-cluster-ip-range", ipRange)
	}
	_, err = testEnv.Start()
	if err != nil {
		return nil, err
	}

	return testEnv, nil
}
