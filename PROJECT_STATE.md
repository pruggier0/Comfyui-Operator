# ComfyUI Operator - Project State

**Last Updated:** 2026-06-22

## Overview

A Kubernetes operator for deploying and managing ComfyUI instances in Kubernetes/OpenShift clusters. Built with Kubebuilder and controller-runtime.

## Project Status

### ✅ Implemented Features

**Core Operator Functionality:**
- Custom Resource Definition (CRD) `ComfyUI` v1alpha1
- Modular controller architecture split across multiple files
- Full reconciliation loop for deployments, services, storage, and ingress
- PersistentVolumeClaim creation and lifecycle management
- Owner references for automatic garbage collection
- Status tracking with phases (Pending, Initializing, Running, Failed, Unknown)

**Deployment Management:**
- Configurable replicas, resources, and node selectors
- GPU support (optional, via `enableGPU` and `gpuCount` spec fields)
- Comprehensive volume mounting for ComfyUI directories (models, custom_nodes, user, output, input, temp)
- **Filebrowser sidecar container** for web-based file management
  - Shares the same PVC as ComfyUI
  - No authentication required (`--noauth` flag)
  - Accessible on port 8080
  - Allows drag-and-drop file uploads to the container

**Storage:**
- Automatic PVC provisioning with configurable size, storage class, and access mode
- SubPath mounting for organized data structure on single PVC
- Filebrowser mounts entire PVC at `/data` for full filesystem access

**Networking:**
- Service creation with configurable ServiceType (ClusterIP, NodePort, LoadBalancer)
- **Dual-protocol ingress support:**
  - **OpenShift Routes** (TLS edge termination, auto-detected via API discovery)
  - **Kubernetes Gateway API v1** (HTTPRoute, configurable Gateway name/namespace/hostname)
  - Automatic fallback: Route → Gateway API → None
- Separate service ports:
  - **8188**: ComfyUI web interface
  - **8080**: Filebrowser sidecar

**Gateway API Integration:**
- Configurable via `spec.gateway` field in ComfyUI CR
- User can specify Gateway name, namespace, and hostname
- Defaults: `default-gateway` in `gateway-system` namespace
- HTTPRoute automatically created with proper parent references

**API Discovery:**
- Runtime detection of available APIs (Route, Gateway API)
- Graceful degradation when APIs are not available

### 🚧 Current Development State

**Testing Environment:**
- **Minikube** with qemu driver (local CPU-only testing)
- **Disk:** 35GB, currently 89% full (3.7GB free)
  - 20GB: Docker storage overhead (normal)
  - 5.9GB: PVC data (ComfyUI)
  - 3.1GB: Docker images
- **Gateway API:** v1.2.1 installed with test Gateway/GatewayClass
- **Operator:** Running locally outside cluster (`./bin/manager`)

**Custom ComfyUI Image:**
- **Location:** `test-image/Containerfile.cpu`
- **Base:** `python:3.12-slim`
- **Type:** CPU-only (PyTorch CPU builds)
- **Size:** 2.65GB
- **Runs as:** root (non-root configs commented out for testing)
  - **TODO:** Re-enable `RunAsNonRoot: true` for production/OpenShift
- **Installed:**
  - ComfyUI from GitHub
  - ComfyUI-Manager (standalone) cloned to custom_nodes at runtime
  - Built-in manager disabled (`--enable-manager` flag removed)
- **Filebrowser:** Removed from image (now runs as sidecar)
- **Image Name:** `comfyui-cpu:latest` (loaded into minikube)

**ComfyUI Manager:**
- Using **standalone custom node version** from https://github.com/ltdrdata/ComfyUI-Manager
- Cloned into PVC at `/app/ComfyUI/custom_nodes/ComfyUI-Manager` on first startup
- Built-in manager intentionally disabled to avoid conflicts
- **Known Issue:** Custom node download fails with channel errors (non-blocking)
- **Working:** Model downloads from Hugging Face, CivitAI, etc.

## Architecture

### Controller Structure

**Main Controller:** `internal/controller/comfyui_controller.go`
- Orchestrates reconciliation flow
- Calls sub-reconcilers in sequence: PVC → Deployment → Service → Ingress

**Modular Reconcilers:**
- `storage.go`: PVC creation, volume mounting
- `deployment.go`: Deployment and pod spec configuration
- `service.go`: Service creation with dual ports
- `route.go`: OpenShift Route reconciliation
- `gateway.go`: Gateway API HTTPRoute reconciliation
- `discovery.go`: API discovery and ingress reconciliation with fallback logic

### Container Architecture

**Pod Structure:** 2 containers
1. **comfyui** (main container)
   - Image: `comfyui-cpu:latest`
   - Port 8188 (http)
   - Mounts: Specific subdirectories via subPath (models, custom_nodes, user, output, input, temp)

2. **filebrowser** (sidecar)
   - Image: `filebrowser/filebrowser:latest`
   - Port 8080 (filebrowser)
   - Mount: Entire PVC at `/data`
   - Args: `--noauth --port=8080 --address=0.0.0.0 --root=/data`

