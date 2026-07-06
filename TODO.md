# ComfyUI Operator - TODO: Get UI Running in Browser

## Current Status

**Completed:**
- ✅ Step 1: CRD Definition
- ✅ Step 2: Main Reconcile Loop
- ✅ Step 3: Deployment Reconciliation
- ✅ Step 4: Service Reconciliation (NodePort)

**In Progress:**
- 🚧 Step 6: PVC Integration

**Current State:**
- Basic operator works (creates Deployment + Service)
- Using pause image for testing
- PVC reconciliation code exists but has bugs
- NOT YET TESTED with real ComfyUI image

---

## Step 6: PVC Integration (CURRENT WORK)

### 6.1 Fix PVC Reconciliation Bugs ⚠️ CRITICAL

**File:** `internal/controller/comfyui_controller.go`

**Issues to fix:**

1. **Logic bug in ReconcilePVC() (lines 92-94)**
   ```go
   if comfyui.Spec.Storage == nil {
       log.Info("No storage specified, skipping PVC creation")
       // MISSING: return nil
   }
   // Code keeps running even when storage is nil!
   ```
   **Fix:** Add `return nil` OR move check before `r.Get()` call

2. **Missing error propagation (lines 96-109)**
   ```go
   if err != nil {
       if errors.IsNotFound(err) {
           // create logic
       }
       // BUG: Other errors (timeout, API error) are ignored!
   }
   return nil  // Always returns success
   ```
   **Fix:** Add `return err` after the `IsNotFound` block

3. **Missing PVC binding check**
   - After PVC exists, need to check if `pvc.Status.Phase == corev1.ClaimBound`
   - If not bound, return error: `"PVC not bound yet, waiting for binding"`
   - This will trigger requeue until PVC is ready

4. **Add logging**
   - Log when creating PVC
   - Log when PVC is bound
   - Log when skipping (no storage)

**Tasks:**
- [ ] Move `if comfyui.Spec.Storage == nil` check to START of ReconcilePVC()
- [ ] Add `return nil` after the nil check
- [ ] Add `return err` after IsNotFound block
- [ ] Add binding check after successful Get
- [ ] Add log.Info statements for all code paths

---

### 6.2 Add RBAC for PVCs

**File:** `internal/controller/comfyui_controller.go` (around line 274)

**Add RBAC marker:**
```go
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
```

**Then run:**
```bash
make manifests  # Regenerate RBAC
```

**Tasks:**
- [ ] Add RBAC marker for PVCs
- [ ] Run `make manifests`
- [ ] Verify `config/rbac/role.yaml` was updated

---

### 6.3 Wire PVC into Reconcile Loop

**File:** `internal/controller/comfyui_controller.go` (lines 285-317)

**Current order (WRONG):**
```go
// 2. Reconcile Deployment
// 3. Reconcile Service
// 4. Reconcile PVC  ← TOO LATE!
```

**Correct order:**
```go
// 2. Reconcile PVC FIRST (before deployment needs it)
if err := r.ReconcilePVC(ctx, comfyui); err != nil {
    log.Error(err, "Failed to reconcile PVC")
    return ctrl.Result{}, err
}

// 3. Reconcile Deployment (can now mount the PVC)
if err := r.reconcileDeployment(ctx, comfyui); err != nil {
    ...
}

// 4. Reconcile Service
```

**Why:** Deployment needs PVC to exist before it can mount it.

**Tasks:**
- [ ] Move PVC reconciliation BEFORE deployment reconciliation
- [ ] Update comments to reflect new order

---

### 6.4 Mount PVC to Deployment

**File:** `internal/controller/comfyui_controller.go` in `reconcileDeployment()` function

**Add this code after resources check (around line 173):**

```go
// Add resources if specified in the CR
if comfyui.Spec.Resources != nil {
    deployment.Spec.Template.Spec.Containers[0].Resources = *comfyui.Spec.Resources
}

// ========== ADD THIS SECTION ==========
// If PVC configured, mount it to the deployment
if comfyui.Spec.Storage != nil {
    pvcName := fmt.Sprintf("%s-storage", comfyui.Name)
    
    // Add volume referencing the PVC
    deployment.Spec.Template.Spec.Volumes = []corev1.Volume{
        {
            Name: "data",
            VolumeSource: corev1.VolumeSource{
                PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
                    ClaimName: pvcName,
                },
            },
        },
    }
    
    // Add volume mount to container
    deployment.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
        {
            Name:      "data",
            MountPath: "/app/ComfyUI/models",
        },
    }
}
// ========== END NEW SECTION ==========

// Step 3: Check if Deployment already exists
found := &appsv1.Deployment{}
// ... rest of function
```

