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


## CRD Reference

The `ComfyUI` custom resource accepts the following fields under `spec`:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `image` | string | *(required)* | Container image for the ComfyUI application |
| `replicas` | int | `1` | Number of pod replicas |
| `resources` | [ResourceRequirements](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/) | none | CPU/memory requests and limits |
| `serviceType` | string | `ClusterIP` | Kubernetes Service type (`ClusterIP`, `NodePort`, `LoadBalancer`) |
| `enableGPU` | bool | `false` | Adds `nvidia.com/gpu` resource limits to the pod |
| `gpuCount` | int | `1` | Number of GPUs to request (only used when `enableGPU: true`) |
| `fsGroup` | int | auto-detected | Pod `fsGroup` for volume permissions. Auto-detected from namespace annotations on OpenShift, falls back to `1000` |
| `nodeSelector` | map | none | Constrains pods to nodes with matching labels |

**Storage** (`spec.storage`):

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `size` | string | none | PVC size (e.g. `50Gi`) |
| `storageClassName` | string | cluster default | Storage class name. Leave empty for cluster default |
| `accessMode` | string | `ReadWriteOnce` | PVC access mode |

**Gateway** (`spec.gateway`) — Kubernetes Gateway API integration:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | `default-gateway` | Name of the Gateway to attach HTTPRoute to |
| `namespace` | string | `gateway-system` | Namespace where the Gateway is located |
| `hostname` | string | `{name}.example.com` | Hostname for the HTTPRoute |

**OAuth2** (`spec.oauth2`) — OAuth2 Proxy sidecar for filebrowser authentication:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `provider` | string | *(required)* | OAuth2 provider: `github`, `google`, or `oidc` |
| `clientID` | string | *(required)* | OAuth2 client ID |
| `clientSecretRef` | SecretKeySelector | *(required)* | Reference to a Secret containing the client secret (key: `client-secret`) |
| `issuerURL` | string | none | OIDC issuer URL (required only for `oidc` provider) |
| `allowedEmails` | []string | none | List of allowed email addresses. If empty, any authenticated user is allowed |
| `allowedDomains` | []string | none | List of allowed email domains (e.g. `example.com`) |
| `cookieSecretRef` | SecretKeySelector | auto-generated | Reference to a Secret for the cookie secret. If omitted, one is generated automatically |

Additional fields: `volumeMounts` and `volumes` can be used to add custom volume mounts beyond the operator-managed storage.

## OAuth2 Setup (Google)

To enable OAuth2 authentication for filebrowser, you need a Google OAuth client:

