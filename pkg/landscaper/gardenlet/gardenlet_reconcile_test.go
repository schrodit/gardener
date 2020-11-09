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

package gardenlet

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/rest"
	bootstraptokenapi "k8s.io/cluster-bootstrap/token/api"
	bootstraptokenutil "k8s.io/cluster-bootstrap/token/util"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	cr "github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	gardenletconfig "github.com/gardener/gardener/pkg/gardenlet/apis/config"
	gardenletconfigv1alpha1 "github.com/gardener/gardener/pkg/gardenlet/apis/config/v1alpha1"
	"github.com/gardener/gardener/pkg/landscaper/gardenlet/apis/imports"
	importsv1alpha1 "github.com/gardener/gardener/pkg/landscaper/gardenlet/apis/imports/v1alpha1"
	appliercommon "github.com/gardener/gardener/pkg/landscaper/gardenlet/applier/common"
	"github.com/gardener/gardener/pkg/logger"
	mockclient "github.com/gardener/gardener/pkg/mock/controller-runtime/client"
	mock "github.com/gardener/gardener/pkg/mock/gardener/client/kubernetes"
	"github.com/gardener/gardener/pkg/utils"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/retry"
	retryfake "github.com/gardener/gardener/pkg/utils/retry/fake"
	"github.com/gardener/gardener/pkg/utils/test"
)

const (
	chartsRootPath = "../../../charts"
)

