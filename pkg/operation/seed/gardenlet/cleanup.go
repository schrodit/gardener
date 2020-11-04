package gardenlet

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/operation/common"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
)

// DeleteGardenlet deletes a previously deployed Gardenlet and all of its resources
func DeleteGardenlet(ctx context.Context, c client.Client) error {
	vpa := &unstructured.Unstructured{}
	vpa.SetAPIVersion("autoscaling.k8s.io/v1beta2")
	vpa.SetKind("VerticalPodAutoscaler")
	vpa.SetName("gardenlet-vpa")
	vpa.SetNamespace(v1beta1constants.GardenNamespace)

	return kutil.DeleteObjects(
		ctx,
		c,
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "gardenlet", Namespace: v1beta1constants.GardenNamespace}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "gardenlet-configmap", Namespace: v1beta1constants.GardenNamespace}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "gardenlet-imagevector-overwrite", Namespace: v1beta1constants.GardenNamespace}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: common.GardenletDefaultKubeconfigBootstrapSecretName, Namespace: v1beta1constants.GardenNamespace}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: common.GardenletDefaultKubeconfigSecretName, Namespace: v1beta1constants.GardenNamespace}},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "gardenlet", Namespace: v1beta1constants.GardenNamespace}},
		&policyv1beta1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "gardenlet", Namespace: v1beta1constants.GardenNamespace}},
		vpa,
	)
}
