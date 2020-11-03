// Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package applier_test

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	baseconfig "k8s.io/component-base/config"
	"k8s.io/klog"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	gardencorev1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	cr "github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	gardenletconfig "github.com/gardener/gardener/pkg/gardenlet/apis/config"
	gardenletconfigv1alpha1 "github.com/gardener/gardener/pkg/gardenlet/apis/config/v1alpha1"
	"github.com/gardener/gardener/pkg/landscaper/gardenlet/applier"
	mockclient "github.com/gardener/gardener/pkg/mock/controller-runtime/client"
	"github.com/gardener/gardener/pkg/operation/botanist/component"
	"github.com/gardener/gardener/pkg/operation/common"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
)

var (
	expectedLabels = map[string]string{
		"app":      "gardener",
		"role":     "gardenlet",
		"chart":    "runtime-0.1.0",
		"release":  "gardenlet",
		"heritage": "Tiller",
	}
)

var _ = Describe("#Gardenlet Chart Test", func() {
	var (
		ctx          context.Context
		c            client.Client
		deployer     component.Deployer
		err          error
		chartApplier kubernetes.ChartApplier
		universalDecoder runtime.Decoder
	)

	BeforeEach(func() {
		ctx = context.TODO()

		s := runtime.NewScheme()
		// for gardenletconfig map
		Expect(corev1.AddToScheme(s)).NotTo(HaveOccurred())
		// for deployment
		Expect(appsv1.AddToScheme(s)).NotTo(HaveOccurred())
		// for unmarshal of GardenletConfiguration
		Expect(gardenletconfig.AddToScheme(s)).NotTo(HaveOccurred())
		Expect(gardenletconfigv1alpha1.AddToScheme(s)).NotTo(HaveOccurred())
		// for priority class
		Expect(schedulingv1.AddToScheme(s)).NotTo(HaveOccurred())
		// for CLusterRole and ClusterRoleBinding
		Expect(rbacv1.AddToScheme(s)).NotTo(HaveOccurred())
		// for deletion of PDB
		Expect(policyv1beta1.AddToScheme(s)).NotTo(HaveOccurred())


		// create decoder for unmarshalling the GardenletConfiguration from the component gardenletconfig Config Map
		codecs := serializer.NewCodecFactory(s)
		universalDecoder = codecs.UniversalDecoder()

		// fake client to use for the chart applier
		c = fake.NewFakeClientWithScheme(s)

		mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion, appsv1.SchemeGroupVersion})

		mapper.Add(appsv1.SchemeGroupVersion.WithKind("Deployment"), meta.RESTScopeNamespace)
		mapper.Add(corev1.SchemeGroupVersion.WithKind("ConfigMap"), meta.RESTScopeNamespace)
		mapper.Add(schedulingv1.SchemeGroupVersion.WithKind("PriorityClass"), meta.RESTScopeRoot)
		mapper.Add(rbacv1.SchemeGroupVersion.WithKind("ClusterRole"), meta.RESTScopeRoot)
		mapper.Add(rbacv1.SchemeGroupVersion.WithKind("ClusterRoleBinding"), meta.RESTScopeRoot)

		// set git version as chart helpers determine resource API versions based on that
		renderer := cr.NewWithServerVersion(&version.Info{
			GitVersion: "1.14.0",
		})

		chartApplier = kubernetes.NewChartApplier(renderer, kubernetes.NewApplier(c, mapper))
		Expect(chartApplier).NotTo(BeNil(), "should return chart applier")
	})

	Describe("Destroy Gardenlet Resources", func() {

		It("should delete all resources", func() {
			ctrl := gomock.NewController(GinkgoT())
			defer ctrl.Finish()

			mockClient := mockclient.NewMockClient(ctrl)
			mockClient.EXPECT().Delete(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "gardenlet", Namespace: gardencorev1beta1constants.GardenNamespace}})
			mockClient.EXPECT().Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "gardenlet-configmap", Namespace: gardencorev1beta1constants.GardenNamespace}})
			mockClient.EXPECT().Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "gardenlet-imagevector-overwrite", Namespace: gardencorev1beta1constants.GardenNamespace}})
			mockClient.EXPECT().Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name:  common.GardenletDefaultKubeconfigBootstrapSecretName, Namespace: gardencorev1beta1constants.GardenNamespace}})
			mockClient.EXPECT().Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: common.GardenletDefaultKubeconfigSecretName, Namespace: gardencorev1beta1constants.GardenNamespace}})
			mockClient.EXPECT().Delete(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "gardenlet", Namespace: gardencorev1beta1constants.GardenNamespace}})
			mockClient.EXPECT().Delete(ctx, &policyv1beta1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "gardenlet", Namespace: gardencorev1beta1constants.GardenNamespace}})

			vpa := &unstructured.Unstructured{}
			vpa.SetAPIVersion("autoscaling.k8s.io/v1beta2")
			vpa.SetKind("VerticalPodAutoscaler")
			vpa.SetName("gardenlet-vpa")
			vpa.SetNamespace(gardencorev1beta1constants.GardenNamespace)
			mockClient.EXPECT().Delete(ctx, vpa)

			deployer, err = applier.NewGardenletChartApplier(chartApplier, mockClient, map[string]interface{}{}, chartsRootPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(deployer.Destroy(ctx)).ToNot(HaveOccurred(), "Destroy Gardenlet resources succeeds")
		})
	})

	DescribeTable("#DeployGardenletChart",
		// configurable with settings from the Gardenlet component configuration
		func(
			imageVectorOverride *string,
			imageVectorOverrideComponents *string,
			componentConfigTlsServerContentCert *string,
			componentConfigTlsServerContentKey *string,
			gardenClientConnectionKubeconfig *string,
			seedClientConnectionKubeconfig *string,
			bootstrapKubeconfig *corev1.SecretReference,
			bootstrapKubeconfigContent *string,
			seedConfig *gardenletconfig.SeedConfig,
			) {
			gardenletValues := map[string]interface{}{
				"enabled": true,
			}

			if imageVectorOverride != nil {
				gardenletValues["imageVectorOverwrite"] = *imageVectorOverride
			}

			if imageVectorOverrideComponents != nil {
				gardenletValues["componentImageVectorOverwrites"] = *imageVectorOverrideComponents
			}

			componentConfigValues := map[string]interface{}{}

			componentConfigUsesTlsServerConfig := componentConfigTlsServerContentCert != nil && componentConfigTlsServerContentKey != nil
			if componentConfigUsesTlsServerConfig {
				componentConfigValues["server"] = map[string]interface{}{
					"https" : map[string]interface{}{
						"tls": map[string]interface{}{
							"crt": *componentConfigTlsServerContentCert,
							"key": *componentConfigTlsServerContentKey,
						},
					},
				}
			}

			if gardenClientConnectionKubeconfig != nil {
				componentConfigValues["gardenClientConnection"] = map[string]interface{}{
					"kubeconfig": *gardenClientConnectionKubeconfig,
				}
			}

			if seedClientConnectionKubeconfig != nil {
				componentConfigValues["seedClientConnection"] = map[string]interface{}{
					"kubeconfig": *seedClientConnectionKubeconfig,
				}
			}

			usesTLSBootstrapping := bootstrapKubeconfigContent != nil && bootstrapKubeconfig != nil
			if usesTLSBootstrapping {
				componentConfigValues["gardenClientConnection"] = map[string]interface{}{
					"bootstrapKubeconfig": map[string]interface{}{
						"name": bootstrapKubeconfig.Name,
						"namespace": bootstrapKubeconfig.Namespace,
						"kubeconfig": *bootstrapKubeconfigContent,
					},
				}
			}

			if seedConfig != nil {
				componentConfigValues["seedConfig"] = *seedConfig
			}

			if len(componentConfigValues) > 0 {
				gardenletValues["config"] = componentConfigValues
			}

			deployer, err = applier.NewGardenletChartApplier(chartApplier, c, map[string]interface{}{
				"global": map[string]interface{}{
					"gardenlet": gardenletValues,
				},
			}, chartsRootPath)
			Expect(err).ToNot(HaveOccurred())

			Expect(deployer.Deploy(ctx)).ToNot(HaveOccurred(), "Gardenlet chart deployment succeeds")

			validatePriorityClass(ctx, c)

			validateRBAC(ctx, c)

			validateServiceAccount(ctx, c, seedClientConnectionKubeconfig != nil)

			expectedGardenletConfig := getExpectedGardenletConfiguration(componentConfigUsesTlsServerConfig, gardenClientConnectionKubeconfig != nil, seedClientConnectionKubeconfig != nil, bootstrapKubeconfig, seedConfig)
			verifyGardenletComponentConfigConfigMap(ctx, c, universalDecoder, expectedGardenletConfig)

			expectedGardenletDeploymentSpec := getExpectedGardenletDeploymentSpec(imageVectorOverride, imageVectorOverrideComponents, componentConfigUsesTlsServerConfig, gardenClientConnectionKubeconfig, seedClientConnectionKubeconfig)
			verifyGardenletDeployment(ctx, c, expectedGardenletDeploymentSpec, imageVectorOverride != nil, imageVectorOverrideComponents != nil, componentConfigUsesTlsServerConfig, gardenClientConnectionKubeconfig != nil, seedClientConnectionKubeconfig != nil, usesTLSBootstrapping)

			if imageVectorOverride != nil {
				cm := getEmptyImageVectorOverwriteConfigMap()
				validateImageVectorOverrideConfigMap(ctx, c, cm, "images_overwrite.yaml", imageVectorOverride)
			}

			if imageVectorOverrideComponents != nil {
				cm := getEmptyImageVectorOverwriteComponentsConfigMap()
				validateImageVectorOverrideConfigMap(ctx, c, cm, "components.yaml", imageVectorOverrideComponents)
			}

			if componentConfigUsesTlsServerConfig {
				validateCertSecret(ctx, c, componentConfigTlsServerContentCert, componentConfigTlsServerContentKey)
			}

			if gardenClientConnectionKubeconfig != nil {
				secret := getEmptyKubeconfigGardenSecret()
				validateKubeconfigSecret(ctx, c, secret, gardenClientConnectionKubeconfig)
			}

			if seedClientConnectionKubeconfig != nil {
				secret := getEmptyKubeconfigSeedSecret()
				validateKubeconfigSecret(ctx, c, secret, seedClientConnectionKubeconfig)
			}

			if bootstrapKubeconfigContent != nil {
				secret := getEmptyKubeconfigGardenBootstrapSecret()
				validateKubeconfigSecret(ctx, c, secret, bootstrapKubeconfigContent)
			}
		},
			Entry("verify the default values for the Gardenlet chart & the Gardenlet component config", nil, nil ,nil, nil, nil, nil, nil, nil, nil),
			Entry("verify deployment with image vector override", pointer.StringPtr("dummy-override-content"), nil ,nil, nil, nil, nil, nil, nil, nil),
			Entry("verify deployment with image vector override components", nil, pointer.StringPtr("dummy-override-content") ,nil, nil, nil, nil, nil, nil, nil),
			Entry("verify Gardenlet with component config with TLS server configuration", nil, nil , pointer.StringPtr("dummy cert content"), pointer.StringPtr("dummy key content"), nil, nil, nil, nil, nil),
			Entry("verify Gardenlet with component config having the Garden client connection kubeconfig set", nil, nil , nil, nil, pointer.StringPtr("dummy garden kubeconfig"), nil, nil, nil, nil),
			Entry("verify Gardenlet with component config having the Seed client connection kubeconfig set", nil, nil , nil, nil, nil, pointer.StringPtr("dummy seed kubeconfig"), nil, nil, nil),
			Entry("verify Gardenlet with component config having a Bootstrap kubeconfig set", nil, nil , nil, nil, nil, nil, &corev1.SecretReference{
			Name: "gardenlet-kubeconfig-bootstrap",
			Namespace: "garden",
			}, pointer.StringPtr("dummy bootstrap kubeconfig"), nil),
			Entry("verify that the SeedConfig is set in the component config Config Map", nil, nil , nil, nil, nil, nil, nil, nil, &gardenletconfig.SeedConfig{
				Seed: gardencorev1beta1.Seed{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sweet-seed",
					},
				},
			}),

	)
})

