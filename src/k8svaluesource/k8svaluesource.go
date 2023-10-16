package k8svaluesource

import (
	"context"
	"strings"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// KubernetesValueSource is a value source getting values from bitwarden
type KubernetesValueSource struct{}

type KubernetesSecretItem struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (KubernetesValueSource) getItemSplitPath(path string) (*KubernetesSecretItem, error) {
	pathParts := strings.Split(string(path), `/`)
	keyUsed := pathParts[2]
	nameUsed := pathParts[1]
	namespaceUsed := pathParts[0]

	return &KubernetesSecretItem{
		Key:       keyUsed,
		Name:      nameUsed,
		Namespace: namespaceUsed,
	}, nil
}

// GetValue returns a value from a path+key in bitwarden or null if it doesn't exist
func (m KubernetesValueSource) GetValue(path []byte, key []byte) (*[]byte, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	ctx := context.Background()

	item, _ := m.getItemSplitPath(string(path))

	secret, err := clientset.CoreV1().Secrets(item.Namespace).Get(ctx, item.Name, v1.GetOptions{})

	if err != nil {
		panic(err.Error())
	}

	value := secret.Data[item.Key]

	return &value, nil
}