1. Go to [Google Cloud Console](https://console.cloud.google.com) → **APIs & Services** → **Credentials**
2. Click **Create Credentials** → **OAuth client ID**
3. Application type: **Web application**
4. Add an authorized redirect URI:
   ```
   https://<comfyui-name>-filebrowser-<namespace>.apps.<cluster-domain>/oauth2/callback
   ```
5. Copy the **Client ID** and **Client Secret**

Create a Kubernetes Secret with the client secret:

```sh
kubectl create secret generic oauth2-client-secret \
  --from-literal=client-secret=<your-client-secret>
```

Then configure the CR:

```yaml
spec:
  oauth2:
    provider: google
    clientID: <your-client-id>.apps.googleusercontent.com
    clientSecretRef:
      name: oauth2-client-secret
      key: client-secret
    allowedEmails:
      - user@example.com
```

If `allowedEmails` and `allowedDomains` are both empty, any authenticated user is allowed through.

## RBAC Requirements

The operator requires a `ClusterRole` with the following permissions. These are auto-generated from controller annotations via `make manifests`.

| API Group | Resources | Verbs | Why |
|-----------|-----------|-------|-----|
| `comfy.redhat.com` | `comfyuis`, `comfyuis/status`, `comfyuis/finalizers` | get, list, watch, create, update, patch, delete | Manage the ComfyUI custom resource lifecycle |
| `apps` | `deployments` | get, list, watch, create, update, patch, delete | Create and update the ComfyUI + filebrowser deployment |
| `core` | `services` | get, list, watch, create, update, patch, delete | Expose ComfyUI and filebrowser ports |
| `core` | `persistentvolumeclaims` | get, list, watch, create, update, patch, delete | Provision storage for models, outputs, and custom nodes |
| `core` | `secrets` | get, list, watch, create, update, patch, delete | Manage OAuth2 cookie secrets |
| `core` | `pods` | get, list, watch | Monitor pod status |
| `core` | `namespaces` | get, list, watch | Read namespace annotations for fsGroup auto-detection on OpenShift |
| `route.openshift.io` | `routes` | get, list, watch, create, update, patch, delete | Create OpenShift Routes for external access |
| `gateway.networking.k8s.io` | `gateways`, `httproutes` | get, list, watch, create, update, patch, delete | Create Gateway API HTTPRoutes on vanilla Kubernetes |

The generated `ClusterRole` is at `config/rbac/role.yaml`. On OpenShift, the operator's service account also needs permission to read namespace annotations — this is included by default.

## Getting Started

### Prerequisites
- go version v1.26.0+
- podman or docker version 17.03+.
- kubectl/oc version v1.11.3+.
- Access to a Kubernetes v1.11.3+ or OpenShift 4.x+ cluster.

### To Deploy on the cluster

**1. Create a namespace for your deployment:**

```sh
oc create namespace <namespace-name>
```

**2. Install the CRDs (cluster-wide):**

```sh
make install
```

**3. Update kustomization to use your namespace:**

```sh
cd config/default
kustomize edit set namespace <namespace-name>
cd ../..
```

**4. Build and push the operator image:**

For OpenShift clusters, use the internal image registry:

```sh
# Get the OpenShift registry URL
export OCP_REGISTRY=$(oc get route default-route -n openshift-image-registry --template=’{{ .spec.host }}’)

# Login to the registry
podman login $OCP_REGISTRY -u $(oc whoami) -p $(oc whoami -t)

# Build and push operator image
make docker-build docker-push IMG=$OCP_REGISTRY/<namespace-name>/comfyui-operator:latest CONTAINER_TOOL=podman
```

For other registries:

```sh
make docker-build docker-push IMG=<registry>/<namespace>/comfyui-operator:tag CONTAINER_TOOL=podman
```

> **NOTE**: Use `CONTAINER_TOOL=docker` if you prefer Docker over Podman.

**5. Deploy the operator to the cluster:**

```sh
make deploy IMG=$OCP_REGISTRY/<namespace-name>/comfyui-operator:latest
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin privileges or be logged in as admin.

**6. Build and push the ComfyUI application image:**

The operator manages ComfyUI instances, but the application image must be built separately. Dockerfiles are provided in `test-image/`.

```sh
# CPU version (recommended for testing)
podman build -t comfyui-cpu:latest -f test-image/Dockerfile test-image/
podman tag comfyui-cpu:latest $OCP_REGISTRY/<namespace-name>/comfyui-cpu:latest
podman push $OCP_REGISTRY/<namespace-name>/comfyui-cpu:latest
```

For GPU workloads:

```sh
# GPU version
podman build -t comfyui-gpu:latest -f test-image/Dockerfile.gpu test-image/
podman tag comfyui-gpu:latest $OCP_REGISTRY/<namespace-name>/comfyui-gpu:latest
podman push $OCP_REGISTRY/<namespace-name>/comfyui-gpu:latest
```

**7. (Optional) Create OAuth2 secret if using OAuth2 authentication:**

If your sample CR includes OAuth2 configuration, create the required secret:

```sh
oc create secret generic oauth2-client-secret \
  --from-literal=client-secret=<your-google-oauth-client-secret> \
  -n <namespace-name>
```

See the [OAuth2 Setup](#oauth2-setup-google) section for details on obtaining credentials.

**8. Deploy a ComfyUI instance:**

Update the sample CR with your namespace and deployment name, then apply:

```sh
# Option 1: Edit and apply directly
sed -e ‘s/<deployment-name>/comfyui/g’ \
    -e ‘s/<namespace>/<namespace-name>/g’ \
    config/samples/openshift-comfyui-demo.yaml | oc apply -f -

# Option 2: Use the pre-configured sample (update namespace first)
oc apply -f config/samples/openshift-comfyui.yaml
```

**9. Verify the deployment:**

```sh
# Check operator is running
oc get all -n <namespace-name>

# Check CRD is installed
oc get crd | grep comfyui

# Check ComfyUI instance
oc get comfyui -n <namespace-name>

# Check all resources created by the operator
oc get deployment,service,pvc,route -n <namespace-name>
```

**Complete example with `comfyui-demo` namespace:**

```sh
oc create namespace comfyui-demo
make install
cd config/default && kustomize edit set namespace comfyui-demo && cd ../..
export OCP_REGISTRY=$(oc get route default-route -n openshift-image-registry --template=’{{ .spec.host }}’)
podman login $OCP_REGISTRY -u $(oc whoami) -p $(oc whoami -t)
make docker-build docker-push IMG=$OCP_REGISTRY/comfyui-demo/comfyui-operator:latest CONTAINER_TOOL=podman
make deploy IMG=$OCP_REGISTRY/comfyui-demo/comfyui-operator:latest
podman build -t comfyui-cpu:latest -f test-image/Dockerfile test-image/
podman tag comfyui-cpu:latest $OCP_REGISTRY/comfyui-demo/comfyui-cpu:latest
podman push $OCP_REGISTRY/comfyui-demo/comfyui-cpu:latest
oc create secret generic oauth2-client-secret --from-literal=client-secret=<your-secret> -n comfyui-demo
sed -e ‘s/<deployment-name>/comfyui/g’ -e ‘s/<namespace>/comfyui-demo/g’ config/samples/openshift-comfyui-demo.yaml | oc apply -f -
oc get all -n comfyui-demo
oc get comfyui -n comfyui-demo
```

**Optional: Deploy vLLM-Omni for accelerated inference**

ComfyUI-vLLM-Omni custom nodes connect to a separate vLLM service for GPU-accelerated model inference. To deploy vLLM-Omni:

```sh
# Download the example template
wget https://raw.githubusercontent.com/pruggier0/Comfyui-Operator/main/config/samples/vllm-omni-example.yaml

# Edit the file to:
# 1. Set your namespace
# 2. Choose a model (uncomment one of the provided options)
# 3. Adjust GPU count and memory based on model size
# 4. Configure storage class

# Deploy
oc apply -f vllm-omni-example.yaml
```

The vLLM service will be accessible at `http://vllm-omni:8000/v1` from pods in the same namespace. In ComfyUI-vLLM-Omni nodes, set the URL field to this value.

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

### To Install Locally 

**Setup a local minikube cluster (Reccomend using QEMU)** 
```sh 
minikube start --driver=qemu -p <cluster-name>
``` 

**Install the CRDS**
```sh 
make install 
make run 
``` 

**In another terminal session, run** 
```sh
kubectl apply -f path/to/file.yaml
```

**port forwardin the application**
First find the service:
```sh 
kubectl get svc
``` 
The port forward the ComfyUI service 

```sh 
kubectl port-forward svc/<comfyui-service-name> 8188:8188
``` 

We also need to port forward the Oauth2 proxy to get to filebrowser
```sh 
kubectl port-forward svc/<comfyui-service-name> 4180:4180
``` 

ComfyUI and filebrowser can be accesses from http://localhost:8188, and from http://localhost:4180 respectively 

## Multi-Tenant Provisioning

The `provision-tenant.sh` script automates the creation of isolated ComfyUI tenants with all necessary resources. It's ideal for multi-tenant environments where each team or user needs their own ComfyUI instance with separate namespaces, resource quotas, and authentication.

### Quick Start

**Before using the script**, edit the configuration section at the top of `provision-tenant.sh`:

1. Set `OAUTH2_CLIENT_ID` to your OAuth2 client ID (or leave empty to disable OAuth)
2. Set `DEFAULT_IMAGE` to your ComfyUI container image
3. Replace all `comfy.example.com` references with your actual CRD API group
4. Update `.example.com` domain references to match your organization

**Basic usage:**

```sh
# Simple CPU tenant
./provision-tenant.sh marketing

# GPU-enabled tenant with custom storage
./provision-tenant.sh data-science --gpu --gpu-count 2 --storage 100Gi

# Custom configuration
./provision-tenant.sh research --gpu --custom-image registry/comfyui:v2.0 \
  --domain research.example.com --email researcher@example.com \
  --cpu-limit 4000m --mem-limit 16Gi
```

### What It Creates

The script provisions a complete isolated environment:

- **Namespace**: `tenant-<name>` with labels for management
- **RBAC**: Role and RoleBinding for tenant self-service
- **Resource Quota**: Limits on CPU, memory, GPU, and PVCs to prevent resource exhaustion
- **OAuth2 Secret**: Client secret for authentication (optional)
- **ComfyUI CR**: The custom resource with your specified configuration
- **Cross-namespace permissions**: Image pull permissions if using OpenShift internal registry

### Key Options

| Option | Description |
|--------|-------------|
| `--gpu` | Enable GPU support (adds `nvidia.com/gpu` resource limits) |
| `--gpu-count N` | Number of GPUs per pod (default: 1) |
| `--storage SIZE` | PVC size (default: 50Gi) |
| `--replicas N` | Number of ComfyUI replicas (default: 1) |
| `--domain DOMAIN` | OAuth2 allowed email domain |
| `--email EMAIL` | OAuth2 allowed email (can specify multiple) |
| `--no-oauth` | Disable OAuth2 authentication |
| `--dry-run` | Preview resources without creating them |

Run `./provision-tenant.sh --help` for the complete list of options.

### Multi-Tenant Workflow

1. Configure the script once with your environment-specific values
2. Provision tenants as needed: `./provision-tenant.sh <tenant-name> [options]`
3. Grant users access: `oc adm groups add-users tenant-<name>-admins <username>`
4. Users can manage their ComfyUI instances via `kubectl/oc` within their namespace
5. Clean up: `kubectl delete namespace tenant-<name>`

The script includes an interactive confirmation prompt and displays all configuration before proceeding. Use `--dry-run` to preview YAML without applying changes.

**TODOS**

* Run more multi-tenant tests on a cluster 
* Test more on remote Kubernetes clusters  
* Validate functionality on GPU cluster with multiple users 














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

## Known Issues & Limitations

- **fsGroup on OpenShift 4.22+**: Pod Security Admission (PSA) replaces SCC and does not inject `fsGroup` automatically. The operator auto-detects from the namespace annotation `openshift.io/sa.scc.supplemental-groups`. If the annotation is missing, it falls back to `1000`. Set `spec.fsGroup` in the CR to override.
- **subPath volume permissions**: PVC subPath mounts are created with root ownership. The operator sets `fsGroup` on the pod security context to grant write access via group permissions.
- **Go 1.26 required**: `make install` and `make test` require Go 1.26.0+. If your system has an older version, set `export GOTOOLCHAIN=auto` or apply CRDs directly: `oc apply -f config/crd/bases/comfy.redhat.com_comfyuis.yaml`
- **TLS when pushing to internal registry**: Port-forwarding the OpenShift internal registry requires `--tls-verify=false` for podman/docker push commands.
- **OAuth2 protects filebrowser only**: The OAuth2 Proxy sidecar sits in front of filebrowser (port 8085), not ComfyUI (port 8188). ComfyUI is accessed directly without authentication.
- **Route/Gateway scheme registration**: On clusters where the Route or Gateway API group is detected but not registered in the controller's scheme, the operator skips ingress creation gracefully and logs a message.

## Design Decisions

### SubPath Mounts for Storage

The operator mounts specific subdirectories (`models`, `custom_nodes`, `user`, `output`, `input`) from the PVC using subPath mounts rather than mounting the entire PVC at `/app/ComfyUI`. This preserves the ComfyUI application code from the container image while persisting only user data. The trade-off: subPath mounts bypass Kubernetes `fsGroup` ownership changes, so new directories are created as root. The operator compensates by setting `fsGroup` on the pod security context, which grants write access via group permissions.

### fsGroup Auto-Detection

Rather than hardcoding an `fsGroup` value (bad practice — it may collide with other users on the cluster), the operator uses a three-tier approach:
1. **Explicit**: If `spec.fsGroup` is set in the CR, use it directly
2. **OpenShift auto-detect**: Read the namespace annotation `openshift.io/sa.scc.supplemental-groups` and use the base GID from the allocated range
3. **Fallback**: Default to `1000` on vanilla Kubernetes

This is necessary because OpenShift 4.22+ uses Pod Security Admission (PSA) instead of Security Context Constraints (SCCs), and PSA does not auto-inject `fsGroup` like SCCs did.

### OAuth2 Protects Filebrowser Only

The OAuth2 Proxy sidecar sits in front of filebrowser (port 8085), not ComfyUI (port 8188). This is intentional: ComfyUI's web UI uses WebSockets extensively, and adding an authenticating reverse proxy in front of it introduces connection management complexity. Filebrowser is the higher-risk surface (it provides direct filesystem access to models and outputs), so it gets authentication. ComfyUI is accessed directly via its own Route/HTTPRoute.

### Platform-Aware Ingress Discovery

The operator auto-discovers available ingress APIs at reconciliation time using the Kubernetes discovery client:
- **OpenShift Route** (preferred if available) — native to OpenShift, auto-generates hostnames
- **Gateway API HTTPRoute** (fallback) — for vanilla Kubernetes clusters with Gateway API
- **None** — ClusterIP-only, user handles ingress themselves

The operator also guards against API groups that are discovered but not registered in the controller's runtime scheme (e.g., envtest doesn't have Route CRDs). This prevents crashes in test environments.