func validateKubeconfigSecret(ctx context.Context, c client.Client, secret *corev1.Secret, kubeconfig *string) {
	expectedSecret := *secret
	expectedSecret.Labels = expectedLabels
	expectedSecret.Type = corev1.SecretTypeOpaque
	expectedSecret.Data = map[string][]byte{
		"kubeconfig": []byte(*kubeconfig),
	}

	Expect(c.Get(
		ctx,
		kutil.KeyFromObject(secret),
		secret,
	)).ToNot(HaveOccurred())
	Expect(secret.Labels).To(Equal(expectedSecret.Labels))
	Expect(secret.Data).To(Equal(expectedSecret.Data))
	Expect(secret.Type).To(Equal(expectedSecret.Type))
}

func validateCertSecret(ctx context.Context, c client.Client, cert *string, key *string) {
	secret := getEmptyCertSecret()
	expectedSecret := getEmptyCertSecret()
	expectedSecret.Labels = expectedLabels
	expectedSecret.Type = corev1.SecretTypeOpaque
	expectedSecret.Data = map[string][]byte{
		"gardenlet.crt": []byte(*cert),
		"gardenlet.key": []byte(*key),
	}

	Expect(c.Get(
		ctx,
		kutil.KeyFromObject(secret),
		secret,
	)).ToNot(HaveOccurred())
	Expect(secret.Labels).To(Equal(expectedSecret.Labels))
	Expect(secret.Data).To(Equal(expectedSecret.Data))
	Expect(secret.Type).To(Equal(expectedSecret.Type))
}