### Storage Layout

**PVC:** `{comfyui-name}-storage`
```
/data/
├── models/
│   ├── checkpoints/
│   ├── vae/
│   ├── loras/
│   ├── embeddings/
│   ├── controlnet/
│   ├── clip/
│   ├── clip_vision/
│   ├── unet/
│   └── upscale_models/
├── custom_nodes/
│   └── ComfyUI-Manager/  (cloned at runtime)
├── user/
├── output/
├── input/
└── temp/
```

## Technical Decisions & Rationale

### Security Context (Temporary Override)

**Current State:**
- `RunAsNonRoot: true` is **commented out** in `deployment.go`
- Pod runs as **root** for testing

**Rationale:**
- Community ComfyUI images run as root
- OpenShift requires non-root for production
- TODO: Build production-ready non-root image before deploying to OpenShift

**Production Requirements:**
- Re-enable `RunAsNonRoot: true` in pod and container security contexts
- Build custom image with `USER 1000`
- Set `fsGroup: 1000` for volume permissions

### Filebrowser as Sidecar

**Decision:** Filebrowser runs as a separate container, not embedded in the ComfyUI image.

**Rationale:**
- **Separation of concerns:** ComfyUI and file management are distinct functions
- **Resource isolation:** Dedicated CPU/memory limits for filebrowser
- **Easier updates:** Update filebrowser independently of ComfyUI
- **Shared storage:** Both containers mount the same PVC with different strategies
  - ComfyUI: SubPath mounts for specific directories
  - Filebrowser: Full PVC mount for complete visibility

**Tradeoff:** Slightly more complex deployment spec, but cleaner architecture.

### Built-in Manager vs Standalone

**Decision:** Disable built-in ComfyUI Manager, use standalone custom node version.

**Rationale:**
- User requested "legacy Manager UI" option
- Built-in manager **blocks** custom node version (policy conflict)
- Cannot run both simultaneously

**Current Behavior:**
- ComfyUI starts without `--enable-manager` flag
- Standalone ComfyUI-Manager loads as custom node
- **Known bug:** Channel URL errors in background thread (non-blocking)
- Model downloads work; custom node installs may be unreliable

### File Upload Limitation

**Browser Security Constraint:**
When ComfyUI's file upload button is clicked, the browser **always** opens the local filesystem picker. This cannot be changed.

**Workarounds:**
1. **ComfyUI Manager:** Download models directly from internet sources
2. **Filebrowser:** Upload files from local machine via web UI (drag-and-drop)
3. **Future:** Consider custom ComfyUI node with server-side file picker

**User Goal:** "Live entirely in the container" - currently achievable via ComfyUI Manager for downloads.

### Gateway API Configuration

**Decision:** Make Gateway name, namespace, and hostname configurable in CR spec.

**API Design:**
```yaml
spec:
  gateway:
    name: "my-gateway"           # Default: "default-gateway"
    namespace: "gateway-system"  # Default: "gateway-system"
    hostname: "comfyui.example.com"  # Default: "{name}.example.com"
```

**Rationale:**
- Different environments may use different Gateway resources
- Hostnames must be unique per deployment
- Avoids hardcoded values in operator code

## Current Issues & TODOs

### Known Issues

1. **ComfyUI-Manager custom node downloads fail**
   - Error: `InvalidChannel` for custom node list
   - Model downloads work fine
   - Non-blocking error in background thread

2. **Disk space tight on minikube**
   - 89% full (3.7GB free)
   - Large models (2-7GB) will cause issues
   - Solution: Recreate minikube with `--disk-size=60g`

3. **kubectl port-forward unstable**
   - Dies on network interruptions, idle timeouts
   - Not suitable for long-term use
   - Solution: Use Routes/Gateway API in production

4. **RunAsNonRoot disabled for testing**
   - Required for OpenShift compatibility
   - Must re-enable before production deployment

### TODOs

**High Priority:**
- [ ] Build production non-root ComfyUI image
- [ ] Re-enable security contexts (`RunAsNonRoot: true`)
- [ ] Test on actual OpenShift cluster
- [ ] Implement status conditions properly (currently minimal)
- [ ] Add status.endpoint URL computation

**Medium Priority:**
- [ ] Add Hugging Face model download init containers (from spec.models)
- [ ] Implement proper status phase transitions
- [ ] Add integration tests
- [ ] Document deployment on OpenShift

**Low Priority / Future:**
- [ ] Auto-restart for port-forward (dev convenience)
- [ ] Custom ComfyUI node for server-side file browsing
- [ ] Metrics/monitoring support
- [ ] Multi-replica support with shared storage considerations

## Development Workflow

### Local Development

