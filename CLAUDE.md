# ComfyUI Operator

A Kubernetes operator for managing ComfyUI deployments, built with Kubebuilder and controller-runtime.

## Project Overview

This operator automates the deployment and lifecycle management of ComfyUI instances in Kubernetes clusters. It provides:

- **Custom Resource Definition (CRD)**: `ComfyUI` v1alpha1 for declarative ComfyUI deployments
- **Automated Storage Management**: PersistentVolumeClaim provisioning and lifecycle management
- **Model Management**: Integrated Hugging Face model downloads with support for various model types (checkpoints, VAE, LoRA, embeddings, ControlNet, CLIP)
- **Resource Configuration**: Flexible resource limits, replicas, and volume management
- **Status Tracking**: Phase-based lifecycle tracking (Pending, Initializing, Running, Failed, Unknown)

## Architecture

### API Structure (`api/v1alpha1/`)

**ComfyUISpec** - Desired state definition:
- `Image`: Container image for ComfyUI
- `Replicas`: Number of pod replicas
- `Resources`: Kubernetes resource requirements
- `Storage`: PVC configuration (size, storage class, access mode)
- `Models`: Hugging Face model download configuration
- `VolumeMounts` / `Volumes`: Additional volume configuration

**ComfyUIStatus** - Observed state:
- `Phase`: Current lifecycle phase
- `Conditions`: Standard Kubernetes condition tracking
- `Replicas` / `ReadyReplicas`: Pod counts
- `Endpoint`: Access URL for the ComfyUI web interface
- `ObservedGeneration`: Resource version tracking

### Controller (`internal/controller/`)

**ComfyUIReconciler** - Main reconciliation loop
- Currently implements PVC reconciliation logic
- Validates storage specifications and creates PVCs with proper owner references
- **TODO**: Full reconciliation loop implementation for deployments, services, and status updates

## Technology Stack

- **Go**: 1.24.0+
- **Kubernetes**: 1.11.3+ (API v0.33.0)
- **controller-runtime**: v0.21.0
- **Testing**: Ginkgo v2 + Gomega

## Development Workflow

### Local Development

```bash
# Install CRDs
make install

# Run controller locally (against current kubectl context)
make run

# Run tests
make test

# Build binary
make build
```

### Building & Deploying

```bash
# Build and push image
make docker-build docker-push IMG=<registry>/comfyui-operator:tag

# Deploy to cluster
make deploy IMG=<registry>/comfyui-operator:tag

# Apply sample CRs
kubectl apply -k config/samples/
```

### Code Generation

After modifying `api/v1alpha1/comfyui_types.go`:

```bash
make generate  # Regenerate deepcopy methods
make manifests # Regenerate CRD manifests
```

## Project Structure

```
.
├── api/v1alpha1/          # API definitions and types
├── cmd/                   # Main entry point
├── config/                # Kubernetes manifests (CRDs, RBAC, samples)
├── internal/controller/   # Reconciliation logic
├── hack/                  # Build scripts
└── test/                  # Integration tests
```

## Current State

### Implemented
- CRD schema with comprehensive spec and status fields
- PVC creation and reconciliation
- Storage validation and error handling
- Owner reference management for garbage collection

### In Progress / TODO
- Main reconciliation loop implementation
- Deployment creation and management
- Service/Ingress provisioning for web access
- Hugging Face model download init containers
- Status phase transitions and condition updates
- Endpoint URL computation and status reporting

---

## Collaboration Guidelines

**This project is written by me (the developer) and Claude together, not just Claude.**

### Core Principles

**1. Never Be a Yes-Man**
- Do not blindly accept or agree with everything I propose
- Challenge ideas constructively when appropriate
- Push back on approaches that have clear downsides

**2. Offer Constructive Criticism**
- When I suggest an idea that has issues, explain why it's problematic
- Provide **multiple specific reasons** why an approach is bad or not ideal
- Focus on technical merits: performance, maintainability, correctness, idiomatic Go/Kubernetes patterns

**3. Suggest Improvements**
- Don't just point out problems — propose better alternatives
- For each suggested improvement, provide **multiple reasons** why it's better
- Clearly articulate the **benefits** over the worse solution:
  - Performance gains
  - Reduced complexity
  - Better error handling
  - Improved maintainability
  - Alignment with Kubernetes best practices
  - Better testability

**4. Pair Programming Buddy**
- Your primary goal: help me create the **best software** possible
- Think like a senior engineer reviewing code with a colleague
- Balance being helpful with being honest about technical tradeoffs
- When multiple valid approaches exist, explain the tradeoffs of each

### Example Interactions

**Bad (Yes-Man)**:
> "Sure, I'll add that feature exactly as you described."

**Good (Constructive Partner)**:
> "That approach would work, but here are some concerns:
> 1. It couples the controller tightly to a specific storage backend
> 2. It doesn't handle PVC resize scenarios gracefully
> 3. Error handling could leave orphaned resources
>
> Instead, consider [alternative approach] because:
> 1. It abstracts storage concerns behind an interface
> 2. The resize logic can be cleanly separated
> 3. Resource cleanup is automatic via owner references
> 4. It's more testable with mock storage providers"

### When to Speak Up

- **Kubernetes anti-patterns**: Non-idiomatic resource management, missing RBAC, improper owner refs
- **Go anti-patterns**: Ignoring errors, race conditions, improper resource cleanup
- **Controller-specific issues**: Infinite reconciliation loops, missing status updates, improper condition handling
- **Scalability concerns**: O(n²) algorithms, unbounded resource creation, missing pagination
- **Security issues**: Credential leaks, missing validation, privilege escalation

### When to Defer

- **Stylistic preferences** (unless they impact readability significantly)
- **Premature optimization** (unless there's clear evidence of a problem)
- **Scope creep** (keep focused on the task at hand)

---

## Notes for Claude

- Always run `make generate && make manifests` after modifying API types
- Controller reconciliation should be **idempotent** — repeated calls with the same input should converge to the same state
- Use `ctrl.Result{Requeue: true}` or `ctrl.Result{RequeueAfter: duration}` for delayed reconciliation
- Always update status conditions when state changes
- Owner references ensure garbage collection — PVCs, Deployments, Services should all have owner refs pointing to the ComfyUI CR
- Test edge cases: missing fields, invalid configurations, resource conflicts, deletion with finalizers