func validateImageVectorOverrideConfigMap(ctx context.Context, c client.Client, cm *corev1.ConfigMap, cmKey string, content *string) {
	expectedCm := *cm
	expectedCm.Labels = expectedLabels
	expectedCm.Data = map[string]string{
		cmKey :  fmt.Sprintf("%s\n", *content),
	}

	Expect(c.Get(
		ctx,
		kutil.KeyFromObject(cm),
		cm,
	)).ToNot(HaveOccurred())

	Expect(cm.Labels).To(Equal(expectedCm.Labels))
	Expect(cm.Data).To(Equal(expectedCm.Data))
}

func verifyGardenletDeployment(ctx context.Context, c client.Client, expectedDeploymentSpec appsv1.DeploymentSpec, hasImageVectorOverride, hasComponentImageVectorOverride, componentConfigHasTLSServerConfig, hasGardenClientConnectionKubeconfig, hasSeedClientConnectionKubeconfig, usesTLSBootstrapping bool) {
	deployment := getEmptyGardenletDeployment()
	expectedDeployment := getEmptyGardenletDeployment()
	expectedDeployment.Labels = expectedLabels

	Expect(c.Get(
		ctx,
		kutil.KeyFromObject(deployment),
		deployment,
	)).ToNot(HaveOccurred())

	Expect(deployment.ObjectMeta.Labels).To(Equal(expectedDeployment.ObjectMeta.Labels))
	Expect(deployment.Spec.Template.Annotations["checksum/configmap-gardenlet-config"]).ToNot(BeEmpty())

	if hasImageVectorOverride {
		Expect(deployment.Spec.Template.Annotations["checksum/configmap-gardenlet-imagevector-overwrite"]).ToNot(BeEmpty())
	}

	if hasComponentImageVectorOverride {
		Expect(deployment.Spec.Template.Annotations["checksum/configmap-gardenlet-imagevector-overwrite-components"]).ToNot(BeEmpty())
	}

	if componentConfigHasTLSServerConfig {
		Expect(deployment.Spec.Template.Annotations["checksum/secret-gardenlet-cert"]).ToNot(BeEmpty())
	}

	if hasGardenClientConnectionKubeconfig {
		Expect(deployment.Spec.Template.Annotations["checksum/secret-gardenlet-kubeconfig-garden"]).ToNot(BeEmpty())
	}

	if hasSeedClientConnectionKubeconfig {
		Expect(deployment.Spec.Template.Annotations["checksum/secret-gardenlet-kubeconfig-seed"]).ToNot(BeEmpty())
	}

	if usesTLSBootstrapping {
		Expect(deployment.Spec.Template.Annotations["checksum/secret-gardenlet-kubeconfig-garden-bootstrap"]).ToNot(BeEmpty())
	}

	// clean annotations with hashes
	deployment.Spec.Template.Annotations = nil
	Expect(deployment.Spec).To(Equal(expectedDeploymentSpec))
}

