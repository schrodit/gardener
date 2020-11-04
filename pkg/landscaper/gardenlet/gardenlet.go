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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	v2 "github.com/gardener/component-spec/bindings-go/apis/v2"
	"github.com/gardener/component-spec/bindings-go/codec"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/landscaper/gardenlet/apis/imports"
	"github.com/gardener/gardener/pkg/landscaper/gardenlet/applier"
	"github.com/gardener/gardener/pkg/logger"
	"github.com/gardener/gardener/pkg/operation/common"
	"github.com/gardener/gardener/pkg/utils"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/kubernetes/bootstrap"
	"github.com/gardener/gardener/pkg/utils/kubernetes/health"
	"github.com/gardener/gardener/pkg/utils/retry"
)

// Landscaper has all the context and parameters needed to run a Gardenlet landscaper.
type Landscaper struct {
	log                            *logrus.Entry
	gardenClient                   kubernetes.Interface
	seedClient                     client.Client
	Imports                        *imports.LandscaperGardenletImport
	landscaperOperation            string
	imageVectorOverride            *string
	componentImageVectorOverwrites *string
	gardenletImageRepository       string
	gardenletImageVersion          string
}

// NewGardenletLandscaper creates a new Gardenlet landscaper
func NewGardenletLandscaper(imports *imports.LandscaperGardenletImport, landscaperOperation, componentDescriptorPath string) (*Landscaper, error) {
	landscaper := &Landscaper{
		log:                 logger.NewFieldLogger(logger.NewLogger("info"), "landscaper-gardenlet operation", landscaperOperation),
		Imports:             imports,
		landscaperOperation: landscaperOperation,
	}

	componentDescriptorData, err := ioutil.ReadFile(componentDescriptorPath)
	if err != nil {
		return nil, fmt.Errorf("failed parsing the Gardenlet component descriptor: %v", err)
	}

	componentDescriptorList := &v2.ComponentDescriptorList{}
	err = codec.Decode(componentDescriptorData, componentDescriptorList)
	if err != nil {
		return nil, fmt.Errorf("failed parsing the Gardenlet component descriptor: %v", err)
	}

	err = landscaper.parseGardenletImage(componentDescriptorList)
	if err != nil {
		return nil, err
	}

	err = landscaper.parseImageVectorOverride()
	if err != nil {
		return nil, fmt.Errorf("failed parsing imports: %v", err)
	}

	// Create Garden client
	gardenClient, err := kubernetes.NewClientFromBytes([]byte(imports.GardenCluster.Spec.Configuration.Kubeconfig), kubernetes.WithClientOptions(
		client.Options{
			Scheme: kubernetes.GardenScheme,
		}))
	if err != nil {
		return nil, fmt.Errorf("failed creating client for Garden cluster: %v", err)
	}
	landscaper.gardenClient = gardenClient

	return landscaper, nil
}

func (g Landscaper) Run(ctx context.Context) error {
	switch g.landscaperOperation {
	case "RECONCILE":
		return g.Reconcile(ctx)
	case "DELETE":
		return g.Delete(ctx)
	default:
		return fmt.Errorf("environment variable \"OPERATION\" must either be \"RECONCILE\" or \"DELETE\"")
	}
}

// Delete removes all deployed Gardenlet resources from the Seed cluster
func (g Landscaper) Delete(ctx context.Context) error {
	seedExists, err := isSeedBootstrapped(ctx, g.gardenClient.Client(), g.Imports.ComponentConfiguration.SeedConfig.ObjectMeta)
	if err != nil {
		return fmt.Errorf("failed to check if seed %q still exists: %v", g.Imports.ComponentConfiguration.SeedConfig.ObjectMeta.Name, err)
	}

	if seedExists {
		return fmt.Errorf("please delete seed %q before removing the Gardenlet deployment resources", g.Imports.ComponentConfiguration.SeedConfig.ObjectMeta.Name)
	}

	// Create Seed client
	seedClient, err := kubernetes.NewClientFromBytes([]byte(g.Imports.RuntimeCluster.Spec.Configuration.Kubeconfig))
	if err != nil {
		return fmt.Errorf("failed creating client for runtime cluster: %v", err)
	}

	applier, err := applier.NewGardenletChartApplier(seedClient.ChartApplier(), seedClient.Client(), map[string]interface{}{}, common.ChartPath)
	if err := applier.Destroy(ctx); err != nil {
		return fmt.Errorf("failed to delete Gardenlet chart from Seed cluster: %v", err)
	}

	g.log.Infof("Successfully deleted Gardenlet resources for Seed %q", g.Imports.ComponentConfiguration.SeedConfig.Name)
	return nil
}