### Owner References for Garbage Collection

All child resources (PVC, Deployment, Service, Secrets, Routes) carry an owner reference pointing to the ComfyUI CR. When the CR is deleted, Kubernetes garbage-collects all children automatically. This eliminates the need for finalizer-based cleanup and reduces the risk of orphaned resources.

### Filebrowser as a Sidecar

Filebrowser runs as a sidecar container in the same pod as ComfyUI, not as a separate Deployment. This simplifies storage sharing (both containers mount the same PVC) and ensures filebrowser is always co-located with ComfyUI. The filebrowser database is stored on an emptyDir backed by memory (`/tmp`), so it's ephemeral and doesn't need persistent storage.

## Compatibility Matrix

| Platform | Versions Tested | Notes |
|----------|----------------|-------|
| OpenShift | 4.22 | PSA-based security; fsGroup auto-detection required |
| Kubernetes | 1.27+ (Kind) | E2E tested via Kind; Gateway API requires separate installation |
| Go | 1.26.0+ | Required for building. Set `GOTOOLCHAIN=auto` if system Go is older |
| Podman | 5.x | Supported via `CONTAINER_TOOL=podman`. Kind requires inotify tuning |
| Docker | 17.03+ | Default container tool |

**OpenShift-specific behavior:**
- Routes are auto-created with TLS edge termination
- fsGroup is auto-detected from namespace annotations
- Internal registry (`image-registry.openshift-image-registry.svc:5000`) is the typical image source