func verifyGardenletComponentConfigConfigMap(ctx context.Context, c client.Client, universalDecoder runtime.Decoder, expectedGardenletConfig gardenletconfig.GardenletConfiguration) {
	componentConfigCm := getEmptyGardenletConfigMap()
	expectedComponentConfigCm := getEmptyGardenletConfigMap()
	expectedComponentConfigCm.Labels = expectedLabels

	Expect(c.Get(
		ctx,
		kutil.KeyFromObject(componentConfigCm),
		componentConfigCm,
	)).ToNot(HaveOccurred())

	Expect(componentConfigCm.Labels).To(Equal(expectedComponentConfigCm.Labels))

	// unmarshal Gardenlet Configuration from deployed Config Map
	componentConfigYaml := componentConfigCm.Data["config.yaml"]
	Expect(componentConfigYaml).ToNot(HaveLen(0))
	gardenletConfig := &gardenletconfig.GardenletConfiguration{}
	_, _, err := universalDecoder.Decode([]byte(componentConfigYaml), nil, gardenletConfig)
	Expect(err).ToNot(HaveOccurred())
	Expect(*gardenletConfig).To(Equal(expectedGardenletConfig))
}

func validateServiceAccount(ctx context.Context, c client.Client, hasSeedClientConnectionKubeconfig bool) {
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gardenlet",
			Namespace: gardencorev1beta1constants.GardenNamespace,
		},
	}

	if hasSeedClientConnectionKubeconfig {
		err := c.Get(
			ctx,
			kutil.KeyFromObject(serviceAccount),
			serviceAccount,
		)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		return
	}


	expectedServiceAccount := *serviceAccount
	expectedServiceAccount.Labels = expectedLabels

	Expect(c.Get(
		ctx,
		kutil.KeyFromObject(serviceAccount),
		serviceAccount,
	)).ToNot(HaveOccurred())
	Expect(serviceAccount.Labels).To(Equal(expectedServiceAccount.Labels))
}

