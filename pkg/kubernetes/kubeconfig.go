package kubernetes

import (
	"log"
	"os"
	"path/filepath"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func WriteProxyKubeconfig(host string, path string) (string, error) {
	if path == "" {
		kubeconfigPath, _, err := GetKubeconfigPathInCWD()
		if err != nil {
			return "", err
		}
		path = kubeconfigPath
	}

	if err := RestConfigToKubeconfig(&rest.Config{
		Host: "http://localhost:8080",
	}, path); err != nil {
		return "", err
	}

	return path, nil
}

func RestConfigToKubeconfig(rc *rest.Config, path string) error {
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

func GetKubeconfigPathInCWD() (string, func(), error) {
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