// Reconcile deploys the Gardenlet into the Seed cluster
func (g Landscaper) Reconcile(ctx context.Context) error {
	seedConfig := g.Imports.ComponentConfiguration.SeedConfig

	// deploy seed secret with seed cluster kubeconfig to Garden cluster
	// only deploy if secret reference is configured in the Seed resource
	if g.Imports.ComponentConfiguration.SeedConfig.Spec.SecretRef != nil {
		seedKubeconfig := g.Imports.RuntimeCluster.Spec.Configuration.Kubeconfig
		if g.Imports.ComponentConfiguration.SeedClientConnection != nil && len(g.Imports.ComponentConfiguration.SeedClientConnection.Kubeconfig) > 0 {
			seedKubeconfig = g.Imports.ComponentConfiguration.SeedClientConnection.Kubeconfig
		}

		if err := deploySeedSecret(ctx, g.gardenClient.Client(), seedKubeconfig, g.Imports.ComponentConfiguration.SeedConfig.Spec.SecretRef); err != nil {
			return fmt.Errorf("failed to deploy secret for the Seed resource containing the Seed cluster's kubeconfig: %v", err)
		}
	}

	// deploy seed-backup secret to Garden cluster if required
	if seedConfig.Spec.Backup != nil {
		credentials := make(map[string][]byte)
		if err := json.Unmarshal(g.Imports.SeedBackup.Credentials.Raw, &credentials); err != nil {
			return err
		}

		if err := deployBackupSecret(ctx, g.gardenClient.Client(), g.Imports.SeedBackup.Provider, credentials, seedConfig.Spec.Backup.SecretRef); err != nil {
			return fmt.Errorf("failed to deploy secret for the Seed resource containing the Seed cluster's kubeconfig: %v", err)
		}
	}

	// check if Gardenlet is already deployed / check Seed exists
	isAlreadyBootstrapped, err := isSeedBootstrapped(ctx, g.gardenClient.Client(), seedConfig.ObjectMeta)
	if err != nil {
		return fmt.Errorf("failed to check if seed %q is already bootstrapped: %v", seedConfig.ObjectMeta.Name, err)
	}

	var bootstrapKubeconfig []byte
	if !isAlreadyBootstrapped {
		bootstrapKubeconfig, err = getKubeconfigWithBootstrapToken(ctx, g.gardenClient.Client(), g.gardenClient.RESTConfig(), seedConfig.ObjectMeta.Name)
		if err != nil {
			return fmt.Errorf("failed to compute bootstrap kubeconfig: %v", err)
		}
	}

	// create required RBAC resources in the Garden cluster
	rbacApplier := applier.NewGardenRBACApplier(g.gardenClient.ChartApplier(), g.gardenClient.Client(), common.ChartPath)
	if err := rbacApplier.Deploy(ctx); err != nil {
		return fmt.Errorf("failed to deploy the RBAC resources to the Garden cluster: %v", err)
	}

	// Create Seed client
	seedClient, err := kubernetes.NewClientFromBytes([]byte(g.Imports.RuntimeCluster.Spec.Configuration.Kubeconfig))
	if err != nil {
		return fmt.Errorf("failed creating client for runtime cluster: %v", err)
	}

	g.seedClient = seedClient.Client()

	// deploy the VPA CRD first, and then later the VPA resource with the Gardenlet chart
	if seedConfig.Spec.Settings.VerticalPodAutoscaler != nil && seedConfig.Spec.Settings.VerticalPodAutoscaler.Enabled {
		applier, err := applier.NewVPACRDApplier(seedClient.ChartApplier(), seedClient.Client(), common.ChartPath)
		if err != nil {
			return fmt.Errorf("failed to create chart applier for VPA CRD: %v", err)
		}
		if err := applier.Deploy(ctx); err != nil {
			return fmt.Errorf("failed to deploy VPA CRD to the Seed cluster: %v", err)
		}
	}

	gardenNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: v1beta1constants.GardenNamespace}}
	if err := seedClient.Client().Create(ctx, gardenNamespace); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	values := g.computeGardenletChartValues(bootstrapKubeconfig)
	applier, err := applier.NewGardenletChartApplier(seedClient.ChartApplier(), seedClient.Client(), values, common.ChartPath)
	if err := applier.Deploy(ctx); err != nil {
		return fmt.Errorf("failed deploying Gardenlet chart to Seed cluster: %v", err)
	}

	g.log.Infof("Successfully deployed Gardenlet resources for Seed %q", g.Imports.ComponentConfiguration.SeedConfig.Name)

	return g.waitForRolloutToBeComplete(ctx)
}