**Tasks:**
- [ ] Add Volume for PVC (if storage configured)
- [ ] Add VolumeMount to container at `/app/ComfyUI/models`
- [ ] Test that it compiles: `go build ./internal/controller/`

---

### 6.5 Add Init Container for Directory Setup

**DECISION NEEDED:** ConfigMap approach (recommended) vs Script-in-Image

#### Option A: ConfigMap (Recommended for Distribution)

**Why:** Works with any ComfyUI image, easy to customize, no registry dependency

**Step 1:** Create `config/scripts/comfyui-setup-configmap.yaml`:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: comfyui-setup-scripts
  namespace: comfyui-operator-system
data:
  setup-storage.sh: |
    #!/bin/sh
    set -e
    echo "=== ComfyUI Storage Setup ==="
    mkdir -p /data/models/checkpoints \
             /data/models/vae \
             /data/models/loras \
             /data/models/embeddings \
             /data/models/controlnet \
             /data/models/clip \
             /data/models/clip_vision \
             /data/models/unet \
             /data/models/diffusion_models \
             /data/models/text_encoders \
             /data/models/upscale_models \
             /data/models/hypernetworks \
             /data/models/style_models \
             /data/output \
             /data/input \
             /data/temp
    chmod -R 777 /data
    echo "✓ Storage setup complete!"
```

**Step 2:** Add to `config/default/kustomization.yaml`:
```yaml
resources:
- ../crd
- ../rbac
- ../manager
- ../scripts/comfyui-setup-configmap.yaml  # ADD THIS
```

**Step 3:** Update `reconcileDeployment()` to add init container:
```go
// If PVC configured, mount it and set up directories
if comfyui.Spec.Storage != nil {
    pvcName := fmt.Sprintf("%s-storage", comfyui.Name)
    
    // Add init container that runs setup script
    deployment.Spec.Template.Spec.InitContainers = []corev1.Container{
        {
            Name:  "setup-storage",
            Image: "busybox:latest",
            Command: []string{"sh", "/scripts/setup-storage.sh"},
            VolumeMounts: []corev1.VolumeMount{
                {Name: "data", MountPath: "/data"},
                {Name: "setup-scripts", MountPath: "/scripts", ReadOnly: true},
            },
        },
    }
    
    // Add volumes: PVC + ConfigMap
    deployment.Spec.Template.Spec.Volumes = []corev1.Volume{
        {
            Name: "data",
            VolumeSource: corev1.VolumeSource{
                PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
                    ClaimName: pvcName,
                },
            },
        },
        {
            Name: "setup-scripts",
            VolumeSource: corev1.VolumeSource{
                ConfigMap: &corev1.ConfigMapVolumeSource{
                    LocalObjectReference: corev1.LocalObjectReference{
                        Name: "comfyui-setup-scripts",
                    },
                    DefaultMode: ptr.To(int32(0755)),
                },
            },
        },
    }
    
    // Main container mounts PVC
    deployment.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
        {Name: "data", MountPath: "/app/ComfyUI/models"},
    }
}
```

**Note:** Need to import `k8s.io/utils/ptr` for `ptr.To()`

#### Option B: Script-in-Image (Simpler but Less Flexible)

**Step 1:** Create `~/ComfyUI/setup-storage.sh`

**Step 2:** Update `~/ComfyUI/Containerfile`:
```dockerfile
COPY setup-storage.sh /usr/local/bin/setup-storage.sh
RUN chmod +x /usr/local/bin/setup-storage.sh
```

**Step 3:** Use your image in init container:
```go
InitContainers: []corev1.Container{
    {
        Name:  "setup-storage",
        Image: comfyui.Spec.Image,  // Same as main container
        Command: []string{"/usr/local/bin/setup-storage.sh"},
        VolumeMounts: []corev1.VolumeMount{
            {Name: "data", MountPath: "/data"},
        },
    },
}
```

**Tasks (Option A - ConfigMap):**
- [ ] Create `config/scripts/` directory
- [ ] Create `comfyui-setup-configmap.yaml`
- [ ] Update `config/default/kustomization.yaml`
- [ ] Add init container to `reconcileDeployment()`
- [ ] Add `k8s.io/utils/ptr` import

**Tasks (Option B - Script-in-Image):**
- [ ] Create setup script in `~/ComfyUI/setup-storage.sh`
- [ ] Update `~/ComfyUI/Containerfile`
- [ ] Build new image
- [ ] Add init container using custom image

---

## Step 7: Build & Deploy Real ComfyUI Image

### 7.1 Build Custom Image

**Location:** `~/ComfyUI/Containerfile`

**Current status:** Containerfile exists with:
- Python 3.13-slim
- ComfyUI cloned from GitHub
- ComfyUI Manager
- PyTorch with CUDA 12.1
- Exposed port 8188

**Commands:**
```bash
# Build image
cd ~/ComfyUI
docker build -t comfyui-custom:v1 .

