package libs

import (
	"context"
	"github.com/google/go-github/v33/github"
	"golang.org/x/oauth2"
	"k8s.io/client-go/rest"

	//corev1 "k8s.io/api/core/v1"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"os"
)

func check(msg string, err error) {
	if err != nil {
		log.Println(msg, err)
	}
}

func GithubAuthenticate() (*github.Client, context.Context) {
	accesstoken := os.Getenv("GITHUB_TOKEN")
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: accesstoken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	return client, ctx
}

func K8sAuthenticate() *kubernetes.Clientset {
	// create k8s client
	cfg, err := clientcmd.BuildConfigFromFlags("", os.Getenv("OPENSHIFT_KUBECONFIG"))
	check("The kubeconfig could not be loaded", err)
	clientset, err := kubernetes.NewForConfig(cfg)

	return clientset
}

func DefaultK8sAuthenticate() (*rest.Config, error) {
	cfg, err := clientcmd.LoadFromFile(os.Getenv("OPENSHIFT_KUBECONFIG"))
	check("The kubeconfig could not be loaded", err)
	client := clientcmd.NewDefaultClientConfig(*cfg, &clientcmd.ConfigOverrides{})

	return client.ClientConfig()
}

//func CreateSSHSecret(clientset *kubernetes.Clientset, request *LabRequest, publickey []byte, privatekey []byte) {
//	sshkeys := corev1.Secret{
//		ObjectMeta: metav1.ObjectMeta{
//			Name: request.ID.String(),
//		},
//		Type: "Opaque",
//		Data: map[string][]byte{
//			"publicsshkey": publickey,
//			"privatesshkey": privatekey,
//		},
//	}
//
//	_, err := clientset.CoreV1().Secrets("hive").Create(context.Background(), &sshkeys, metav1.CreateOptions{})
//	check("Unable to create secret", err)