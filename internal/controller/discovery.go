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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	comfyv1alpha1 "github.com/comfyui-operator/api/v1alpha1"
)

// discoverIngressAPIs checks which ingress APIs are available in the cluster
func discoverIngressAPIs() (bool, bool, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return false, false, fmt.Errorf("failed to get k8s config: %w", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return false, false, fmt.Errorf("failed to create discovery client: %w", err)
	}

	apiGroups, err := discoveryClient.ServerGroups()
	if err != nil {
		return false, false, fmt.Errorf("failed to discover API groups: %w", err)
	}

	hasRoute := false
	hasGateway := false

	for _, group := range apiGroups.Groups {
		if group.Name == "route.openshift.io" {
			hasRoute = true
		}
		if group.Name == "gateway.networking.k8s.io" {
			hasGateway = true
		}
	}

	return hasRoute, hasGateway, nil
}

// reconcileIngress discovers available ingress APIs and creates the appropriate resource
// Preference order: OpenShift Route (if on OpenShift) > Gateway API > None (ClusterIP only)
func (r *ComfyUIReconciler) reconcileIngress(ctx context.Context, comfyui *comfyv1alpha1.ComfyUI) error {
	log := logf.FromContext(ctx)

	// Discover what ingress APIs are available
	hasRoute, hasGateway, err := discoverIngressAPIs()
	if err != nil {
		// Don't fail reconciliation if discovery fails - just log and continue
		// The service will still be accessible via ClusterIP
		log.Error(err, "Failed to discover ingress APIs, skipping ingress creation")
		return nil
	}

	// Prefer OpenShift Route if available (native to OpenShift)
	if hasRoute {
		if !r.Scheme.Recognizes(schema.GroupVersionKind{Group: routev1.GroupName, Version: "v1", Kind: "Route"}) {
			log.Info("Route API discovered but scheme not registered, skipping Route creation")
		} else {
			if err := r.reconcileRoute(ctx, comfyui); err != nil {
				return fmt.Errorf("failed to reconcile Route: %w", err)
			}
			log.Info("Route reconciled successfully", "name", comfyui.Name)
			return nil
		}
	}

	// Fall back to Gateway API if available
	if hasGateway {
		if !r.Scheme.Recognizes(schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute"}) {
			log.Info("Gateway API discovered but scheme not registered, skipping HTTPRoute creation")
		} else {
			if err := r.reconcileHTTPRoute(ctx, comfyui); err != nil {
				return fmt.Errorf("failed to reconcile HTTPRoute: %w", err)
			}
			log.Info("HTTPRoute reconciled successfully", "name", comfyui.Name)
			return nil
		}
	}

	// Neither API available - log info but don't fail
	log.Info("No ingress API available (Route or Gateway API), service will be ClusterIP only",
		"name", comfyui.Name,
		"namespace", comfyui.Namespace)
	return nil
}