**Vanilla Kubernetes behavior:**
- Gateway API HTTPRoutes are created if the Gateway API is installed
- Falls back to ClusterIP-only if no ingress API is available
- fsGroup defaults to `1000`

## Troubleshooting

### Permission denied on model directories

**Symptom:** Pod crashes with `Permission denied` on `mkdir` or write operations in `/app/ComfyUI/models`.

**Cause:** SubPath PVC mounts are created with root ownership. The container runs as a non-root user and can't write.

**Fix:** The operator sets `fsGroup` to grant group write access. If it's not working:
1. Check that the operator is running the latest version (fsGroup auto-detection was added later)
2. On OpenShift, verify the namespace has the `openshift.io/sa.scc.supplemental-groups` annotation: `oc get ns <namespace> -o yaml | grep supplemental`
3. Override manually by setting `spec.fsGroup` in the CR

### Pod stuck in ImagePullBackOff

**Symptom:** The ComfyUI pod can't pull the image from the internal registry.

**Cause:** The image doesn't exist in the registry, or the service account lacks pull permissions.

**Fix:**
1. Verify the image exists: `oc get is -n <namespace>`
2. Check the image reference matches the internal registry format: `image-registry.openshift-image-registry.svc:5000/<namespace>/<image>:<tag>`
3. Ensure the service account can pull: `oc policy add-role-to-user system:image-puller system:serviceaccount:<namespace>:default -n <source-namespace>`

