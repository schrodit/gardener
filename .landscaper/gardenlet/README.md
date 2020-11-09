# Gardenlet Landscaper Component

This component is supposed to be used in the context of the [Gardener Landscaper](https://github.com/gardener/landscaper).
Its purpose is to deploy a Gardenlet to a Kubernetes cluster
and automatically register this cluster as a `Seed` resource in the Gardener installation.

TLS bootstrapping using a bootstrap token is used to obtain a valid Gardenlet certificate for the Garden cluster.
Essentially follows this [documentation](../../docs/deployment/deploy_gardenlet_manually.md).

## Run

The Gardenlet landscaper is supposed to run as part of the [Gardener Landscaper](https://github.com/gardener/landscaper) but can also be executed stand-alone.

The Gardenlet landscaper does not support command line arguments.
However, make sure to set the following environment variables
- `IMPORTS_PATH` contains the path to a file containing [required import configuration](#required-configuration).
- `OPERATION` is set to either `RECONCILE` (deploys the Gardenlet) or `DELETE` to remove the deployed resources.
- `COMPONENT_DESCRIPTOR_PATH` contains the path to a file containing a component descriptor for the Gardenlet. 
   This file contains the OCI image reference to use for the Gardenlet deployment.
   You can find a sample descriptor [here](component_descriptor_list.yaml).
   
The Gardenlet landscaper can be run locally by executing the below `make` statement.
The filepath to a valid landscaper import configuration file has to be provided as the first argument.
The [example file](./example/imports.yaml) can be used as a blueprint.

```
make start-landscaper-gardenlet IMPORT_PATH=<filepath>
```

## Import Configuration

You can find an example import configuration file [here](./example/imports.yaml).

### Required configuration

- `runtimeCluster` contains the kubeconfig for the Kubernetes cluster where the Gardenlet is deployed to. 
   Requires admin permissions. Only used for the Gardenlet deployment. 
   This is the Kubernetes cluster targeted as Seed (via in-cluster mounted service account token),
   if not otherwise specified in `.componentConfiguration.seedClientConnection.kubeconfig`.
- `gardenCluster` contains the kubeconfig for the Garden cluster. Requires admin permissions to create necessary RBAC roles and secrets.
- `componentConfiguration` has to contain a valid [component configuration for the Gardenlet](../../example/20-componentconfig-gardenlet.yaml).
- `componentConfiguration.seedConfig` must contain the `Seed` resource that will be automatically registered by the Gardenlet. 

### Forbidden configuration 

Because the Gardenlet landscaper only supports TLS bootstrapping with a bootstrap token, the configuration of the field
`.componentConfiguration.gardenClientConnection.kubeconfig` is forbidden.

### Default values 

The field `.componentConfiguration.gardenClientConnection.kubeconfigSecret` defaults to:

```
kubeconfigSecret:
  name: gardenlet-kubeconfig
  namespace: garden
```

The field `.componentConfiguration.gardenClientConnection.bootstrapKubeconfig` defaults to:

```
bootstrapKubeconfig:
  name: gardenlet-kubeconfig-bootstrap
  namespace: garden
```

All other default values can be found in the [Gardenlet helm chart](../../charts/gardener/gardenlet/values.yaml).

### Seed backup

The field `componentConfiguration.seedConfig.backup` as well as the field `seedBackup` need to be set.
if `componentConfiguration.seedConfig.backup` is specified, also the field `seedBackup` must be configured with 
credentials for a matching provider.

Before the Gardenlet deployment, the Gardenlet landscaper creates a Kubernetes secret with the name and namespace given in `componentConfiguration.seedConfig.backup.secretRef`
in the Garden cluster containing the credentials given in `seedBackup`.

### Additional Considerations

If the configured in `componentConfiguration.seedConfig.secretRef`, the landscaper will create a secret in 
the Garden cluster containing the kubeconfig of the Seed cluster.
Per default the kubeconfig is taken from `runtimeCluster.config.kubeconfig`, or if specified, from `.componentConfiguration.seedClientConnection.kubeconfig`.

Please note that in the imports you can configure `.componentConfiguration.seedClientConnection.kubeconfig` with a different kubeconfig than
specified in `runtimeCluster` in the import configuration. The Gardenlet is deployed to the cluster given in `runtimeCluster` and can control
the cluster specified in `.componentConfiguration.seedClientConnection.kubeconfig`.
However, it is recommended to run the Gardenlet directly in the Seed cluster (in-cluster using auto-mounted service account token) 
due to network latency and cost. 
Per default the Gardenlet is deployed in-cluster.


