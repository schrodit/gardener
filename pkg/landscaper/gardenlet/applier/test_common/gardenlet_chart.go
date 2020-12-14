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

package test_common

import (
	"context"
	"time"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	baseconfig "k8s.io/component-base/config"
	"k8s.io/klog"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	gardencorev1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	gardenletconfig "github.com/gardener/gardener/pkg/gardenlet/apis/config"
	gardenletconfigv1alpha1 "github.com/gardener/gardener/pkg/gardenlet/apis/config/v1alpha1"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
)

// ValidateGardenletChartPriorityClass validates the priority class of the Gardenlet chart.
func ValidateGardenletChartPriorityClass(ctx context.Context, c client.Client) {
	priorityClass := getEmptyPriorityClass()

	Expect(c.Get(
		ctx,
		kutil.KeyFromObject(priorityClass),
		priorityClass,
	)).ToNot(HaveOccurred())
	Expect(priorityClass.GlobalDefault).To(Equal(false))
	Expect(priorityClass.Value).To(Equal(int32(1000000000)))
}

func getEmptyPriorityClass() *schedulingv1.PriorityClass {
	return &schedulingv1.PriorityClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gardenlet",
		},
	}
}

// ValidateGardenletChartRBAC validates the RBAC resources of the Gardenlet chart.
func ValidateGardenletChartRBAC(ctx context.Context, c client.Client, expectedLabels map[string]string) {
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

// ValidateGardenletChartServiceAccount validates the Service Account of the Gardenlet chart.
func ValidateGardenletChartServiceAccount(ctx context.Context, c client.Client, hasSeedClientConnectionKubeconfig bool, expectedLabels map[string]string) {
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

const acceptContentType = "application/json"

// ComputeExpectedGardenletConfiguration computes the expected Gardenlet configuration based
// on input parameters.
func ComputeExpectedGardenletConfiguration(componentConfigUsesTlsServerConfig, hasGardenClientConnectionKubeconfig, hasSeedClientConnectionKubeconfig bool, bootstrapKubeconfig *corev1.SecretReference, bootstrapKubeconfigSecret *corev1.SecretReference, seedConfig *gardenletconfig.SeedConfig) gardenletconfig.GardenletConfiguration {
	var (
		zero   = 0
		one    = 1
		three  = 3
		five   = 5
		twenty = 20

		logLevelInfo        = "info"
		lockObjectName      = "gardenlet-leader-election"
		lockObjectNamespace = "garden"
		kubernetesLogLevel  = new(klog.Level)
	)
	Expect(kubernetesLogLevel.Set("0")).ToNot(HaveOccurred())

	config := gardenletconfig.GardenletConfiguration{
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
				DNSEntryTTLSeconds: pointer.Int64Ptr(120),
			},
			ShootedSeedRegistration: &gardenletconfig.ShootedSeedRegistrationControllerConfiguration{
				SyncJitterPeriod: &metav1.Duration{
					Duration: 300000000000,
				},
			},
			ShootCare: &gardenletconfig.ShootCareControllerConfiguration{
				ConcurrentSyncs: &five,
				SyncPeriod: &metav1.Duration{
					Duration: 30 * time.Second,
				},
				StaleExtensionHealthChecks: &gardenletconfig.StaleExtensionHealthChecks{
					Enabled:   true,
					Threshold: &metav1.Duration{Duration: 300000000000},
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
		Resources: &gardenletconfig.ResourcesConfiguration{
			Capacity: corev1.ResourceList{
				"shoots": resource.MustParse("250"),
			},
		},
		SNI: &gardenletconfig.SNI{Ingress: &gardenletconfig.SNIIngress{
			ServiceName: pointer.StringPtr(gardenletconfigv1alpha1.DefaultSNIIngresServiceName),
			Namespace:   pointer.StringPtr(gardenletconfigv1alpha1.DefaultSNIIngresNamespace),
			Labels:      map[string]string{"istio": "ingressgateway"},
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
		config.GardenClientConnection.KubeconfigSecret = bootstrapKubeconfigSecret
	}

	if seedConfig != nil {
		config.SeedConfig = seedConfig
	}

	return config
}

// VerifyGardenletComponentConfigConfigMap verifies that the actual Gardenlet component config config map equals the expected config map.
func VerifyGardenletComponentConfigConfigMap(ctx context.Context, c client.Client, universalDecoder runtime.Decoder, expectedGardenletConfig gardenletconfig.GardenletConfiguration, expectedLabels map[string]string) {
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

func getEmptyGardenletConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gardenlet-configmap",
			Namespace: gardencorev1beta1constants.GardenNamespace,
		},
	}
}

// GetExpectedGardenletDeploymentSpec computes the expected Gardenlet deployment spec based on input parameters
func ComputeExpectedGardenletDeploymentSpec(imageVectorOverride, imageVectorOverrideComponents *string, componentConfigUsesTlsServerConfig bool, gardenClientConnectionKubeconfig, seedClientConnectionKubeconfig *string, expectedLabels map[string]string) appsv1.DeploymentSpec {
	deployment := appsv1.DeploymentSpec{
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
						VolumeMounts:             []corev1.VolumeMount{},
					},
				},
				Volumes: []corev1.Volume{},
			},
		},
	}

	if imageVectorOverride != nil {
		deployment.Template.Spec.Containers[0].Env = append(deployment.Template.Spec.Containers[0].Env, corev1.EnvVar{
			Name:  "IMAGEVECTOR_OVERWRITE",
			Value: "/charts_overwrite/images_overwrite.yaml",
		})
		deployment.Template.Spec.Containers[0].VolumeMounts = append(deployment.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "gardenlet-imagevector-overwrite",
			ReadOnly:  true,
			MountPath: "/charts_overwrite",
		})
		deployment.Template.Spec.Volumes = append(deployment.Template.Spec.Volumes, corev1.Volume{
			Name: "gardenlet-imagevector-overwrite",
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
			Name:  "IMAGEVECTOR_OVERWRITE_COMPONENTS",
			Value: "/charts_overwrite_components/components.yaml",
		})
		deployment.Template.Spec.Containers[0].VolumeMounts = append(deployment.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "gardenlet-imagevector-overwrite-components",
			ReadOnly:  true,
			MountPath: "/charts_overwrite_components",
		})
		deployment.Template.Spec.Volumes = append(deployment.Template.Spec.Volumes, corev1.Volume{
			Name: "gardenlet-imagevector-overwrite-components",
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
			Name: "gardenlet-kubeconfig-garden",
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
			Name: "gardenlet-kubeconfig-seed",
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
			Name: "gardenlet-cert",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "gardenlet-cert",
				},
			},
		})
	}

	return deployment
}

// VerifyGardenletDeployment verifies that the actual Gardenlet deployment equals the expected deployment
func VerifyGardenletDeployment(ctx context.Context, c client.Client, expectedDeploymentSpec appsv1.DeploymentSpec, hasImageVectorOverride, hasComponentImageVectorOverride, componentConfigHasTLSServerConfig, hasGardenClientConnectionKubeconfig, hasSeedClientConnectionKubeconfig, usesTLSBootstrapping bool, expectedLabels map[string]string) {
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

func getEmptyGardenletDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gardenlet",
			Namespace: gardencorev1beta1constants.GardenNamespace,
		},
	}
}
