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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	comfyv1alpha1 "github.com/comfyui-operator/api/v1alpha1"
)

// reconcileService ensures the Service exists for the ComfyUI instance
func (r *ComfyUIReconciler) reconcileService(ctx context.Context, comfyui *comfyv1alpha1.ComfyUI) error {
	log := logf.FromContext(ctx)

	// Determine service type (default to ClusterIP)
	serviceType := corev1.ServiceTypeClusterIP
	if comfyui.Spec.ServiceType != "" {
		serviceType = comfyui.Spec.ServiceType
	}

	// Build standard labels
	labels := map[string]string{
		"app":                          comfyui.Name,
		"app.kubernetes.io/name":       "comfyui",
		"app.kubernetes.io/instance":   comfyui.Name,
		"app.kubernetes.io/managed-by": "comfyui-operator",
	}

	// Build the desired Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      comfyui.Name,
			Namespace: comfyui.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(comfyui, comfyv1alpha1.GroupVersion.WithKind("ComfyUI")),
			},
		},
		Spec: corev1.ServiceSpec{
			Type: serviceType,
			Selector: map[string]string{
				"app": comfyui.Name, // Must match deployment pod labels
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8188,
					TargetPort: intstr.FromInt(8188),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "filebrowser",
					Port:       8085,
					TargetPort: intstr.FromInt(8085),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "oauth2-proxy",
					Port:       4180,
					TargetPort: intstr.FromInt(4180),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	// Check if Service already exists
	found := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: comfyui.Name, Namespace: comfyui.Namespace}, found)

	if err != nil {
		if errors.IsNotFound(err) {
			// Service doesn't exist - create it
			if err := r.Create(ctx, service); err != nil {
				return fmt.Errorf("failed to create Service: %w", err)
			}
			log.Info("Service created", "name", comfyui.Name)
			return nil
		}
		return fmt.Errorf("failed to get Service: %w", err)
	}

	// Service exists - no updates needed
	// Services are mostly immutable (ClusterIP can't be changed)
	// Only spec.type, ports, and selectors can be updated if needed
	return nil
}
