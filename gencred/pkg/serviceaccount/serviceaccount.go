/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package serviceaccount

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api/v1"
	"k8s.io/test-infra/gencred/pkg/kubeconfig"
)

const (
	// clusterRoleBindingName is the name for the cluster administrator ClusterRoleBinding.
	clusterRoleBindingName = "serviceaccount-cluster-admin-crb"
	// serviceAccountName is the name for the cluster administrator ServiceAccount.
	serviceAccountName = "serviceaccount-cluster-admin"
)

// getOrCreateSA gets existing or creates new service account (SA).
func getOrCreateSA(clientset kubernetes.Interface) ([]byte, []byte, error) {
	client := clientset.CoreV1().ServiceAccounts(corev1.NamespaceDefault)

	// Get ServiceAccount if exists.
	if saObj, err := client.Get(serviceAccountName, metav1.GetOptions{}); err == nil {
		return getSASecrets(clientset, saObj)
	}

	// Generate a Kubernetes ServiceAccount object.
	saObj := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceAccountName,
		},
	}

	// Create ServiceAccount.
	_, err := client.Create(saObj)
	if err != nil {
		return nil, nil, fmt.Errorf("create SA: %v", err)
	}

	// Get/Create ClusterRoleBinding.
	err = getOrCreateCRB(clientset)
	if err != nil {
		return nil, nil, fmt.Errorf("get or create CRB: %v", err)
	}

	// Get ServiceAccount.
	saObj, err = client.Get(serviceAccountName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("get SA: %v", err)
	}

	return getSASecrets(clientset, saObj)
}

// getOrCreateCRB gets existing or creates new cluster role binding (CRB).
func getOrCreateCRB(clientset kubernetes.Interface) error {
	client := clientset.RbacV1().ClusterRoleBindings()

	// Get ClusterRoleBinding if exists.
	if _, err := client.Get(clusterRoleBindingName, metav1.GetOptions{}); err == nil {
		return nil
	}

	// Generate a Kubernetes ClusterRoleBinding object.
	crbObj := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: clusterRoleBindingName},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: serviceAccountName, Namespace: corev1.NamespaceDefault}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "cluster-admin"},
	}

	// Create ClusterRoleBinding.
	_, err := client.Create(crbObj)
	if err != nil {
		return fmt.Errorf("create CRB: %v", err)
	}

	return nil
}

// getSASecrets gets service account token and root CA secrets.
func getSASecrets(clientset kubernetes.Interface, saObj *corev1.ServiceAccount) ([]byte, []byte, error) {
	client := clientset.CoreV1().Secrets(corev1.NamespaceDefault)

	if len(saObj.Secrets) == 0 {
		return nil, nil, errors.New("locate secrets")
	}

	// Get Secret.
	secretObj, err := client.Get(saObj.Secrets[0].Name, metav1.GetOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("get secret: %v", err)
	}

	token, ok := secretObj.Data[corev1.ServiceAccountTokenKey]
	if !ok {
		return nil, nil, errors.New("locate token")
	}

	caPEM, ok := secretObj.Data[corev1.ServiceAccountRootCAKey]
	if !ok {
		return nil, nil, errors.New("locate root CA")
	}

	return token, caPEM, nil
}

// CreateClusterServiceAccountCredentials creates a service account to authenticate to a cluster API server.
func CreateClusterServiceAccountCredentials(clientset kubernetes.Interface) (token []byte, caPEM []byte, err error) {
	token, caPEM, err = getOrCreateSA(clientset)
	if err != nil {
		return nil, nil, fmt.Errorf("get or create SA: %v", err)
	}

	return token, caPEM, nil
}

// CreateKubeConfigWithServiceAccountCredentials creates a kube config containing a service account token to authenticate to a Kubernetes cluster API server.
func CreateKubeConfigWithServiceAccountCredentials(clientset kubernetes.Interface, name string) ([]byte, error) {
	token, caPEM, err := CreateClusterServiceAccountCredentials(clientset)
	if err != nil {
		return nil, err
	}

	authInfo := clientcmdapi.AuthInfo{
		Token: string(token),
	}

	return kubeconfig.CreateKubeConfig(clientset, name, caPEM, authInfo)
}