func validateRBAC(ctx context.Context, c client.Client) {
	// RBAC - Cluster Role
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gardener.cloud:system:gardenlet",
		},
	}
	expectedClusterRole := *clusterRole
	expectedClusterRole.Labels = map[string]string{
		gardencorev1beta1constants.GardenRole: "gardenlet",
		"app":                                 "gardener",
		"role":                                "gardenlet",
		"chart":                               "runtime-0.1.0",
		"release":                             "gardenlet",
		"heritage":                            "Tiller",
	}
	expectedClusterRole.Rules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{"*"},
			Resources: []string{"*"},
			Verbs:     []string{"*"},
		},
	}

	Expect(c.Get(
		ctx,
		kutil.KeyFromObject(clusterRole),
		clusterRole,
	)).ToNot(HaveOccurred())
	Expect(clusterRole.Labels).To(Equal(expectedClusterRole.Labels))
	Expect(clusterRole.Rules).To(Equal(expectedClusterRole.Rules))

	// RBAC - Cluster Role Binding
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gardener.cloud:system:gardenlet",
		},
	}
	expectedClusterRoleBinding := *clusterRoleBinding
	expectedClusterRoleBinding.Labels = expectedLabels
	expectedClusterRoleBinding.RoleRef = rbacv1.RoleRef{
		APIGroup: rbacv1.SchemeGroupVersion.Group,
		Kind:     "ClusterRole",
		Name:     clusterRole.Name,
	}
	expectedClusterRoleBinding.Subjects = []rbacv1.Subject{
		{
			Kind:      "ServiceAccount",
			Name:      "gardenlet",
			Namespace: gardencorev1beta1constants.GardenNamespace,
		},
	}
	Expect(c.Get(
		ctx,
		kutil.KeyFromObject(clusterRoleBinding),
		clusterRoleBinding,
	)).ToNot(HaveOccurred())
	Expect(clusterRoleBinding.Labels).To(Equal(expectedClusterRoleBinding.Labels))
	Expect(clusterRoleBinding.RoleRef).To(Equal(expectedClusterRoleBinding.RoleRef))
	Expect(clusterRoleBinding.Subjects).To(Equal(expectedClusterRoleBinding.Subjects))
}

func validatePriorityClass(ctx context.Context, c client.Client) {
	priorityClass := getEmptyPriorityClass()

	Expect(c.Get(
		ctx,
		kutil.KeyFromObject(priorityClass),
		priorityClass,
	)).ToNot(HaveOccurred())
	Expect(priorityClass.GlobalDefault).To(Equal(false))
	Expect(priorityClass.Value).To(Equal(int32(1000000000)))
}

