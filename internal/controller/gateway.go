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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	comfyv1alpha1 "github.com/comfyui-operator/api/v1alpha1"
)

// reconcileHTTPRoute creates or updates a Kubernetes Gateway API HTTPRoute for the ComfyUI instance
func (r *ComfyUIReconciler) reconcileHTTPRoute(ctx context.Context, comfyui *comfyv1alpha1.ComfyUI) error {
	log := logf.FromContext(ctx)

	// Gateway configuration with defaults
	gatewayName := "default-gateway"
	gatewayNamespace := "gateway-system"
	hostname := fmt.Sprintf("%s.example.com", comfyui.Name)

	// Override with user-specified values if provided
	if comfyui.Spec.Gateway != nil {
		if comfyui.Spec.Gateway.Name != "" {
			gatewayName = comfyui.Spec.Gateway.Name
		}
		if comfyui.Spec.Gateway.Namespace != "" {
			gatewayNamespace = comfyui.Spec.Gateway.Namespace
		}
		if comfyui.Spec.Gateway.Hostname != "" {
			hostname = comfyui.Spec.Gateway.Hostname
		}
	}

	// Build standard labels
	labels := map[string]string{
		"app.kubernetes.io/name":       "comfyui",
		"app.kubernetes.io/instance":   comfyui.Name,
		"app.kubernetes.io/managed-by": "comfyui-operator",
	}

	// Build the desired HTTPRoute
	httpRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      comfyui.Name,
			Namespace: comfyui.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(comfyui, comfyv1alpha1.GroupVersion.WithKind("ComfyUI")),
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				// Attach to the parent Gateway
				ParentRefs: []gatewayv1.ParentReference{
					{
						Name:      gatewayv1.ObjectName(gatewayName),
						Namespace: (*gatewayv1.Namespace)(&gatewayNamespace),
					},
				},
			},
			// Hostname for this route
			Hostnames: []gatewayv1.Hostname{
				gatewayv1.Hostname(hostname),
			},
			// Routing rules
			Rules: []gatewayv1.HTTPRouteRule{
				{
					// Match all requests with path prefix "/"
					Matches: []gatewayv1.HTTPRouteMatch{
						{
							Path: &gatewayv1.HTTPPathMatch{
								Type:  ptr.To(gatewayv1.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
						},
					},
					// Backend service reference
					BackendRefs: []gatewayv1.HTTPBackendRef{
						{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: gatewayv1.ObjectName(comfyui.Name),
									Port: ptr.To(gatewayv1.PortNumber(8188)),
								},
							},
						},
					},
				},
			},
		},
	}

	// Check if HTTPRoute already exists
	found := &gatewayv1.HTTPRoute{}
	err := r.Get(ctx, types.NamespacedName{Name: comfyui.Name, Namespace: comfyui.Namespace}, found)

	if err != nil {
		if errors.IsNotFound(err) {
			// HTTPRoute doesn't exist - create it
			if err := r.Create(ctx, httpRoute); err != nil {
				return fmt.Errorf("failed to create HTTPRoute: %w", err)
			}
			log.Info("HTTPRoute created", "name", comfyui.Name)
			return nil
		}
		return fmt.Errorf("failed to get HTTPRoute: %w", err)
	}

	// HTTPRoute exists - update if needed
	found.Spec = httpRoute.Spec
	if err := r.Update(ctx, found); err != nil {
		return fmt.Errorf("failed to update HTTPRoute: %w", err)
	}

	log.Info("HTTPRoute reconciled", "name", comfyui.Name)
	return nil
}
