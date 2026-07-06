# comfyui-operator
ComfyUI-Operator is a Kubernetes operator designed to make it easy to deploy and run ComfyUI on Kubernetes or OpenShift. 

## Description
ComfyUI is a powerful node-based UI for Stable Diffusion and other generative AI models. Running it on Kubernetes typically requires hand-crafting several tightly coupled resources: a Deployment with GPU tolerations and resource limits, a PersistentVolumeClaim for models and 
outputs, a Service, and an ingress resource that varies by platform (OpenShift Route vs. Gateway API HTTPRoute). Add authentication and you're managing an OAuth2 proxy sidecar on top of all that.
This operator collapses that complexity into a single ComfyUI custom resource. It:
 - Provisions storage automatically — creates and manages a PVC with configurable size, storage class, and access mode. Delete the CR and the PVC is garbage-collected via owner references.
 - Detects your platform's ingress API — auto-discovers whether the cluster supports OpenShift Routes or the Kubernetes Gateway API and creates the appropriate resource. On vanilla clusters with neither, it falls back to ClusterIP-only.
 - Makes GPU scheduling trivial — set enableGPU: true and gpuCount: N and the operator injects nvidia.com/gpu resource limits into the pod spec.
 - Provides built-in OAuth2 authentication — optionally deploys an OAuth2 Proxy sidecar supporting GitHub, Google, and generic OIDC providers with email and domain allowlists.
 - Manages the full lifecycle idempotently — the controller converges Deployments, Services, PVCs, Routes/HTTPRoutes, and Secrets to match the desired state on every reconciliation loop.


## Getting Started

### Prerequisites
- go version v1.26.0+
- podman or docker version 17.03+.
- kubectl/oc version v1.11.3+.
- Access to a Kubernetes v1.11.3+ or OpenShift 4.x+ cluster.

### To Deploy on the cluster

**Install the CRDs into the cluster:**

```sh
make install
```

**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/comfyui-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

To use podman instead of docker, add `CONTAINER_TOOL=podman` to the build command.

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/comfyui-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Build and push the ComfyUI application image:**

The operator manages ComfyUI instances, but the application image must be built and pushed separately. Dockerfiles are provided in `test-image/`.

```sh
docker build -t <some-registry>/comfyui-cpu:latest -f test-image/Dockerfile test-image/
docker push <some-registry>/comfyui-cpu:latest
```

> A GPU Dockerfile is also available at `test-image/Dockerfile.gpu`.

**Create instances of your solution**
You can apply the samples (examples) from the config/samples:

```sh
kubectl apply -f config/samples/openshift-cpu-simple.yaml
```

See `config/samples/` for additional examples including GPU, OAuth2, and custom storage configurations.

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/comfyui-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/comfyui-operator/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
operator-sdk edit --plugins=helm/v1-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