func getExpectedGardenletDeploymentSpec(imageVectorOverride, imageVectorOverrideComponents *string, componentConfigUsesTlsServerConfig bool, gardenClientConnectionKubeconfig, seedClientConnectionKubeconfig *string) appsv1.DeploymentSpec {
	deployment :=  appsv1.DeploymentSpec{
		RevisionHistoryLimit: pointer.Int32Ptr(10),
		Replicas:             pointer.Int32Ptr(1),
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app":  "gardener",
				"role": "gardenlet",
			},
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: expectedLabels,
			},
			Spec: corev1.PodSpec{
				PriorityClassName:  "gardenlet",
				ServiceAccountName: "gardenlet",
				Containers: []corev1.Container{
					{
						Name:            "gardenlet",
						Image:           "eu.gcr.io/gardener-project/gardener/gardenlet:latest",
						ImagePullPolicy: corev1.PullIfNotPresent,
						Command: []string{
							"/gardenlet",
							"--config=/etc/gardenlet/config/config.yaml",
						},
						LivenessProbe: &corev1.Probe{
							Handler: corev1.Handler{
								HTTPGet: &corev1.HTTPGetAction{
									Path:   "/healthz",
									Port:   intstr.IntOrString{IntVal: 2720},
									Scheme: corev1.URISchemeHTTPS,
								},
							},
							InitialDelaySeconds: 15,
							TimeoutSeconds:      5,
							PeriodSeconds:       15,
							SuccessThreshold:    1,
							FailureThreshold:    3,
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("2000m"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("100Mi"),
							},
						},
						TerminationMessagePath:   "/dev/termination-log",
						TerminationMessagePolicy: corev1.TerminationMessageReadFile,
						VolumeMounts: []corev1.VolumeMount{},
					},
				},
				Volumes: []corev1.Volume{},
			},
		},
	}

	if imageVectorOverride != nil {
		deployment.Template.Spec.Containers[0].Env = append(deployment.Template.Spec.Containers[0].Env, corev1.EnvVar{
			Name:      "IMAGEVECTOR_OVERWRITE",
			Value:     "/charts_overwrite/images_overwrite.yaml",
		})
		deployment.Template.Spec.Containers[0].VolumeMounts = append(deployment.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "gardenlet-imagevector-overwrite",
			ReadOnly:  true,
			MountPath: "/charts_overwrite",
		})
		deployment.Template.Spec.Volumes = append(deployment.Template.Spec.Volumes, corev1.Volume{
			Name:         "gardenlet-imagevector-overwrite",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "gardenlet-imagevector-overwrite",
					},
				},
			},
		})
	}

	if imageVectorOverrideComponents != nil {
		deployment.Template.Spec.Containers[0].Env = append(deployment.Template.Spec.Containers[0].Env, corev1.EnvVar{
			Name:      "IMAGEVECTOR_OVERWRITE_COMPONENTS",
			Value:     "/charts_overwrite_components/components.yaml",
		})
		deployment.Template.Spec.Containers[0].VolumeMounts = append(deployment.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "gardenlet-imagevector-overwrite-components",
			ReadOnly:  true,
			MountPath: "/charts_overwrite_components",
		})
		deployment.Template.Spec.Volumes = append(deployment.Template.Spec.Volumes, corev1.Volume{
			Name:         "gardenlet-imagevector-overwrite-components",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "gardenlet-imagevector-overwrite-components",
					},
				},
			},
		})
	}

	if gardenClientConnectionKubeconfig != nil {
		deployment.Template.Spec.Containers[0].VolumeMounts = append(deployment.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "gardenlet-kubeconfig-garden",
			MountPath: "/etc/gardenlet/kubeconfig-garden",
			ReadOnly:  true,
		})
		deployment.Template.Spec.Volumes = append(deployment.Template.Spec.Volumes, corev1.Volume{
			Name:         "gardenlet-kubeconfig-garden",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "gardenlet-kubeconfig-garden",
				},
			},
		})
	}

	if seedClientConnectionKubeconfig != nil {
		deployment.Template.Spec.Containers[0].VolumeMounts = append(deployment.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "gardenlet-kubeconfig-seed",
			MountPath: "/etc/gardenlet/kubeconfig-seed",
			ReadOnly:  true,
		})
		deployment.Template.Spec.Volumes = append(deployment.Template.Spec.Volumes, corev1.Volume{
			Name:         "gardenlet-kubeconfig-seed",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "gardenlet-kubeconfig-seed",
				},
			},
		})
		deployment.Template.Spec.ServiceAccountName = ""
		deployment.Template.Spec.AutomountServiceAccountToken = pointer.BoolPtr(false)
	}

	deployment.Template.Spec.Containers[0].VolumeMounts = append(deployment.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "gardenlet-config",
			MountPath: "/etc/gardenlet/config",
		},
	)

	deployment.Template.Spec.Volumes = append(deployment.Template.Spec.Volumes, corev1.Volume{
			Name: "gardenlet-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "gardenlet-configmap",
					},
				},
			},
		},
	)

	if componentConfigUsesTlsServerConfig {
		deployment.Template.Spec.Containers[0].VolumeMounts = append(deployment.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "gardenlet-cert",
			ReadOnly:  true,
			MountPath: "/etc/gardenlet/srv",
		})
		deployment.Template.Spec.Volumes = append(deployment.Template.Spec.Volumes, corev1.Volume{
			Name:         "gardenlet-cert",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "gardenlet-cert",
				},
			},
		})
	}

	return deployment
}