func (g Landscaper) computeGardenletChartValues(bootstrapKubeconfig []byte) map[string]interface{} {
	gardenClientConnection := map[string]interface{}{
		"acceptContentTypes": g.Imports.ComponentConfiguration.GardenClientConnection.AcceptContentTypes,
		"contentType":        g.Imports.ComponentConfiguration.GardenClientConnection.ContentType,
		"qps":                g.Imports.ComponentConfiguration.GardenClientConnection.QPS,
		"burst":              g.Imports.ComponentConfiguration.GardenClientConnection.Burst,
		"kubeconfigSecret":   *g.Imports.ComponentConfiguration.GardenClientConnection.KubeconfigSecret,
	}

	if len(bootstrapKubeconfig) > 0 {
		gardenClientConnection["bootstrapKubeconfig"] = map[string]interface{}{
			"name":       g.Imports.ComponentConfiguration.GardenClientConnection.BootstrapKubeconfig.Name,
			"namespace":  g.Imports.ComponentConfiguration.GardenClientConnection.BootstrapKubeconfig.Namespace,
			"kubeconfig": string(bootstrapKubeconfig),
		}
	}

	if g.Imports.ComponentConfiguration.GardenClientConnection.GardenClusterAddress != nil {
		gardenClientConnection["gardenClusterAddress"] = *g.Imports.ComponentConfiguration.GardenClientConnection.GardenClusterAddress
	}

	configValues := map[string]interface{}{
		"gardenClientConnection": gardenClientConnection,
		"featureGates":           g.Imports.ComponentConfiguration.FeatureGates,
		"server":                 *g.Imports.ComponentConfiguration.Server,
		"seedConfig":             *g.Imports.ComponentConfiguration.SeedConfig,
	}

	if g.Imports.ComponentConfiguration.ShootClientConnection != nil {
		configValues["shootClientConnection"] = *g.Imports.ComponentConfiguration.ShootClientConnection
	}

	if g.Imports.ComponentConfiguration.SeedClientConnection != nil {
		configValues["seedClientConnection"] = *g.Imports.ComponentConfiguration.SeedClientConnection
	}

	if g.Imports.ComponentConfiguration.Controllers != nil {
		configValues["controllers"] = *g.Imports.ComponentConfiguration.Controllers
	}

	if g.Imports.ComponentConfiguration.LeaderElection != nil {
		configValues["leaderElection"] = *g.Imports.ComponentConfiguration.LeaderElection
	}

	if g.Imports.ComponentConfiguration.LogLevel != nil {
		configValues["logLevel"] = *g.Imports.ComponentConfiguration.LogLevel
	}

	if g.Imports.ComponentConfiguration.KubernetesLogLevel != nil {
		configValues["kubernetesLogLevel"] = *g.Imports.ComponentConfiguration.KubernetesLogLevel
	}

	if g.Imports.ComponentConfiguration.Logging != nil {
		configValues["logging"] = *g.Imports.ComponentConfiguration.Logging
	}

	gardenletValues := map[string]interface{}{
		"replicaCount":       1,
		"serviceAccountName": "gardenlet",
		"image": map[string]interface{}{
			"repository": g.gardenletImageRepository,
			"tag":        g.gardenletImageVersion,
		},
		"vpa":    g.Imports.ComponentConfiguration.SeedConfig.Spec.Settings.VerticalPodAutoscaler != nil && g.Imports.ComponentConfiguration.SeedConfig.Spec.Settings.VerticalPodAutoscaler.Enabled,
		"config": configValues,
	}

	if g.Imports.RevisionHistoryLimit != nil {
		gardenletValues["revisionHistoryLimit"] = *g.Imports.RevisionHistoryLimit
	}

	if g.imageVectorOverride != nil {
		gardenletValues["imageVectorOverwrite"] = *g.imageVectorOverride
	}

	if g.componentImageVectorOverwrites != nil {
		gardenletValues["componentImageVectorOverwrites"] = *g.componentImageVectorOverwrites
	}

	if g.Imports.Resources != nil {
		gardenletValues["resources"] = *g.Imports.Resources
	}

	return map[string]interface{}{
		"global": map[string]interface{}{
			"gardenlet": gardenletValues,
		},
	}
}

