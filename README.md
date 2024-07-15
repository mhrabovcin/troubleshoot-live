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

```bash
export VERSION=v0.0.10
export OS=linux
export ARCH=amd64
curl -LS "https://github.com/mhrabovcin/troubleshoot-live/releases/download/${VERSION}/troubleshoot-live_${VERSION}_${OS}_${ARCH}.tar.gz" | tar -zxvf -
chmod +x troubleshoot-live
```

[Optional] Then copy the `troubleshoot-live` binary to a directory in your `PATH`.

```bash
sudo cp ./troubleshoot-live /usr/local/bin/troubleshoot-live
```

- Use the `asdf` [plugin](https://github.com/adyatlov/asdf-troubleshoot-live).

## Usage

You can spin up a new API server and import resources from the support bundle using:

```bash
troubleshoot-live serve support-bundle.tar.gz
```

where:

- `support-bundle.tar.gz` is the support bundle file
- `/path/to/bundle` is the path to the extracted support bundle

The output of the command should look like:

```bash
 ✓ Starting k8s server
Processing 450 records from CRD file
 ✓ Importing bundle resources
Running HTTPs proxy service on: http://localhost:8080
Kubeconfig path: ./support-bundle-kubeconfig
```

You can then use the `support-bundle-kubeconfig` file to access the API server like always with `kubectl`:

```bash
$ kubectl --kubeconfig support-bundle-kubeconfig get pods
NAMESPACE     NAME                                       READY   STATUS    RESTARTS   AGE
default   my-pod-66bff467f8-2j2xv                   1/1     Running   0          2m
```

## Known issues

### `etcd` process is crashing

The OS level resource limits (like `ulimit`) might need to be increased to allow the API server and `etcd` to start. The `etcd` process is using a lot of file descriptors and might hit the open files limit. The `etcd` process logs can be printed by running `troubleshoot-live` with `KUBEBUILDER_ATTACH_CONTROL_PLANE_OUTPUT=true` environment variable.