const acceptContentType = "application/json"

func getExpectedGardenletConfiguration(componentConfigUsesTlsServerConfig, hasGardenClientConnectionKubeconfig, hasSeedClientConnectionKubeconfig bool, bootstrapKubeconfig *corev1.SecretReference, seedConfig *gardenletconfig.SeedConfig) gardenletconfig.GardenletConfiguration {
	var (
		zero = 0
		one = 1
		three = 3
		five = 5
		twenty = 20

		logLevelInfo = "info"
		lockObjectName = "gardenlet-leader-election"
		lockObjectNamespace = "garden"
		kubernetesLogLevel = new(klog.Level)
	)
	Expect(kubernetesLogLevel.Set("0")).ToNot(HaveOccurred())

	config :=  gardenletconfig.GardenletConfiguration{
		GardenClientConnection: &gardenletconfig.GardenClientConnection{
			ClientConnectionConfiguration: baseconfig.ClientConnectionConfiguration{
				AcceptContentTypes: acceptContentType,
				ContentType:        acceptContentType,
				QPS:                100,
				Burst:              130,
			},
		},
		SeedClientConnection: &gardenletconfig.SeedClientConnection{
			ClientConnectionConfiguration: baseconfig.ClientConnectionConfiguration{
				AcceptContentTypes: acceptContentType,
				ContentType:        acceptContentType,
				QPS:                100,
				Burst:              130,
			},
		},
		ShootClientConnection: &gardenletconfig.ShootClientConnection{
			ClientConnectionConfiguration: baseconfig.ClientConnectionConfiguration{
				AcceptContentTypes: acceptContentType,
				ContentType:        acceptContentType,
				QPS:                25,
				Burst:              50,
			},
		},
		Controllers: &gardenletconfig.GardenletControllerConfiguration{
			BackupBucket: &gardenletconfig.BackupBucketControllerConfiguration{
				ConcurrentSyncs: &twenty,
			},
			BackupEntry: &gardenletconfig.BackupEntryControllerConfiguration{
				ConcurrentSyncs:          &twenty,
				DeletionGracePeriodHours: &zero,
			},
			Seed: &gardenletconfig.SeedControllerConfiguration{
				ConcurrentSyncs: &five,
				SyncPeriod: &metav1.Duration{
					Duration: time.Minute,
				},
			},
			Shoot: &gardenletconfig.ShootControllerConfiguration{
				ReconcileInMaintenanceOnly: pointer.BoolPtr(false),
				RespectSyncPeriodOverwrite: pointer.BoolPtr(false),
				ConcurrentSyncs:            &twenty,
				SyncPeriod: &metav1.Duration{
					Duration: time.Hour,
				},
				RetryDuration: &metav1.Duration{
					Duration: 12 * time.Hour,
				},
			},
			ShootCare: &gardenletconfig.ShootCareControllerConfiguration{
				ConcurrentSyncs: &five,
				SyncPeriod: &metav1.Duration{
					Duration: 30 * time.Second,
				},
				ConditionThresholds: []gardenletconfig.ConditionThreshold{
					{
						Type: string(gardencorev1beta1.ShootAPIServerAvailable),
						Duration: &metav1.Duration{
							Duration: 1 * time.Minute,
						},
					},
					{
						Type: string(gardencorev1beta1.ShootControlPlaneHealthy),
						Duration: &metav1.Duration{
							Duration: 1 * time.Minute,
						},
					},
					{
						Type: string(gardencorev1beta1.ShootSystemComponentsHealthy),
						Duration: &metav1.Duration{
							Duration: 1 * time.Minute,
						},
					},
					{
						Type: string(gardencorev1beta1.ShootEveryNodeReady),
						Duration: &metav1.Duration{
							Duration: 5 * time.Minute,
						},
					},
				},
			},
			ShootStateSync: &gardenletconfig.ShootStateSyncControllerConfiguration{
				ConcurrentSyncs: &five,
				SyncPeriod: &metav1.Duration{
					Duration: 30 * time.Second,
				},
			},
			ControllerInstallation: &gardenletconfig.ControllerInstallationControllerConfiguration{
				ConcurrentSyncs: &twenty,
			},
			ControllerInstallationCare: &gardenletconfig.ControllerInstallationCareControllerConfiguration{
				ConcurrentSyncs: &twenty,
				SyncPeriod:      &metav1.Duration{Duration: 30 * time.Second},
			},
			ControllerInstallationRequired: &gardenletconfig.ControllerInstallationRequiredControllerConfiguration{
				ConcurrentSyncs: &one,
			},
			SeedAPIServerNetworkPolicy: &gardenletconfig.SeedAPIServerNetworkPolicyControllerConfiguration{
				ConcurrentSyncs: &three,
			},
		},
		LeaderElection: &gardenletconfig.LeaderElectionConfiguration{
			LeaderElectionConfiguration: baseconfig.LeaderElectionConfiguration{
				LeaderElect:   true,
				LeaseDuration: metav1.Duration{Duration: 15 * time.Second},
				RenewDeadline: metav1.Duration{Duration: 10 * time.Second},
				RetryPeriod:   metav1.Duration{Duration: 2 * time.Second},
				ResourceLock:  resourcelock.ConfigMapsResourceLock,
			},
			LockObjectName:      &lockObjectName,
			LockObjectNamespace: &lockObjectNamespace,
		},
		LogLevel:           &logLevelInfo,
		KubernetesLogLevel: kubernetesLogLevel,
		Server: &gardenletconfig.ServerConfiguration{HTTPS: gardenletconfig.HTTPSServer{
			Server: gardenletconfig.Server{
				BindAddress: "0.0.0.0",
				Port:        2720,
			},
		}},
	}

	if componentConfigUsesTlsServerConfig {
		config.Server.HTTPS.TLS = &gardenletconfig.TLSServer{
			ServerCertPath: "/etc/gardenlet/srv/gardenlet.crt",
			ServerKeyPath:  "/etc/gardenlet/srv/gardenlet.key",
		}
	}

	if hasGardenClientConnectionKubeconfig {
		config.GardenClientConnection.Kubeconfig = "/etc/gardenlet/kubeconfig-garden/kubeconfig"
	}

	if hasSeedClientConnectionKubeconfig {
		config.SeedClientConnection.Kubeconfig = "/etc/gardenlet/kubeconfig-seed/kubeconfig"
	}

	if bootstrapKubeconfig != nil {
		config.GardenClientConnection.BootstrapKubeconfig = bootstrapKubeconfig
	}

	if seedConfig != nil {
		config.SeedConfig = seedConfig
	}

	return config
}

func getEmptyGardenletDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gardenlet",
			Namespace: gardencorev1beta1constants.GardenNamespace,
		},
	}
}

func getEmptyGardenletConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gardenlet-configmap",
			Namespace: gardencorev1beta1constants.GardenNamespace,
		},
	}
}

func getEmptyPriorityClass() *schedulingv1.PriorityClass {
	return &schedulingv1.PriorityClass{
		ObjectMeta:       metav1.ObjectMeta{
			Name: "gardenlet",
		},
	}
}

func getEmptyImageVectorOverwriteConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gardenlet-imagevector-overwrite",
			Namespace: gardencorev1beta1constants.GardenNamespace,
		},
	}
}

func getEmptyImageVectorOverwriteComponentsConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gardenlet-imagevector-overwrite-components",
			Namespace: gardencorev1beta1constants.GardenNamespace,
		},
	}
}

func getEmptyCertSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gardenlet-cert",
			Namespace: gardencorev1beta1constants.GardenNamespace,
		},
	}
}

func getEmptyKubeconfigGardenSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gardenlet-kubeconfig-garden",
			Namespace: gardencorev1beta1constants.GardenNamespace,
		},
	}
}

func getEmptyKubeconfigSeedSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gardenlet-kubeconfig-seed",
			Namespace: gardencorev1beta1constants.GardenNamespace,
		},
	}
}

func getEmptyKubeconfigGardenBootstrapSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gardenlet-kubeconfig-bootstrap",
			Namespace: gardencorev1beta1constants.GardenNamespace,
		},
	}
}