```bash
# Install CRDs
make install

# Run operator locally
GOTOOLCHAIN=auto ./bin/manager

# Build operator
GOTOOLCHAIN=auto make build

# Build custom ComfyUI image
cd test-image
podman build -f Containerfile.cpu -t comfyui-cpu:latest .

# Load image into minikube
podman save localhost/comfyui-cpu:latest | minikube image load -
minikube ssh -- 'docker tag localhost/comfyui-cpu:latest comfyui-cpu:latest'

# Apply sample CR
kubectl apply -f config/samples/comfy_v1alpha1_comfyui.yaml

# Port forward for testing
kubectl port-forward svc/comfyui-cpu 8188:8188 -n default &  # ComfyUI
kubectl port-forward svc/comfyui-cpu 8080:8080 -n default &  # Filebrowser
```

### Minikube Setup

```bash
# Create cluster (qemu driver for rootless podman compatibility)
minikube start --driver=qemu --disk-size=40g

# Install Gateway API CRDs
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.1/standard-install.yaml

# Create test Gateway
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: test-gateway-class
spec:
  controllerName: example.com/gateway-controller
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: test-gateway
  namespace: default
spec:
  gatewayClassName: test-gateway-class
  listeners:
  - name: http
    protocol: HTTP
    port: 80
EOF
```

### Troubleshooting

**Operator not reconciling:**
- Check if operator is running: `ps aux | grep manager`
- Check logs: `tail -f /tmp/operator.log`
- Restart: `pkill -9 manager && ./bin/manager > /tmp/operator.log 2>&1 &`

**Pod stuck in Error/CrashLoopBackOff:**
- Check logs: `kubectl logs -l app=comfyui-cpu -n default -c comfyui`
- Check events: `kubectl describe pod -l app=comfyui-cpu -n default`

**Port-forward died:**
- Restart: `pkill -f "port-forward" && kubectl port-forward svc/comfyui-cpu 8188:8188 8080:8080 -n default &`

**Out of disk space:**
- Check usage: `minikube ssh -- 'df -h /mnt/sda1'`
- Clean images: `minikube ssh -- 'docker image prune -a -f'`
- Recreate with more space: `minikube delete && minikube start --driver=qemu --disk-size=60g`

## Dependencies

**Runtime:**
- Go 1.26.0+ (with `GOTOOLCHAIN=auto`)
- Kubernetes 1.35.1 (API v0.36.0)
- controller-runtime v0.24.1
- Gateway API v1.2.1 (optional)
- OpenShift Route API (optional)

**Development:**
- Kubebuilder
- Minikube
- Podman (rootless)

**Container Images:**
- `python:3.12-slim` (base for ComfyUI)
- `filebrowser/filebrowser:latest` (sidecar)

## Key Files

**Operator Code:**
- `api/v1alpha1/comfyui_types.go` - CRD definition
- `internal/controller/comfyui_controller.go` - Main reconciliation loop
- `internal/controller/storage.go` - PVC and volume management
- `internal/controller/deployment.go` - Deployment and sidecar configuration
- `internal/controller/service.go` - Service with dual ports
- `internal/controller/route.go` - OpenShift Route support
- `internal/controller/gateway.go` - Gateway API HTTPRoute support
- `internal/controller/discovery.go` - API discovery and fallback logic
- `cmd/main.go` - Operator entry point, scheme registration

**Custom Image:**
- `test-image/Containerfile.cpu` - CPU-only ComfyUI image

**Samples:**
- `config/samples/comfy_v1alpha1_comfyui.yaml` - Example ComfyUI CR

**Generated:**
- `config/crd/bases/` - Generated CRD manifests
- `bin/manager` - Compiled operator binary

## Testing Access

When running locally with minikube:

- **ComfyUI**: http://localhost:8188 (via port-forward)
- **FileBrowser**: http://localhost:8080 (via port-forward)

**FileBrowser Usage:**
1. Open http://localhost:8080
2. Navigate to desired directory (e.g., `models/checkpoints`)
3. Click "Upload" button
4. Drag and drop files
5. Files persist on PVC and are immediately visible to ComfyUI

## Production Deployment Considerations

**Before deploying to OpenShift:**

1. **Re-enable security contexts** in `deployment.go`
2. **Build non-root image** with `USER 1000`
3. **Test with restricted SCC** (OpenShift's default security policy)
4. **Configure proper Route** with TLS certificates
5. **Set resource limits** appropriate for workload
6. **Consider persistent storage class** (e.g., AWS EBS, Ceph RBD)
7. **Document GPU node setup** if using GPU-enabled deployments

**Resource Recommendations:**
- **ComfyUI container**: 4-8 CPU, 8-16GB RAM (CPU mode), add GPU if enabled
- **Filebrowser sidecar**: 100m CPU, 128Mi RAM (current limits: 200m CPU, 256Mi RAM)
- **PVC**: Start with 50Gi, monitor actual model storage needs

## Collaboration Notes

This project was developed collaboratively with Claude as a coding partner. Key principles:

- **Constructive criticism encouraged** - Claude challenges bad ideas and suggests better alternatives
- **Multiple reasons for decisions** - Both problems and solutions explained with technical rationale
- **Pair programming approach** - Focus on creating the best software, not just "making it work"
- **No yes-man behavior** - Push back on approaches with clear downsides

See `CLAUDE.md` for full collaboration guidelines.
