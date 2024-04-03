package kubernetes

import (
	"log"
	"os"
	"path/filepath"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// WriteProxyKubeconfig creates a KUBECONFIG file for http proxy server. If path
// for kubeconfig is not provided then default value is create in `CWD`.
func WriteProxyKubeconfig(host, path string) (string, error) {
	if path == "" {
		kubeconfigPath, _, err := getKubeconfigPathInCWD()
		if err != nil {
			return "", err
		}
		path = kubeconfigPath
	}

	if err := restConfigToKubeconfig(&rest.Config{
		Host: host,
	}, path); err != nil {
		return "", err
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	return absPath, nil
}

func restConfigToKubeconfig(rc *rest.Config, path string) error {
	clusters := map[string]*clientcmdapi.Cluster{}
	clusters["default-cluster"] = &clientcmdapi.Cluster{
		Server:                   rc.Host,
		CertificateAuthorityData: rc.TLSClientConfig.CAData,
	}

	contexts := map[string]*clientcmdapi.Context{}
	contexts["default-context"] = &clientcmdapi.Context{
		Cluster:  "default-cluster",
		AuthInfo: "default",
	}

	authinfos := map[string]*clientcmdapi.AuthInfo{}
	authinfos["default"] = &clientcmdapi.AuthInfo{
		ClientKeyData:         rc.TLSClientConfig.KeyData,
		ClientCertificateData: rc.TLSClientConfig.CertData,
	}

	clientConfig := clientcmdapi.Config{
		Kind:           "Config",
		APIVersion:     "v1",
		Clusters:       clusters,
		Contexts:       contexts,
		CurrentContext: "default-context",
		AuthInfos:      authinfos,
	}

	return clientcmd.WriteToFile(clientConfig, path)
}

func getKubeconfigPathInCWD() (string, func(), error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", nil, err
	}

	kubeconfigPath := filepath.Join(wd, "support-bundle-kubeconfig")

	cleanupFn := func() {
		if err := os.Remove(kubeconfigPath); err != nil {
			log.Println(err)
		}
	}

	return kubeconfigPath, cleanupFn, nil
}