var _ = Describe("Gardenlet Landscaper reconciliation testing", func() {
	var (
		landscaper Landscaper
		seed       = &gardencorev1beta1.Seed{ObjectMeta: metav1.ObjectMeta{
			Name: "sweet-seed",
		}}

		mockController      *gomock.Controller
		mockGardenClient    *mockclient.MockClient
		mockSeedClient      *mockclient.MockClient
		mockGardenInterface *mock.MockInterface
		mockSeedInterface   *mock.MockInterface

		ctx         = context.TODO()
		cleanupFunc func()
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())

		mockGardenClient = mockclient.NewMockClient(mockController)
		mockGardenInterface = mock.NewMockInterface(mockController)

		mockSeedClient = mockclient.NewMockClient(mockController)
		mockSeedInterface = mock.NewMockInterface(mockController)

		landscaper = Landscaper{
			gardenClient: mockGardenInterface,
			seedClient:   mockSeedInterface,
			log:          logger.NewNopLogger().WithContext(ctx),
			Imports: &imports.LandscaperGardenletImport{
				ComponentConfiguration: gardenletconfigv1alpha1.GardenletConfiguration{
					SeedConfig: &gardenletconfigv1alpha1.SeedConfig{
						Seed: *seed,
					},
				},
			},
		}

		waiter := &retryfake.Ops{MaxAttempts: 1}
		cleanupFunc = test.WithVars(
			&retry.UntilTimeout, waiter.UntilTimeout,
		)
	})

	AfterEach(func() {
		mockController.Finish()
		cleanupFunc()
	})

	Describe("#Reconcile", func() {
		var (
			rbacChartApplier kubernetes.ChartApplier
			fakeRBACClient   client.Client

			vpaCRDChartApplier kubernetes.ChartApplier
			fakeVpaCRDClient   client.Client

			gardenletChartApplier          kubernetes.ChartApplier
			fakeGardenletChartClient       client.Client
			gardenletChartUniversalDecoder runtime.Decoder

			seedSecretName      = "seed-secret"
			seedSecretNamespace = "garden"
			seedSecret          = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      seedSecretName,
					Namespace: seedSecretNamespace,
				},
			}
			seedSecretRef = &corev1.SecretReference{
				Name:      seedSecretName,
				Namespace: seedSecretNamespace,
			}

			kubeconfigContentRuntimeCluster = "very secure"

			backupSecretName      = "backup-secret"
			backupSecretNamespace = "garden"
			backupSecret          = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      backupSecretName,
					Namespace: backupSecretNamespace,
				},
			}
			backupSecretRef = corev1.SecretReference{
				Name:      backupSecretName,
				Namespace: backupSecretNamespace,
			}

			backupProviderName = "abc"
			backupCredentials  = map[string][]byte{
				"KEY": []byte("value"),
			}

			restConfig = &rest.Config{
				Host: "apiserver.dummy",
			}
			tokenID                  = utils.ComputeSHA256Hex([]byte(seed.Name))[:6]
			bootstrapTokenSecretName = bootstraptokenutil.BootstrapTokenSecretName(tokenID)
			timestampInTheFuture     = time.Now().UTC().Add(15 * time.Hour).Format(time.RFC3339)
		)

		// Before each ensures the chart appliers to have fake clients (cannot use mocks when applying charts)
		BeforeEach(func() {
			renderer := cr.NewWithServerVersion(&version.Info{})

			rbacScheme := runtime.NewScheme()
			vpaScheme := runtime.NewScheme()
			gardenletChartScheme := runtime.NewScheme()

			Expect(rbacv1.AddToScheme(rbacScheme)).NotTo(HaveOccurred())
			Expect(apiextensionsv1beta1.AddToScheme(vpaScheme)).NotTo(HaveOccurred())
			Expect(corev1.AddToScheme(gardenletChartScheme)).NotTo(HaveOccurred())
			Expect(appsv1.AddToScheme(gardenletChartScheme)).NotTo(HaveOccurred())
			Expect(gardenletconfig.AddToScheme(gardenletChartScheme)).NotTo(HaveOccurred())
			Expect(gardenletconfigv1alpha1.AddToScheme(gardenletChartScheme)).NotTo(HaveOccurred())
			Expect(schedulingv1.AddToScheme(gardenletChartScheme)).NotTo(HaveOccurred())
			Expect(rbacv1.AddToScheme(gardenletChartScheme)).NotTo(HaveOccurred())
			Expect(policyv1beta1.AddToScheme(gardenletChartScheme)).NotTo(HaveOccurred())

			codecs := serializer.NewCodecFactory(gardenletChartScheme)
			gardenletChartUniversalDecoder = codecs.UniversalDecoder()

			fakeRBACClient = fake.NewFakeClientWithScheme(rbacScheme)
			fakeVpaCRDClient = fake.NewFakeClientWithScheme(vpaScheme)
			fakeGardenletChartClient = fake.NewFakeClientWithScheme(gardenletChartScheme)

			rbacMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{rbacv1.SchemeGroupVersion})
			rbacMapper.Add(rbacv1.SchemeGroupVersion.WithKind("ClusterRole"), meta.RESTScopeRoot)
			rbacMapper.Add(rbacv1.SchemeGroupVersion.WithKind("ClusterRoleBinding"), meta.RESTScopeRoot)

			vpaMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{apiextensionsv1beta1.SchemeGroupVersion})
			vpaMapper.Add(apiextensionsv1beta1.SchemeGroupVersion.WithKind("CustomResourceDefinition"), meta.RESTScopeRoot)

			gardenletMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion, appsv1.SchemeGroupVersion})
			gardenletMapper.Add(appsv1.SchemeGroupVersion.WithKind("Deployment"), meta.RESTScopeNamespace)
			gardenletMapper.Add(corev1.SchemeGroupVersion.WithKind("ConfigMap"), meta.RESTScopeNamespace)
			gardenletMapper.Add(schedulingv1.SchemeGroupVersion.WithKind("PriorityClass"), meta.RESTScopeRoot)
			gardenletMapper.Add(rbacv1.SchemeGroupVersion.WithKind("ClusterRole"), meta.RESTScopeRoot)
			gardenletMapper.Add(rbacv1.SchemeGroupVersion.WithKind("ClusterRoleBinding"), meta.RESTScopeRoot)

			// set git version as the chart helpers in the gardenlet determine resource API versions based on that
			gardenletChartRenderer := cr.NewWithServerVersion(&version.Info{
				GitVersion: "1.14.0",
			})

			rbacChartApplier = kubernetes.NewChartApplier(renderer, kubernetes.NewApplier(fakeRBACClient, rbacMapper))
			vpaCRDChartApplier = kubernetes.NewChartApplier(renderer, kubernetes.NewApplier(fakeVpaCRDClient, vpaMapper))
			gardenletChartApplier = kubernetes.NewChartApplier(gardenletChartRenderer, kubernetes.NewApplier(fakeGardenletChartClient, gardenletMapper))
		})

		// only test the happy case here, as there are more fine grained tests for each individual function
		It("should successfully reconcile", func() {
			// deploy seed secret
			landscaper.Imports.ComponentConfiguration.SeedConfig.Spec = gardencorev1beta1.SeedSpec{SecretRef: seedSecretRef}
			landscaper.Imports.RuntimeCluster = imports.Target{
				Spec: imports.TargetSpec{
					Configuration: imports.KubernetesClusterTargetConfig{
						Kubeconfig: kubeconfigContentRuntimeCluster,
					},
				},
			}

			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenClient.EXPECT().Get(ctx, kutil.KeyFromObject(seedSecret), seedSecret).Return(nil)

			expectedSecret := *seedSecret
			expectedSecret.Data = map[string][]byte{
				"kubeconfig": []byte(kubeconfigContentRuntimeCluster),
			}
			expectedSecret.Type = corev1.SecretTypeOpaque
			mockGardenClient.EXPECT().Update(ctx, &expectedSecret).Return(nil)

			// deployBackupSecret
			rawBackupCredentials, err := json.Marshal(backupCredentials)
			Expect(err).ToNot(HaveOccurred())
			landscaper.Imports.SeedBackup = &imports.SeedBackup{
				Provider: backupProviderName,
				Credentials: &runtime.RawExtension{
					Raw: rawBackupCredentials,
				},
			}
			landscaper.Imports.ComponentConfiguration.SeedConfig.Spec.Backup = &gardencorev1beta1.SeedBackup{
				Provider:  backupProviderName,
				SecretRef: backupSecretRef,
			}

			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenClient.EXPECT().Get(ctx, kutil.KeyFromObject(backupSecret), backupSecret).Return(apierrors.NewNotFound(schema.GroupResource{}, backupSecret.Name))

			expectedBackupSecret := *backupSecret
			expectedBackupSecret.Data = backupCredentials
			expectedBackupSecret.Type = corev1.SecretTypeOpaque
			expectedBackupSecret.Labels = map[string]string{
				"provider": backupProviderName,
			}
			mockGardenClient.EXPECT().Create(ctx, &expectedBackupSecret).Return(nil)

			// isSeedBootstrapped
			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenClient.EXPECT().Get(ctx, kutil.KeyFromObject(seed), seed).DoAndReturn(func(_ context.Context, _ client.ObjectKey, s *gardencorev1beta1.Seed) error {
				s.ObjectMeta = seed.ObjectMeta
				s.Status.KubernetesVersion = nil
				return nil
			})

			// getKubeconfigWithBootstrapToken - re-use existing bootstrap token
			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenInterface.EXPECT().RESTConfig().Return(restConfig)
			mockGardenClient.EXPECT().Get(ctx, kutil.Key(metav1.NamespaceSystem, bootstrapTokenSecretName), gomock.AssignableToTypeOf(&corev1.Secret{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, s *corev1.Secret) error {
				s.Data = map[string][]byte{
					bootstraptokenapi.BootstrapTokenExpirationKey: []byte(timestampInTheFuture),
					bootstraptokenapi.BootstrapTokenIDKey:         []byte("dummy"),
					bootstraptokenapi.BootstrapTokenSecretKey:     []byte(bootstrapTokenSecretName),
				}
				return nil
			})

			// RBAC applier
			mockGardenInterface.EXPECT().ChartApplier().Return(rbacChartApplier)

			defer test.WithVars(
				&GetChartPath, func() string { return chartsRootPath },
			)()

			// VPA CRD
			landscaper.Imports.ComponentConfiguration.SeedConfig.Spec.Settings = &gardencorev1beta1.SeedSettings{
				VerticalPodAutoscaler: &gardencorev1beta1.SeedSettingVerticalPodAutoscaler{
					Enabled: true,
				},
			}
			mockSeedInterface.EXPECT().ChartApplier().Return(vpaCRDChartApplier)

			// Gardenlet chart for the Seed cluster
			mockSeedInterface.EXPECT().Client().Return(mockSeedClient)
			mockSeedClient.EXPECT().Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "garden",
				},
			})

			// required configuration that is validated when parsing the Gardenlet landscaper configuration
			// validated via the Seed validation
			// this test uses the default configuration from the helm chart
			landscaper.Imports.ComponentConfiguration.Server = &gardenletconfigv1alpha1.ServerConfiguration{
				HTTPS: gardenletconfigv1alpha1.HTTPSServer{
					Server: gardenletconfigv1alpha1.Server{
						BindAddress: "0.0.0.0",
						Port:        2720,
					},
				},
			}
			// set defaults for the imports to be able to calculate the values for the Gardenlet chart
			v1alphaImport := &importsv1alpha1.LandscaperGardenletImport{}
			importsv1alpha1.Convert_imports_LandscaperGardenletImport_To_v1alpha1_LandscaperGardenletImport(landscaper.Imports, v1alphaImport, nil)
			importsv1alpha1.SetDefaults_LandscaperGardenletImport(v1alphaImport)
			importsv1alpha1.Convert_v1alpha1_LandscaperGardenletImport_To_imports_LandscaperGardenletImport(v1alphaImport, landscaper.Imports, nil)

			mockSeedInterface.EXPECT().ChartApplier().Return(gardenletChartApplier)
			mockSeedInterface.EXPECT().Client().Return(mockSeedClient)

			// the repository and tag are required values in the gardenlet chart
			// use default values from the gardenlet helm chart
			landscaper.gardenletImageRepository = "eu.gcr.io/gardener-project/gardener/gardenlet"
			landscaper.gardenletImageTag = "latest"

			// waitForRolloutToBeComplete
			defer test.WithVars(
				&Sleep, func() time.Duration { return 0 * time.Second },
			)()

			mockSeedInterface.EXPECT().Client().Return(mockSeedClient)
			mockSeedClient.EXPECT().Get(ctx, kutil.Key(v1beta1constants.GardenNamespace, "gardenlet"), gomock.AssignableToTypeOf(&appsv1.Deployment{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, d *appsv1.Deployment) error {
				d.Generation = 1
				d.Status.ObservedGeneration = 1
				d.Status.Conditions = []appsv1.DeploymentCondition{
					{
						Type:   appsv1.DeploymentAvailable,
						Status: corev1.ConditionTrue,
					},
					{
						Type:   appsv1.DeploymentReplicaFailure,
						Status: corev1.ConditionFalse,
					},
				}
				return nil
			})

			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)

			mockGardenClient.EXPECT().Get(ctx, kutil.KeyFromObject(seed), seed).DoAndReturn(func(_ context.Context, _ client.ObjectKey, s *gardencorev1beta1.Seed) error {
				s.ObjectMeta = seed.ObjectMeta
				s.Generation = 1
				s.Status.ObservedGeneration = 1
				s.Status.Conditions = []gardencorev1beta1.Condition{
					{
						Type:   gardencorev1beta1.SeedGardenletReady,
						Status: gardencorev1beta1.ConditionTrue,
					},
					{
						Type:   gardencorev1beta1.SeedBootstrapped,
						Status: gardencorev1beta1.ConditionTrue,
					},
				}
				return nil
			})

			// start reconciliation
			err = landscaper.Reconcile(ctx)
			Expect(err).ToNot(HaveOccurred())

			// check RBAC chart has been applied to the Garden cluster
			appliercommon.ValidateGardenletRBACChartResources(ctx, fakeRBACClient)

			// check VPA CRD chart has been applied to the Seed cluster
			definition := apiextensionsv1beta1.CustomResourceDefinition{}
			Expect(fakeVpaCRDClient.Get(
				ctx,
				client.ObjectKey{Name: "verticalpodautoscalers.autoscaling.k8s.io"},
				&definition,
			)).ToNot(HaveOccurred())

			// verify resources applied via the Gardenlet chart
			expectedLabels := map[string]string{
				"app":      "gardener",
				"role":     "gardenlet",
				"chart":    "runtime-0.1.0",
				"release":  "gardenlet",
				"heritage": "Tiller",
			}

			appliercommon.ValidateGardenletChartPriorityClass(ctx, fakeGardenletChartClient)

			appliercommon.ValidateGardenletChartRBAC(ctx, fakeGardenletChartClient, expectedLabels)

			appliercommon.ValidateGardenletChartServiceAccount(ctx, fakeGardenletChartClient, false, expectedLabels)

			// convert SeedConfig before validation
			seedConfig := &gardenletconfig.SeedConfig{}
			Expect(gardenletconfigv1alpha1.Convert_v1alpha1_SeedConfig_To_config_SeedConfig(landscaper.Imports.ComponentConfiguration.SeedConfig, seedConfig, nil)).ToNot(HaveOccurred())

			// can be overwritten in the componentConfig.SeedConfig.GardenClientConnection.BootstrapKubeconfig
			// this test uses the default values from the helm chart
			bootstrapSecretRef := &corev1.SecretReference{
				Name:      "gardenlet-kubeconfig-bootstrap",
				Namespace: "garden",
			}

			// can be overwritten in the componentConfig.SeedConfig.GardenClientConnection.KubeconfigSecret
			// this test uses the default values from the helm chart
			bootstrapKubeconfigSecretRef := &corev1.SecretReference{
				Name:      "gardenlet-kubeconfig",
				Namespace: "garden",
			}

			expectedGardenletConfig := appliercommon.ComputeExpectedGardenletConfiguration(false, false, false, bootstrapSecretRef, bootstrapKubeconfigSecretRef, seedConfig)
			appliercommon.VerifyGardenletComponentConfigConfigMap(ctx, fakeGardenletChartClient, gardenletChartUniversalDecoder, expectedGardenletConfig, expectedLabels)

			expectedGardenletDeploymentSpec := appliercommon.ComputeExpectedGardenletDeploymentSpec(nil, nil, false, nil, nil, expectedLabels)
			appliercommon.VerifyGardenletDeployment(ctx, fakeGardenletChartClient, expectedGardenletDeploymentSpec, false, false, false, false, false, true, expectedLabels)
		})
	})
	Describe("#waitForRolloutToBeComplete", func() {
		It("should fail because the Gardenlet deployment is not healthy (outdated Generation)", func() {
			defer test.WithVars(
				&Sleep, func() time.Duration { return 0 * time.Second },
			)()

			mockSeedInterface.EXPECT().Client().Return(mockSeedClient)
			mockSeedClient.EXPECT().Get(ctx, kutil.Key(v1beta1constants.GardenNamespace, "gardenlet"), gomock.AssignableToTypeOf(&appsv1.Deployment{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, d *appsv1.Deployment) error {
				d.Generation = 2
				d.Status.ObservedGeneration = 1
				return nil
			})

			err := landscaper.waitForRolloutToBeComplete(ctx)
			Expect(err).To(HaveOccurred())
		})

		It("should fail because the Seed is not yet registered", func() {
			defer test.WithVars(
				&Sleep, func() time.Duration { return 0 * time.Second },
			)()

			mockSeedInterface.EXPECT().Client().Return(mockSeedClient)
			mockSeedClient.EXPECT().Get(ctx, kutil.Key(v1beta1constants.GardenNamespace, "gardenlet"), gomock.AssignableToTypeOf(&appsv1.Deployment{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, d *appsv1.Deployment) error {
				d.Generation = 1
				d.Status.ObservedGeneration = 1
				d.Status.Conditions = []appsv1.DeploymentCondition{
					{
						Type:   appsv1.DeploymentAvailable,
						Status: corev1.ConditionTrue,
					},
					{
						Type:   appsv1.DeploymentReplicaFailure,
						Status: corev1.ConditionFalse,
					},
				}
				return nil
			})

			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenClient.EXPECT().Get(ctx, kutil.KeyFromObject(seed), seed).Return(apierrors.NewNotFound(schema.GroupResource{}, seed.Name))

			err := landscaper.waitForRolloutToBeComplete(ctx)
			Expect(err).To(HaveOccurred())
		})

		It("should fail because failed to get the Seed resource", func() {
			defer test.WithVars(
				&Sleep, func() time.Duration { return 0 * time.Second },
			)()

			mockSeedInterface.EXPECT().Client().Return(mockSeedClient)
			mockSeedClient.EXPECT().Get(ctx, kutil.Key(v1beta1constants.GardenNamespace, "gardenlet"), gomock.AssignableToTypeOf(&appsv1.Deployment{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, d *appsv1.Deployment) error {
				d.Generation = 1
				d.Status.ObservedGeneration = 1
				d.Status.Conditions = []appsv1.DeploymentCondition{
					{
						Type:   appsv1.DeploymentAvailable,
						Status: corev1.ConditionTrue,
					},
					{
						Type:   appsv1.DeploymentReplicaFailure,
						Status: corev1.ConditionFalse,
					},
				}
				return nil
			})

			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenClient.EXPECT().Get(ctx, kutil.KeyFromObject(seed), seed).Return(fmt.Errorf("some error"))

			err := landscaper.waitForRolloutToBeComplete(ctx)
			Expect(err).To(HaveOccurred())
		})

		It("should fail because the Seed is unhealthy (not bootstrapped yet)", func() {
			defer test.WithVars(
				&Sleep, func() time.Duration { return 0 * time.Second },
			)()

			mockSeedInterface.EXPECT().Client().Return(mockSeedClient)
			mockSeedClient.EXPECT().Get(ctx, kutil.Key(v1beta1constants.GardenNamespace, "gardenlet"), gomock.AssignableToTypeOf(&appsv1.Deployment{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, d *appsv1.Deployment) error {
				d.Generation = 1
				d.Status.ObservedGeneration = 1
				d.Status.Conditions = []appsv1.DeploymentCondition{
					{
						Type:   appsv1.DeploymentAvailable,
						Status: corev1.ConditionTrue,
					},
					{
						Type:   appsv1.DeploymentReplicaFailure,
						Status: corev1.ConditionFalse,
					},
				}
				return nil
			})

			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)

			mockGardenClient.EXPECT().Get(ctx, kutil.KeyFromObject(seed), seed).DoAndReturn(func(_ context.Context, _ client.ObjectKey, s *gardencorev1beta1.Seed) error {
				s.ObjectMeta = seed.ObjectMeta
				s.Generation = 1
				s.Status.ObservedGeneration = 1
				s.Status.Conditions = []gardencorev1beta1.Condition{
					{
						Type:   gardencorev1beta1.SeedGardenletReady,
						Status: gardencorev1beta1.ConditionTrue,
					},
					{
						Type:   gardencorev1beta1.SeedBootstrapped,
						Status: gardencorev1beta1.ConditionFalse,
					},
				}
				return nil
			})

			err := landscaper.waitForRolloutToBeComplete(ctx)
			Expect(err).To(HaveOccurred())
		})

		It("should succeed  because the Seed is registered, bootstrapped and ready", func() {
			defer test.WithVars(
				&Sleep, func() time.Duration { return 0 * time.Second },
			)()

			mockSeedInterface.EXPECT().Client().Return(mockSeedClient)
			mockSeedClient.EXPECT().Get(ctx, kutil.Key(v1beta1constants.GardenNamespace, "gardenlet"), gomock.AssignableToTypeOf(&appsv1.Deployment{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, d *appsv1.Deployment) error {
				d.Generation = 1
				d.Status.ObservedGeneration = 1
				d.Status.Conditions = []appsv1.DeploymentCondition{
					{
						Type:   appsv1.DeploymentAvailable,
						Status: corev1.ConditionTrue,
					},
					{
						Type:   appsv1.DeploymentReplicaFailure,
						Status: corev1.ConditionFalse,
					},
				}
				return nil
			})

			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)

			mockGardenClient.EXPECT().Get(ctx, kutil.KeyFromObject(seed), seed).DoAndReturn(func(_ context.Context, _ client.ObjectKey, s *gardencorev1beta1.Seed) error {
				s.ObjectMeta = seed.ObjectMeta
				s.Generation = 1
				s.Status.ObservedGeneration = 1
				s.Status.Conditions = []gardencorev1beta1.Condition{
					{
						Type:   gardencorev1beta1.SeedGardenletReady,
						Status: gardencorev1beta1.ConditionTrue,
					},
					{
						Type:   gardencorev1beta1.SeedBootstrapped,
						Status: gardencorev1beta1.ConditionTrue,
					},
				}
				return nil
			})

			err := landscaper.waitForRolloutToBeComplete(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("#getKubeconfigWithBootstrapToken", func() {
		var (
			restConfig = &rest.Config{
				Host: "apiserver.dummy",
			}
			tokenID                  = utils.ComputeSHA256Hex([]byte(seed.Name))[:6]
			bootstrapTokenSecretName = bootstraptokenutil.BootstrapTokenSecretName(tokenID)
			timestampInTheFuture     = time.Now().UTC().Add(15 * time.Hour).Format(time.RFC3339)
			timestampInThePast       = time.Now().UTC().Add(-15 * time.Hour).Format(time.RFC3339)
		)

		It("should fail to parse the expiration time on the existing bootstrap token", func() {
			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenInterface.EXPECT().RESTConfig().Return(restConfig)
			mockGardenClient.EXPECT().Get(ctx, kutil.Key(metav1.NamespaceSystem, bootstrapTokenSecretName), gomock.AssignableToTypeOf(&corev1.Secret{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, s *corev1.Secret) error {
				s.Data = map[string][]byte{
					bootstraptokenapi.BootstrapTokenExpirationKey: []byte("unparsable time"),
				}
				return nil
			})

			kubeconfig, err := landscaper.getKubeconfigWithBootstrapToken(ctx, seed.Name)
			Expect(err).To(HaveOccurred())
			Expect(kubeconfig).To(BeNil())
		})

		It("should successfully refresh the bootstrap token", func() {
			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenInterface.EXPECT().RESTConfig().Return(restConfig)
			// There are 3 calls requesting the same secret in the code. This can be improved.
			// However it is not critical as bootstrap token generation does not happen too frequently
			mockGardenClient.EXPECT().Get(ctx, kutil.Key(metav1.NamespaceSystem, bootstrapTokenSecretName), gomock.AssignableToTypeOf(&corev1.Secret{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, s *corev1.Secret) error {
				s.Data = map[string][]byte{
					bootstraptokenapi.BootstrapTokenExpirationKey: []byte(timestampInThePast),
				}
				return nil
			})
			mockGardenClient.EXPECT().Get(ctx, kutil.Key(metav1.NamespaceSystem, bootstrapTokenSecretName), gomock.AssignableToTypeOf(&corev1.Secret{})).Return(nil).Times(2)

			mockGardenClient.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, s *corev1.Secret) error {
				Expect(s.Name).To(Equal(bootstrapTokenSecretName))
				Expect(s.Namespace).To(Equal(metav1.NamespaceSystem))
				Expect(s.Type).To(Equal(bootstraptokenapi.SecretTypeBootstrapToken))
				Expect(s.Data).ToNot(BeNil())
				Expect(s.Data[bootstraptokenapi.BootstrapTokenDescriptionKey]).ToNot(BeNil())
				Expect(s.Data[bootstraptokenapi.BootstrapTokenIDKey]).To(Equal([]byte(tokenID)))
				Expect(s.Data[bootstraptokenapi.BootstrapTokenSecretKey]).ToNot(BeNil())
				Expect(s.Data[bootstraptokenapi.BootstrapTokenExpirationKey]).ToNot(BeNil())
				Expect(s.Data[bootstraptokenapi.BootstrapTokenUsageAuthentication]).To(Equal([]byte("true")))
				Expect(s.Data[bootstraptokenapi.BootstrapTokenUsageSigningKey]).To(Equal([]byte("true")))
				return nil
			})

			kubeconfig, err := landscaper.getKubeconfigWithBootstrapToken(ctx, seed.Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(kubeconfig).ToNot(BeNil())

			rest, err := kubernetes.RESTConfigFromKubeconfig(kubeconfig)
			Expect(err).ToNot(HaveOccurred())
			Expect(rest.Host).To(Equal(restConfig.Host))
		})

		It("should reuse existing bootstrap token", func() {
			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenInterface.EXPECT().RESTConfig().Return(restConfig)
			mockGardenClient.EXPECT().Get(ctx, kutil.Key(metav1.NamespaceSystem, bootstrapTokenSecretName), gomock.AssignableToTypeOf(&corev1.Secret{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, s *corev1.Secret) error {
				s.Data = map[string][]byte{
					bootstraptokenapi.BootstrapTokenExpirationKey: []byte(timestampInTheFuture),
					bootstraptokenapi.BootstrapTokenIDKey:         []byte("dummy"),
					bootstraptokenapi.BootstrapTokenSecretKey:     []byte(bootstrapTokenSecretName),
				}
				return nil
			})

			kubeconfig, err := landscaper.getKubeconfigWithBootstrapToken(ctx, seed.Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(kubeconfig).ToNot(BeNil())

			rest, err := kubernetes.RESTConfigFromKubeconfig(kubeconfig)
			Expect(err).ToNot(HaveOccurred())
			Expect(rest.Host).To(Equal(restConfig.Host))
		})
	})

	Describe("#deployBackupSecret", func() {
		secretName := "backup-secret"
		secretNamespace := "garden"
		var (
			backupSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: secretNamespace,
				},
			}
			backupSecretRef = corev1.SecretReference{
				Name:      secretName,
				Namespace: secretNamespace,
			}

			providerName = "abc"
			credentials  = map[string][]byte{
				"KEY": []byte("value"),
			}
		)

		It("should create the backup secret successfully", func() {
			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenClient.EXPECT().Get(ctx, kutil.KeyFromObject(backupSecret), backupSecret).Return(apierrors.NewNotFound(schema.GroupResource{}, backupSecret.Name))

			expectedSecret := *backupSecret
			expectedSecret.Data = credentials
			expectedSecret.Type = corev1.SecretTypeOpaque
			expectedSecret.Labels = map[string]string{
				"provider": providerName,
			}
			mockGardenClient.EXPECT().Create(ctx, &expectedSecret).Return(nil)

			err := landscaper.deployBackupSecret(ctx, providerName, credentials, backupSecretRef)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should update the secret successfully", func() {
			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenClient.EXPECT().Get(ctx, kutil.KeyFromObject(backupSecret), backupSecret).Return(nil)

			expectedSecret := *backupSecret
			expectedSecret.Data = credentials
			expectedSecret.Type = corev1.SecretTypeOpaque
			expectedSecret.Labels = map[string]string{
				"provider": providerName,
			}
			mockGardenClient.EXPECT().Update(ctx, &expectedSecret).Return(nil)

			err := landscaper.deployBackupSecret(ctx, providerName, credentials, backupSecretRef)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return an error when failing to deploy the secret", func() {
			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenClient.EXPECT().Get(ctx, kutil.KeyFromObject(backupSecret), backupSecret).Return(nil)
			mockGardenClient.EXPECT().Update(ctx, gomock.Any()).Return(fmt.Errorf("some error"))

			err := landscaper.deployBackupSecret(ctx, providerName, credentials, backupSecretRef)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("#deploySeedSecret", func() {
		secretName := "seed-secret"
		secretNamespace := "garden"
		var (
			seedSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: secretNamespace,
				},
			}
			secretRef = &corev1.SecretReference{
				Name:      secretName,
				Namespace: secretNamespace,
			}

			kubeconfigContent = "very secure"
		)

		It("should create the secret successfully", func() {
			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenClient.EXPECT().Get(ctx, kutil.KeyFromObject(seedSecret), seedSecret).Return(apierrors.NewNotFound(schema.GroupResource{}, seedSecret.Name))

			expectedSecret := *seedSecret
			expectedSecret.Data = map[string][]byte{
				"kubeconfig": []byte(kubeconfigContent),
			}
			expectedSecret.Type = corev1.SecretTypeOpaque
			mockGardenClient.EXPECT().Create(ctx, &expectedSecret).Return(nil)

			err := landscaper.deploySeedSecret(ctx, kubeconfigContent, secretRef)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should update the secret successfully", func() {
			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenClient.EXPECT().Get(ctx, kutil.KeyFromObject(seedSecret), seedSecret).Return(nil)

			expectedSecret := *seedSecret
			expectedSecret.Data = map[string][]byte{
				"kubeconfig": []byte(kubeconfigContent),
			}
			expectedSecret.Type = corev1.SecretTypeOpaque
			mockGardenClient.EXPECT().Update(ctx, &expectedSecret).Return(nil)

			err := landscaper.deploySeedSecret(ctx, kubeconfigContent, secretRef)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return an error when failing to deploy the secret", func() {
			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenClient.EXPECT().Get(ctx, kutil.KeyFromObject(seedSecret), seedSecret).Return(nil)
			mockGardenClient.EXPECT().Update(ctx, gomock.Any()).Return(fmt.Errorf("some error"))

			err := landscaper.deploySeedSecret(ctx, kubeconfigContent, secretRef)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("#isSeedBootstrapped", func() {
		It("the requested seed is bootstrapped", func() {
			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenClient.EXPECT().Get(ctx, kutil.KeyFromObject(seed), seed).DoAndReturn(func(_ context.Context, _ client.ObjectKey, s *gardencorev1beta1.Seed) error {
				s.ObjectMeta = seed.ObjectMeta
				s.Status.KubernetesVersion = pointer.StringPtr("some version")
				return nil
			})

			exists, err := landscaper.isSeedBootstrapped(ctx, seed.ObjectMeta)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(Equal(true))
		})

		It("the requested seed does NOT exist", func() {
			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenClient.EXPECT().Get(ctx, kutil.KeyFromObject(seed), seed).Return(apierrors.NewNotFound(schema.GroupResource{}, seed.Name))

			exists, err := landscaper.isSeedBootstrapped(ctx, seed.ObjectMeta)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(Equal(false))
		})

		It("the requested seed's Kubernetes version is not yet discovered", func() {
			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenClient.EXPECT().Get(ctx, kutil.KeyFromObject(seed), seed).DoAndReturn(func(_ context.Context, _ client.ObjectKey, s *gardencorev1beta1.Seed) error {
				s.ObjectMeta = seed.ObjectMeta
				s.Status.KubernetesVersion = nil
				return nil
			})

			exists, err := landscaper.isSeedBootstrapped(ctx, seed.ObjectMeta)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(Equal(false))
		})

		It("expecting an error", func() {
			mockGardenInterface.EXPECT().Client().Return(mockGardenClient)
			mockGardenClient.EXPECT().Get(ctx, kutil.KeyFromObject(seed), seed).Return(fmt.Errorf("fake error"))

			exists, err := landscaper.isSeedBootstrapped(ctx, seed.ObjectMeta)
			Expect(err).To(HaveOccurred())
			Expect(exists).To(Equal(false))
		})
	})
})
