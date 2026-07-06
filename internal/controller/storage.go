/*
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
*/

package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	comfyv1alpha1 "github.com/comfyui-operator/api/v1alpha1"
)

// createPVC creates a PersistentVolumeClaim spec for the ComfyUI instance
func (r *ComfyUIReconciler) createPVC(ctx context.Context, comfyui *comfyv1alpha1.ComfyUI) (*corev1.PersistentVolumeClaim, error) {
	// Check if storage spec exists
	if comfyui.Spec.Storage == nil {
		return nil, fmt.Errorf("storage spec is required")
	}

	// Parse and validate storage size
	quantity, err := resource.ParseQuantity(comfyui.Spec.Storage.Size)
	if err != nil {
		return nil, fmt.Errorf("invalid storage size %q: %w", comfyui.Spec.Storage.Size, err)
	}
	if quantity.Sign() <= 0 {
		return nil, fmt.Errorf("storage size must be a positive value")
	}

	// Handle storage class: if empty string or nil, let Kubernetes use default
	var storageClassName *string
	if comfyui.Spec.Storage.StorageClassName != nil && *comfyui.Spec.Storage.StorageClassName != "" {
		storageClassName = comfyui.Spec.Storage.StorageClassName
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-storage", comfyui.Name),
			Namespace: comfyui.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "comfyui",
				"app.kubernetes.io/instance":   comfyui.Name,
				"app.kubernetes.io/managed-by": "comfyui-operator",
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(comfyui, comfyv1alpha1.GroupVersion.WithKind("ComfyUI")),
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				comfyui.Spec.Storage.AccessMode,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: quantity,
				},
			},
			StorageClassName: storageClassName,
		},
	}
	return pvc, nil
}

// reconcilePVC ensures the PersistentVolumeClaim exists and is properly configured
func (r *ComfyUIReconciler) reconcilePVC(ctx context.Context, comfyui *comfyv1alpha1.ComfyUI) error {
	log := logf.FromContext(ctx)

	// If no storage is specified, nothing to do
	if comfyui.Spec.Storage == nil {
		return nil
	}

	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, types.NamespacedName{Name: comfyui.Name + "-storage", Namespace: comfyui.Namespace}, pvc)

	if err != nil {
		if errors.IsNotFound(err) {
			// PVC doesn't exist - create it
			newPVC, err := r.createPVC(ctx, comfyui)
			if err != nil {
				return fmt.Errorf("failed to create PVC spec: %w", err)
			}
			if err := r.Create(ctx, newPVC); err != nil {
				return fmt.Errorf("failed to create PVC: %w", err)
			}
			log.Info("PVC created", "name", newPVC.Name)
			return nil
		}
		return fmt.Errorf("failed to get PVC: %w", err)
	}

	// PVC is being deleted — requeue so we can create a new one after it's gone
	if pvc.DeletionTimestamp != nil {
		log.Info("PVC is being deleted, requeueing", "name", pvc.Name)
		return fmt.Errorf("PVC %s is being deleted, will retry", pvc.Name)
	}

	if pvc.Status.Phase != corev1.ClaimBound {
		log.Info("PVC pending", "phase", pvc.Status.Phase, "name", pvc.Name)
	}

	return nil
}

// mountVolume configures volume mounts for the ComfyUI deployment
func (r *ComfyUIReconciler) mountVolume(comfyui *comfyv1alpha1.ComfyUI, deployment *appsv1.Deployment) error {
	if comfyui.Spec.Storage == nil {
		return nil
	}

	pvcName := fmt.Sprintf("%s-storage", comfyui.Name)

	// Add PVC volume to the deployment
	// Preserve existing volumes (e.g., filebrowser-tmp) and user-specified volumes
	deployment.Spec.Template.Spec.Volumes = append(
		deployment.Spec.Template.Spec.Volumes,
		append(
			comfyui.Spec.Volumes, // User-specified volumes
			corev1.Volume{
				Name: "models-storage",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvcName,
					},
				},
			},
		)...,
	)

	// Mount PVC subdirectories to persist user data while preserving ComfyUI code from image
	// Use subPath to mount specific directories from the PVC
	deployment.Spec.Template.Spec.Containers[0].VolumeMounts = append(
		comfyui.Spec.VolumeMounts, // Preserve user-specified mounts
		corev1.VolumeMount{
			Name:      "models-storage",
			MountPath: "/app/ComfyUI/models",
			SubPath:   "models",
		},
		corev1.VolumeMount{
			Name:      "models-storage",
			MountPath: "/app/ComfyUI/custom_nodes",
			SubPath:   "custom_nodes",
		},
		corev1.VolumeMount{
			Name:      "models-storage",
			MountPath: "/app/ComfyUI/user",
			SubPath:   "user",
		},
		corev1.VolumeMount{
			Name:      "models-storage",
			MountPath: "/app/ComfyUI/output",
			SubPath:   "output",
		},
		corev1.VolumeMount{
			Name:      "models-storage",
			MountPath: "/app/ComfyUI/input",
			SubPath:   "input",
		},
		// Note: temp directory is intentionally ephemeral (not persisted)
	)

	// Mount entire PVC to filebrowser sidecar at /data for browsing all user files
	// Filebrowser is container index 1
	// This gives access to models, outputs, inputs, custom_nodes, and user directories
	if len(deployment.Spec.Template.Spec.Containers) > 1 {
		deployment.Spec.Template.Spec.Containers[1].VolumeMounts = append(
			deployment.Spec.Template.Spec.Containers[1].VolumeMounts,
			corev1.VolumeMount{
				Name:      "models-storage",
				MountPath: "/data",
			},
		)
	}

	return nil
}
