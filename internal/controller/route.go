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

	routev1 "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	comfyv1alpha1 "github.com/comfyui-operator/api/v1alpha1"
)

// reconcileRoute creates or updates OpenShift Routes for the ComfyUI instance
// Creates two routes: one for ComfyUI (port 8188) and one for filebrowser (port 8080)
func (r *ComfyUIReconciler) reconcileRoute(ctx context.Context, comfyui *comfyv1alpha1.ComfyUI) error {
	log := logf.FromContext(ctx)

	// Build standard labels
	labels := map[string]string{
		"app.kubernetes.io/name":       "comfyui",
		"app.kubernetes.io/instance":   comfyui.Name,
		"app.kubernetes.io/managed-by": "comfyui-operator",
	}

	// Reconcile ComfyUI route (port 8188)
	if err := r.reconcileSingleRoute(ctx, comfyui, comfyui.Name, "http", labels); err != nil {
		return fmt.Errorf("failed to reconcile ComfyUI route: %w", err)
	}

	// Reconcile Filebrowser route
	// If OAuth2 is enabled, route to oauth2-proxy (port 4180), otherwise direct to filebrowser (port 8080)
	filebrowserRouteName := comfyui.Name + "-filebrowser"
	filebrowserTargetPort := "filebrowser"
	if comfyui.Spec.OAuth2 != nil {
		filebrowserTargetPort = "oauth2-proxy"
	}
	if err := r.reconcileSingleRoute(ctx, comfyui, filebrowserRouteName, filebrowserTargetPort, labels); err != nil {
		return fmt.Errorf("failed to reconcile Filebrowser route: %w", err)
	}

	log.Info("Routes reconciled successfully", "name", comfyui.Name)
	return nil
}

// reconcileSingleRoute creates or updates a single OpenShift Route
func (r *ComfyUIReconciler) reconcileSingleRoute(ctx context.Context, comfyui *comfyv1alpha1.ComfyUI, routeName, targetPort string, labels map[string]string) error {
	log := logf.FromContext(ctx)

	// Build the desired Route
	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: comfyui.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(comfyui, comfyv1alpha1.GroupVersion.WithKind("ComfyUI")),
			},
		},
		Spec: routev1.RouteSpec{
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: comfyui.Name,
			},
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromString(targetPort),
			},
			// TLS configuration - edge termination for HTTPS
			TLS: &routev1.TLSConfig{
				Termination:                   routev1.TLSTerminationEdge,
				InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
			},
		},
	}

	// Check if Route already exists
	found := &routev1.Route{}
	err := r.Get(ctx, types.NamespacedName{Name: routeName, Namespace: comfyui.Namespace}, found)

	if err != nil {
		if errors.IsNotFound(err) {
			// Route doesn't exist - create it
			if err := r.Create(ctx, route); err != nil {
				return fmt.Errorf("failed to create Route: %w", err)
			}
			log.Info("Route created", "name", routeName, "host", route.Spec.Host)
			return nil
		}
		return fmt.Errorf("failed to get Route: %w", err)
	}

	// Route exists - update if needed
	found.Spec = route.Spec
	if err := r.Update(ctx, found); err != nil {
		return fmt.Errorf("failed to update Route: %w", err)
	}

	log.Info("Route reconciled", "name", routeName, "host", found.Spec.Host)
	return nil
}