func getKubeconfigWithBootstrapToken(ctx context.Context, gardenClient client.Client, gardenClientRestConfig *rest.Config, seedName string) ([]byte, error) {
	var (
		tokenID  = utils.ComputeSHA256Hex([]byte(seedName))[:6]
		validity = 24 * time.Hour
	)
	return bootstrap.ComputeKubeconfigWithBootstrapToken(ctx, gardenClient, gardenClientRestConfig, tokenID, fmt.Sprintf("A bootstrap token for the Gardenlet for seed %q.", seedName), validity)
}

func isSeedBootstrapped(ctx context.Context, gardenClient client.Client, seedMeta metav1.ObjectMeta) (bool, error) {
	seed := &gardencorev1beta1.Seed{ObjectMeta: seedMeta}
	if err := gardenClient.Get(ctx, kutil.KeyFromObject(seed), seed); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return seed.Status.KubernetesVersion != nil, nil
}

func deploySeedSecret(ctx context.Context, gardenClient client.Client, runtimeClusterKubeconfig string, secretName *corev1.SecretReference) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName.Name,
			Namespace: secretName.Namespace,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, gardenClient, secret, func() error {
		secret.Data = map[string][]byte{
			kubernetes.KubeConfig: []byte(runtimeClusterKubeconfig),
		}
		secret.Type = corev1.SecretTypeOpaque
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func deployBackupSecret(ctx context.Context, gardenClient client.Client, providerName string, credentials map[string][]byte, backupSecretRef corev1.SecretReference) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupSecretRef.Name,
			Namespace: backupSecretRef.Namespace,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, gardenClient, secret, func() error {
		secret.Data = credentials
		secret.Type = corev1.SecretTypeOpaque
		secret.Labels = map[string]string{
			"provider": providerName,
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (g *Landscaper) parseImageVectorOverride() error {
	if g.Imports.ImageVectorOverwrite != nil {
		var imageVectorOverwrite string
		if err := json.Unmarshal(g.Imports.ImageVectorOverwrite.Raw, &imageVectorOverwrite); err != nil {
			return err
		}
		g.imageVectorOverride = &imageVectorOverwrite
	}

	if g.Imports.ComponentImageVectorOverwrites != nil {
		var componentImageVectorOverwrites string
		if err := json.Unmarshal(g.Imports.ComponentImageVectorOverwrites.Raw, &componentImageVectorOverwrites); err != nil {
			return err
		}
		g.componentImageVectorOverwrites = &componentImageVectorOverwrites
	}
	return nil
}

// parseGardenletImage gets the Gardenlet image from the component descriptor
// The component descriptor is the only image source and must be provided
func (g *Landscaper) parseGardenletImage(componentDescriptorList *v2.ComponentDescriptorList) error {
	// get a component by its name
	// The method returns a list as there could be multiple components with the same name but different version
	components := componentDescriptorList.GetComponentByName("github.com/gardener/gardener")
	if len(components) != 1 {
		return fmt.Errorf("expecting exactly one component with name \"github.com/gardener/gardener\"")
	}

	// get gardenlet image from component descriptor
	res := components[0].GetLocalResourcesByName(v2.OCIImageType, "gardenlet")
	if len(res) == 0 {
		return fmt.Errorf("OCI image for gardenlet not found in component descriptor")
	}
	gardenletResource := res[0]
	imageVersion := gardenletResource.GetVersion()
	if len(imageVersion) == 0 {
		return fmt.Errorf("OCI image version for gardenlet not found in component descriptor")
	}
	imageReference := gardenletResource.Access.(*v2.OCIRegistryAccess).ImageReference
	if len(imageReference) == 0 {
		return fmt.Errorf("OCI image reference for gardenlet not found in component descriptor")
	}

	// split version from reference eu.gcr.io/gardener-project/gardener/gardenlet:v1.11.3
	split := strings.Split(imageReference, ":")
	if len(split) != 2 {
		return fmt.Errorf("OCI image repository for gardenlet not found in component descriptor")
	}
	g.gardenletImageRepository = split[0]
	g.gardenletImageVersion = imageVersion
	return nil
}

func (g Landscaper) waitForRolloutToBeComplete(ctx context.Context) error {
	g.log.Info("Waiting for the Gardenlet to be rolled out successfully...")

	// sleep for couple seconds to give the gardenlet process time to startup
	time.Sleep(10 * time.Second)

	var (
		deploymentRolloutSuccessful bool
		seedIsRegistered            bool
	)

	return retry.UntilTimeout(ctx, 10*time.Second, 15*time.Minute, func(ctx context.Context) (done bool, err error) {
		gardenletDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gardenlet",
				Namespace: v1beta1constants.GardenNamespace,
			},
		}

		err = g.seedClient.Get(ctx, client.ObjectKey{Namespace: v1beta1constants.GardenNamespace, Name: v1beta1constants.DeploymentNameGardenlet}, gardenletDeployment)
		if err != nil {
			return retry.SevereError(err)
		}

		if err := health.CheckDeployment(gardenletDeployment); err != nil {
			msg := fmt.Sprintf("gardenlet deployment is not rolled out successfuly yet...: %v", err)
			g.log.Info(msg)
			return retry.MinorError(fmt.Errorf(msg))
		}

		if !deploymentRolloutSuccessful {
			g.log.Info("Gardenlet deployment rollout successful!")
		}

		seed := &gardencorev1beta1.Seed{
			ObjectMeta: metav1.ObjectMeta{
				Name: g.Imports.ComponentConfiguration.SeedConfig.Name,
			},
		}

		err = g.gardenClient.Client().Get(ctx, kutil.KeyFromObject(seed), seed)
		if err != nil {
			if apierrors.IsNotFound(err) {
				msg := fmt.Sprintf("seed %q is not yet registered...: %v", seed.Name, err)
				g.log.Info(msg)
				return retry.MinorError(fmt.Errorf(msg))
			}
			return retry.SevereError(err)
		}

		if !seedIsRegistered {
			g.log.Infof("Seed %q is registered!", seed.Name)
		}

		// Check Seed without caring for the observing Gardener version (unknown to the landscaper)
		err = health.CheckSeed(seed, seed.Status.Gardener)
		if err != nil {
			msg := fmt.Sprintf("seed %q is not ready yet...: %v", seed.Name, err)
			g.log.Info(msg)
			return retry.MinorError(fmt.Errorf(msg))
		}
		g.log.Info("Seed is bootstrapped and ready!")
		return retry.Ok()
	})
}