# Load into minikube (since we're using minikube)
minikube image load comfyui-custom:v1

# Verify it's loaded
minikube image ls | grep comfyui
```

**Tasks:**
- [ ] (Optional) Add setup-storage.sh to Containerfile if using Option B
- [ ] Build image: `docker build -t comfyui-custom:v1 ~/ComfyUI/`
- [ ] Load to minikube: `minikube image load comfyui-custom:v1`
- [ ] Verify: `minikube image ls | grep comfyui`

---

### 7.2 Update CR to Use Real Image

**File:** `config/samples/comfy_v1alpha1_comfyui_simple.yaml`

**Change from:**
```yaml
spec:
  image: registry.k8s.io/pause:3.9
  replicas: 1
```

**To:**
```yaml
spec:
  image: comfyui-custom:v1
  replicas: 1
  storage:
    size: 10Gi
    accessMode: ReadWriteOnce
```

**Tasks:**
- [ ] Update image to `comfyui-custom:v1`
- [ ] Add storage spec
- [ ] Remove old comments about pause image

---

## Step 8: Deploy & Test

### 8.1 Redeploy Operator

**Commands:**
```bash
# If controller running locally, stop it (Ctrl+C)

# Delete old ComfyUI instance
kubectl delete comfyui comfyui-test

# Update CRDs (in case anything changed)
make install

# Start controller
make run
```

**Tasks:**
- [ ] Stop running controller
- [ ] Delete old deployment: `kubectl delete comfyui comfyui-test`
- [ ] Run `make install`
- [ ] Run `make run`

---

### 8.2 Create ComfyUI Instance

**In new terminal:**

```bash
# Apply the CR
kubectl apply -f config/samples/comfy_v1alpha1_comfyui_simple.yaml

# Watch controller logs (in make run terminal)
# Look for:
#   - "Creating PVC"
#   - "PVC is bound and ready"
#   - "Creating Deployment"
#   - "Creating Service"

# Check PVC status
kubectl get pvc
# Should show: comfyui-test-storage   Bound   ...   10Gi

# Check pods
kubectl get pods
# Should show: comfyui-test-xxx-xxx   0/1  Init:0/1  (init container running)
# Then:        comfyui-test-xxx-xxx   1/1  Running   (main container running)

# Check init container logs
kubectl describe pod comfyui-test-<pod-id>
# Look for: Init Containers: setup-storage: Completed

# Check main container logs
kubectl logs -f comfyui-test-<pod-id>
# Should see ComfyUI startup logs
```

**Tasks:**
- [ ] Apply CR
- [ ] Verify PVC created and bound
- [ ] Verify init container completed
- [ ] Verify pod is Running (not CrashLoopBackOff)
- [ ] Check logs for errors

---

### 8.3 Access in Browser

**Commands:**
```bash
# Get service info
kubectl get svc comfyui-test
# Note the NodePort (e.g., 30123)

# Get minikube IP
minikube ip
# Example output: 10.0.2.15

# Open browser
firefox http://10.0.2.15:30123
# OR
firefox http://$(minikube ip):$(kubectl get svc comfyui-test -o jsonpath='{.spec.ports[0].nodePort}')
```

**Expected:** ComfyUI web interface loads in browser!

**Tasks:**
- [ ] Get NodePort from service
- [ ] Get minikube IP
- [ ] Open `http://<ip>:<port>` in browser
- [ ] Verify ComfyUI UI loads
- [ ] Try loading a workflow
- [ ] Generate a test image

---

## Verification Checklist

After completing all steps, verify:

**PVC:**
- [ ] PVC created: `kubectl get pvc`
- [ ] PVC bound (not Pending)
- [ ] PVC has correct size (10Gi)

**Storage Setup:**
- [ ] Init container ran: `kubectl describe pod comfyui-test-xxx` shows "Completed"
- [ ] Directories exist: `kubectl exec -it comfyui-test-xxx -- ls /app/ComfyUI/models`
- [ ] Subdirectories exist: `kubectl exec -it comfyui-test-xxx -- ls /app/ComfyUI/models/checkpoints`

**Pod Health:**
- [ ] Pod is Running: `kubectl get pods`
- [ ] No restarts: `kubectl get pods` shows RESTARTS = 0
- [ ] Logs show ComfyUI started: `kubectl logs comfyui-test-xxx`

