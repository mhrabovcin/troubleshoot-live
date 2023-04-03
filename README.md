# troubleshoot-live

This tool exposes K8s API resources from support bundle collected with [`troubleshoot.sh`](https://troubleshoot.sh) via locally launched API server.

The tools is heavily inspired by existing [`sbctl`](https://github.com/replicatedhq/sbctl) that tries to mock the whole Kubernetes API server. The `troubleshoot-live` is using [`envtest`](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest) package from `controller-runtime` and instead of mocking API server it launches actual K8s API server with `etcd`. It exports API resources from the support bundle to a running Kubernetes API server. The testing server is running without webhooks so there is no validation of resources by controllers that normally check on resources.

The flow is the following:

- First the version of the Kubernetes API server from which was the support bundle collected is detected. The [`cluster-info`](https://troubleshoot.sh/docs/collect/cluster-info/) collector stores this information in the bundle.
- CRDs are loaded from bundle.
- Kubernetes API server and ETCD for detected version are downloaded using the `envtest`.
- Kubernetes API server is started.
- Resources from the bundle are imported to the API server.
- A new proxy HTTP server is launched that will expose Kubernetes API server (default on `localhost:8080`)

The proxy server allows to define on which address is the API server available. It also enables providing some custom functionality that wouldn't be possible with launched API server:

- The `creationTimestamp` is not preserved when imported from the bundle files. The proxy handler mutates API server responses and replaces `creationTimestamp` with data from the bundle.
- A custom handler for serving logs data from the support bundle. This allows to use `kubectl` and other tools to retrieve logs for pods.

## Installation

- Download and unpack a release from the Release page.
- Use the `asdf` [plugin](https://github.com/adyatlov/asdf-troubleshoot-live).
