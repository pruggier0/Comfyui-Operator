# OpenShift Compatibility Changes

This document tracks the changes made to make the ComfyUI operator compatible with OpenShift's security requirements.

## Summary

The operator has been modified to comply with OpenShift's **restricted Security Context Constraints (SCC)**, which enforces:
- Non-root container execution
- Random UID assignment
- No privilege escalation
- Capability restrictions

## Changes Made

### 1. Added Security Contexts to Deployment Pods

**File:** `internal/controller/comfyui_controller.go`

**Pod-level security context:**
- `runAsNonRoot: true` - Ensures the pod cannot run as root
- `seccompProfile: RuntimeDefault` - Uses the default seccomp profile
- `fsGroup: 1000` - Sets filesystem group ownership for volumes (critical for PVC permissions with random UIDs)

**Container-level security context (main container):**
- `runAsNonRoot: true` - Ensures the container cannot run as root
- `allowPrivilegeEscalation: false` - Prevents privilege escalation
- `capabilities.drop: ["ALL"]` - Drops all Linux capabilities

### 2. Fixed Init Container for Non-Root Execution

**File:** `internal/controller/comfyui_controller.go`

**Changes:**
- Replaced `busybox:latest` (runs as root) with `registry.access.redhat.com/ubi9/ubi-minimal:latest`
- UBI (Universal Base Image) is Red Hat's official base image designed for OpenShift
- Added same security context to init container as main container
- Removed `chmod 777` from setup script (handled by fsGroup instead)

**File:** `config/scripts/comfyui-setup-configmap.yaml`

**Changes:**
- Removed `chmod -R 777` command (incompatible with OpenShift and unnecessary with fsGroup)
- Updated comments to explain that permissions are handled by pod security context

### 3. Made Service Type Configurable

**File:** `api/v1alpha1/comfyui_types.go`

**Changes:**
- Added `ServiceType` field to `ComfyUISpec`
- Defaults to `ClusterIP` (OpenShift best practice)
- Supports `ClusterIP`, `NodePort`, and `LoadBalancer` via kubebuilder validation
- Removed hard-coded `NodePort` service type

**File:** `internal/controller/comfyui_controller.go`

**Changes:**
- Updated `reconcileService()` to use `comfyui.Spec.ServiceType` instead of hard-coded value
- Falls back to `ClusterIP` if not specified

### 4. ConfigMap Approach

**Decision:** Using static ConfigMap deployment (Option 2)

The `comfyui-setup-scripts` ConfigMap is deployed as a static resource rather than being reconciled by the operator. This:
- Simplifies operator code
- Allows manual ConfigMap updates without operator changes
- Is deployed once and shared across ComfyUI instances in the same namespace

**Installation requirement:**
```bash
kubectl apply -f config/scripts/comfyui-setup-configmap.yaml
```

## Testing Checklist

Before deploying to OpenShift:

- [ ] Verify CRD installs successfully: `oc apply -f config/crd/bases/`
- [ ] Deploy static ConfigMap: `oc apply -f config/scripts/comfyui-setup-configmap.yaml`
- [ ] Build and push operator image to accessible registry
- [ ] Deploy operator with proper RBAC
- [ ] Create test ComfyUI CR with storage configured
- [ ] Verify pods run without SCC violations
- [ ] Verify PVC permissions work correctly with random UID
- [ ] Verify init container completes successfully
- [ ] Verify main ComfyUI container starts and serves traffic

## Known Limitations

1. **ConfigMap must be manually deployed** - Users must remember to deploy the setup scripts ConfigMap before creating ComfyUI instances
2. **No Route support yet** - For external access, users must manually create OpenShift Routes pointing to the Service
3. **Storage class must support fsGroup** - The storage class must honor fsGroup for volume permissions (most do)

## Future Improvements

1. Add OpenShift Route reconciliation for automatic external access
2. Create a custom ComfyUI container image with pre-configured directory structure
3. Add validation webhook to check ConfigMap exists before allowing ComfyUI CR creation
4. Support for OpenShift-specific annotations and labels
5. Integration with OpenShift's image streams
6. Support for OpenShift OAuth proxy for authentication

## References

- [OpenShift Security Context Constraints](https://docs.openshift.com/container-platform/latest/authentication/managing-security-context-constraints.html)
- [Kubernetes Security Contexts](https://kubernetes.io/docs/tasks/configure-pod-container/security-context/)
- [Red Hat Universal Base Images](https://developers.redhat.com/products/rhel/ubi)