**Service:**
- [ ] Service exists: `kubectl get svc comfyui-test`
- [ ] Endpoints populated: `kubectl get endpoints comfyui-test`
- [ ] NodePort assigned

**Browser Access:**
- [ ] UI loads at `http://<minikube-ip>:<nodePort>`
- [ ] Can interact with UI
- [ ] Can load/save workflows

**Persistence:**
- [ ] Upload a model to UI (or touch a file)
- [ ] Delete pod: `kubectl delete pod comfyui-test-xxx`
- [ ] Wait for new pod to start
- [ ] Verify file still exists in new pod
- [ ] Verify UI still works

---

## Known Issues & Gotchas

**Issue 1: PVC Stuck in Pending**
- **Symptom:** `kubectl get pvc` shows "Pending"
- **Cause:** No default StorageClass or PV available
- **Fix (minikube):** Enable addon: `minikube addons enable default-storageclass`

**Issue 2: Pod CrashLoopBackOff**
- **Symptom:** Pod keeps restarting
- **Debug:** `kubectl logs comfyui-test-xxx` and `kubectl describe pod comfyui-test-xxx`
- **Common causes:**
  - Missing model directories (init container didn't run)
  - ComfyUI can't bind to port 8188 (check container logs)
  - Out of memory (check resources)

**Issue 3: Can't Access UI in Browser**
- **Symptom:** Connection refused or timeout
- **Debug:**
  - Verify pod is running: `kubectl get pods`
  - Verify endpoints: `kubectl get endpoints comfyui-test`
  - Check minikube IP is correct: `minikube ip`
  - Try port-forward instead: `kubectl port-forward svc/comfyui-test 8188:8188`
  - Then access: `http://localhost:8188`

**Issue 4: Init Container Fails**
- **Symptom:** Pod stuck in Init:Error or Init:CrashLoopBackOff
- **Debug:** `kubectl logs comfyui-test-xxx -c setup-storage`
- **Common causes:**
  - ConfigMap not found (forgot to deploy it)
  - Script has syntax error
  - Permissions issue

**Issue 5: Image Pull Error**
- **Symptom:** `kubectl describe pod` shows "ImagePullBackOff"
- **Cause:** Minikube can't find `comfyui-custom:v1`
- **Fix:** 
  - Verify image loaded: `minikube image ls | grep comfyui`
  - Reload if needed: `minikube image load comfyui-custom:v1`
  - Or build inside minikube: `eval $(minikube docker-env) && docker build -t comfyui-custom:v1 ~/ComfyUI/`

---

## Next Steps After UI is Working

**Step 5: Status Updates** (deferred from earlier)
- Update `status.phase` based on pod readiness
- Compute and set `status.endpoint`
- Update replica counts
- Set conditions (Ready, DeploymentReady, ServiceReady)

**Step 7: Update Handling**
- Detect when CR spec changes
- Smart update vs always-update
- Handle image/replicas/resources changes

**Step 8: Error Handling & Requeue**
- Implement `isTransientError()` helper
- Return proper requeue results
- Update status to Failed on permanent errors

**Step 9: HuggingFace Model Downloads**
- Add init containers for model downloads
- Use `huggingface-cli` to download models to PVC
- Support token authentication

**Step 10: Watch Owned Resources**
- Update `SetupWithManager()` to watch Deployments, Services, PVCs
- Enable self-healing (recreate if manually deleted)

**Step 11: Finalizers**
- Add finalizer on creation
- Handle cleanup on deletion
- Remove finalizer after cleanup

---

## Current Blocker

**NEXT ACTION:** Choose init container approach
- **Option A (ConfigMap):** Recommended for easy distribution, works with any image
- **Option B (Script-in-Image):** Simpler, but locks users to your image

Then implement Step 6.1-6.5.

**Estimated time to working UI:** 1-2 hours if no issues

---

## Collaboration Notes

**From CLAUDE.md:** Never be a yes-man, offer constructive criticism, suggest improvements with multiple reasons.

**Design decisions pending:**
1. **Init container approach:** ConfigMap vs Script-in-Image (discussed, leaning toward ConfigMap)
2. **Mount point:** Single `/app/ComfyUI/models` or multiple (models/, output/, input/)?
3. **Default storage size:** 10Gi? 50Gi? Make it configurable?

**Good patterns to follow:**
- Incremental development (test each step)
- Immediate feedback (verify before moving on)
- Declarative configuration (avoid hardcoding)
- Kubernetes-native solutions (ConfigMaps, not bash scripts outside K8s)