### TLS errors when pushing to OpenShift internal registry

**Symptom:** `podman push` fails with certificate errors when pushing to `localhost:5000`.

**Fix:** The internal registry's TLS cert isn't valid for `localhost`. Use `--tls-verify=false`:
```sh
oc port-forward -n openshift-image-registry svc/image-registry 5000:5000 &
podman login --tls-verify=false localhost:5000 -u $(oc whoami) -p $(oc whoami -t)
podman push --tls-verify=false localhost:5000/<namespace>/<image>:<tag>
```

### E2E tests fail with "Too many open files"

**Symptom:** `make test-e2e` fails during Kind cluster creation with `Failed to create control group inotify object: Too many open files`.

**Fix:** Increase inotify limits (requires sudo):
```sh
sudo sysctl fs.inotify.max_user_watches=1048576
sudo sysctl fs.inotify.max_user_instances=8192
```

To make it permanent:
```sh
echo 'fs.inotify.max_user_watches = 1048576' | sudo tee /etc/sysctl.d/99-kind.conf
echo 'fs.inotify.max_user_instances = 8192' | sudo tee -a /etc/sysctl.d/99-kind.conf
sudo sysctl --system
```

### Go version mismatch

**Symptom:** `make test` or `make install` fails with `compile: version "go1.26.0" does not match go tool version`.

**Fix:** Your system Go is older than the project requires (1.26.0). Either:
- Set `export GOTOOLCHAIN=auto` to auto-download the right version
- Install Go 1.26.0+ from [go.dev](https://go.dev/dl/)

### OAuth2 callback URL mismatch

**Symptom:** OAuth2 login redirects to the wrong URL or returns a redirect_uri_mismatch error.

**Fix:** The operator reads the filebrowser Route's hostname to set the OAuth2 redirect URL. If the Route doesn't exist yet when the Deployment is created, the redirect URL will be missing. Delete the pod to trigger re-reconciliation:
```sh
kubectl delete pod -l app=<comfyui-name>
```
Also ensure your OAuth2 provider's authorized redirect URI matches: `https://<comfyui-name>-filebrowser-<namespace>.apps.<cluster-domain>/oauth2/callback`

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

