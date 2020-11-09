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

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/version"
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
	appliercommon "github.com/gardener/gardener/pkg/landscaper/gardenlet/applier/common"
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
		ctx              context.Context
		c                client.Client
		deployer         component.Deployer
		chartApplier     kubernetes.ChartApplier
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
		// for ClusterRole and ClusterRoleBinding
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

		// set the git version required for rendering of the Gardenlet chart -  chart helpers determine resource API versions based on that
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
			mockClient.EXPECT().Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: common.GardenletDefaultKubeconfigBootstrapSecretName, Namespace: gardencorev1beta1constants.GardenNamespace}})
			mockClient.EXPECT().Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: common.GardenletDefaultKubeconfigSecretName, Namespace: gardencorev1beta1constants.GardenNamespace}})
			mockClient.EXPECT().Delete(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "gardenlet", Namespace: gardencorev1beta1constants.GardenNamespace}})
			mockClient.EXPECT().Delete(ctx, &policyv1beta1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "gardenlet", Namespace: gardencorev1beta1constants.GardenNamespace}})

			vpa := &unstructured.Unstructured{}
			vpa.SetAPIVersion("autoscaling.k8s.io/v1beta2")
			vpa.SetKind("VerticalPodAutoscaler")
			vpa.SetName("gardenlet-vpa")
			vpa.SetNamespace(gardencorev1beta1constants.GardenNamespace)
			mockClient.EXPECT().Delete(ctx, vpa)

			deployer = applier.NewGardenletChartApplier(chartApplier, mockClient, map[string]interface{}{}, chartsRootPath)
			Expect(deployer.Destroy(ctx)).ToNot(HaveOccurred(), "Destroy Gardenlet resources succeeds")
		})
	})

	DescribeTable("#DeployGardenletChart",
		func(
			imageVectorOverride *string,
			imageVectorOverrideComponents *string,
			componentConfigTlsServerContentCert *string,
			componentConfigTlsServerContentKey *string,
			gardenClientConnectionKubeconfig *string,
			seedClientConnectionKubeconfig *string,
			bootstrapKubeconfig *corev1.SecretReference,
			bootstrapKubeconfigSecret *corev1.SecretReference,
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
					"https": map[string]interface{}{
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

			// bootstrap configurations are tested in one test-case
			usesTLSBootstrapping := bootstrapKubeconfigContent != nil && bootstrapKubeconfig != nil && bootstrapKubeconfigSecret != nil
			if usesTLSBootstrapping {
				componentConfigValues["gardenClientConnection"] = map[string]interface{}{
					"bootstrapKubeconfig": map[string]interface{}{
						"name":       bootstrapKubeconfig.Name,
						"namespace":  bootstrapKubeconfig.Namespace,
						"kubeconfig": *bootstrapKubeconfigContent,
					},
					"kubeconfigSecret": map[string]interface{}{
						"name":      bootstrapKubeconfigSecret.Name,
						"namespace": bootstrapKubeconfigSecret.Namespace,
					},
				}
			}

			if seedConfig != nil {
				componentConfigValues["seedConfig"] = *seedConfig
			}

			if len(componentConfigValues) > 0 {
				gardenletValues["config"] = componentConfigValues
			}

			deployer = applier.NewGardenletChartApplier(chartApplier, c, map[string]interface{}{
				"global": map[string]interface{}{
					"gardenlet": gardenletValues,
				},
			}, chartsRootPath)

			Expect(deployer.Deploy(ctx)).ToNot(HaveOccurred(), "Gardenlet chart deployment succeeds")

			appliercommon.ValidateGardenletChartPriorityClass(ctx, c)

			appliercommon.ValidateGardenletChartRBAC(ctx, c, expectedLabels)

			appliercommon.ValidateGardenletChartServiceAccount(ctx, c, seedClientConnectionKubeconfig != nil, expectedLabels)

			expectedGardenletConfig := appliercommon.ComputeExpectedGardenletConfiguration(componentConfigUsesTlsServerConfig, gardenClientConnectionKubeconfig != nil, seedClientConnectionKubeconfig != nil, bootstrapKubeconfig, bootstrapKubeconfigSecret, seedConfig)
			appliercommon.VerifyGardenletComponentConfigConfigMap(ctx, c, universalDecoder, expectedGardenletConfig, expectedLabels)

			expectedGardenletDeploymentSpec := appliercommon.ComputeExpectedGardenletDeploymentSpec(imageVectorOverride, imageVectorOverrideComponents, componentConfigUsesTlsServerConfig, gardenClientConnectionKubeconfig, seedClientConnectionKubeconfig, expectedLabels)
			appliercommon.VerifyGardenletDeployment(ctx, c, expectedGardenletDeploymentSpec, imageVectorOverride != nil, imageVectorOverrideComponents != nil, componentConfigUsesTlsServerConfig, gardenClientConnectionKubeconfig != nil, seedClientConnectionKubeconfig != nil, usesTLSBootstrapping, expectedLabels)

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
		Entry("verify the default values for the Gardenlet chart & the Gardenlet component config", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil),
		Entry("verify deployment with image vector override", pointer.StringPtr("dummy-override-content"), nil, nil, nil, nil, nil, nil, nil, nil, nil),
		Entry("verify deployment with image vector override components", nil, pointer.StringPtr("dummy-override-content"), nil, nil, nil, nil, nil, nil, nil, nil),
		Entry("verify Gardenlet with component config with TLS server configuration", nil, nil, pointer.StringPtr("dummy cert content"), pointer.StringPtr("dummy key content"), nil, nil, nil, nil, nil, nil),
		Entry("verify Gardenlet with component config having the Garden client connection kubeconfig set", nil, nil, nil, nil, pointer.StringPtr("dummy garden kubeconfig"), nil, nil, nil, nil, nil),
		Entry("verify Gardenlet with component config having the Seed client connection kubeconfig set", nil, nil, nil, nil, nil, pointer.StringPtr("dummy seed kubeconfig"), nil, nil, nil, nil),
		Entry("verify Gardenlet with component config having a Bootstrap kubeconfig set", nil, nil, nil, nil, nil, nil, &corev1.SecretReference{
			Name:      "gardenlet-kubeconfig-bootstrap",
			Namespace: "garden",
		}, &corev1.SecretReference{
			Name:      "gardenlet-kubeconfig",
			Namespace: gardencorev1beta1constants.GardenNamespace,
		}, pointer.StringPtr("dummy bootstrap kubeconfig"), nil),
		Entry("verify that the SeedConfig is set in the component config Config Map", nil, nil, nil, nil, nil, nil, nil, nil, nil, &gardenletconfig.SeedConfig{
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
		cmKey: fmt.Sprintf("%s\n", *content),
	}

	Expect(c.Get(
		ctx,
		kutil.KeyFromObject(cm),
		cm,
	)).ToNot(HaveOccurred())

	Expect(cm.Labels).To(Equal(expectedCm.Labels))
	Expect(cm.Data).To(Equal(expectedCm.Data))
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
