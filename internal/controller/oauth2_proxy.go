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
	"crypto/rand"
	"encoding/base64"
	"fmt"

	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	comfyv1alpha1 "github.com/comfyui-operator/api/v1alpha1"
)

// addOAuth2Proxy adds an OAuth2 Proxy sidecar to protect FileBrowser with OAuth2 authentication
func (r *ComfyUIReconciler) addOAuth2Proxy(ctx context.Context, comfyui *comfyv1alpha1.ComfyUI, deployment *appsv1.Deployment) error {
	oauth2 := comfyui.Spec.OAuth2

	// Ensure cookie secret exists (generate if not provided)
	cookieSecretName := comfyui.Name + "-oauth2-cookie"
	cookieSecretKey := "cookie-secret"

	if oauth2.CookieSecretRef != nil {
		cookieSecretName = oauth2.CookieSecretRef.Name
		cookieSecretKey = oauth2.CookieSecretRef.Key
	} else {
		// Generate cookie secret if not provided
		if err := r.ensureOAuth2CookieSecret(ctx, comfyui, cookieSecretName); err != nil {
			return fmt.Errorf("failed to ensure OAuth2 cookie secret: %w", err)
		}
	}

	// Build oauth2-proxy container args based on provider
	args := []string{
		"--http-address=0.0.0.0:4180",
		"--upstream=http://localhost:8085",
		"--email-domain=*",     // Will be overridden by allowed emails/domains if specified
		"--reverse-proxy=true", // Trust X-Forwarded-* headers from OpenShift router / ingress
		"--cookie-secure=true",
		"--cookie-httponly=true",
		"--cookie-samesite=lax",
		"--set-xauthrequest=true",
		"--pass-access-token=true",
		"--pass-user-headers=true", // Pass X-Forwarded-User header to upstream (FileBrowser)
		"--skip-provider-button=true",
	}

	// Configure provider-specific settings
	switch oauth2.Provider {
	case "github":
		args = append(args, "--provider=github")
	case "google":
		args = append(args, "--provider=google")
	case "oidc":
		if oauth2.IssuerURL == "" {
			return fmt.Errorf("issuerURL is required for oidc provider")
		}
		args = append(args,
			"--provider=oidc",
			fmt.Sprintf("--oidc-issuer-url=%s", oauth2.IssuerURL),
		)
	default:
		return fmt.Errorf("unsupported OAuth2 provider: %s", oauth2.Provider)
	}

	// Look up the filebrowser Route to set the OAuth2 redirect URL
	filebrowserRoute := &routev1.Route{}
	routeName := comfyui.Name + "-filebrowser"
	if err := r.Get(ctx, types.NamespacedName{Name: routeName, Namespace: comfyui.Namespace}, filebrowserRoute); err == nil {
		host := filebrowserRoute.Spec.Host
		if host != "" {
			args = append(args, fmt.Sprintf("--redirect-url=https://%s/oauth2/callback", host))
		}
	}

	// Add allowed emails if specified
	// Individual emails are set via env var below, not via args
	if len(oauth2.AllowedEmails) > 0 {
		// Use email domain wildcard, actual emails enforced via env var
		args = append(args, "--email-domain=*")
	}

	// Add allowed domains if specified
	if len(oauth2.AllowedDomains) > 0 {
		for _, domain := range oauth2.AllowedDomains {
			args = append(args, fmt.Sprintf("--email-domain=%s", domain))
		}
	}

	// Build environment variables
	env := []corev1.EnvVar{
		{
			Name:  "OAUTH2_PROXY_CLIENT_ID",
			Value: oauth2.ClientID,
		},
		{
			Name: "OAUTH2_PROXY_CLIENT_SECRET",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &oauth2.ClientSecretRef,
			},
		},
		{
			Name: "OAUTH2_PROXY_COOKIE_SECRET",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cookieSecretName,
					},
					Key: cookieSecretKey,
				},
			},
		},
	}

	// Add allowed emails as comma-separated env var if specified
	if len(oauth2.AllowedEmails) > 0 {
		emailList := ""
		for i, email := range oauth2.AllowedEmails {
			if i > 0 {
				emailList += ","
			}
			emailList += email
		}
		env = append(env, corev1.EnvVar{
			Name:  "OAUTH2_PROXY_AUTHENTICATED_EMAILS_LIST",
			Value: emailList,
		})
	}

	// Create OAuth2 Proxy container
	oauth2ProxyContainer := corev1.Container{
		Name:            "oauth2-proxy",
		Image:           "quay.io/oauth2-proxy/oauth2-proxy:v7.6.0",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            args,
		Env:             env,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 4180,
				Name:          "oauth2-proxy",
				Protocol:      corev1.ProtocolTCP,
			},
		},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("64Mi"),
				corev1.ResourceCPU:    resource.MustParse("50m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
				corev1.ResourceCPU:    resource.MustParse("100m"),
			},
		},
	}

	// Add oauth2-proxy container to the deployment
	deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, oauth2ProxyContainer)

	return nil
}

// ensureOAuth2CookieSecret creates a Secret with a random cookie secret for OAuth2 Proxy
func (r *ComfyUIReconciler) ensureOAuth2CookieSecret(ctx context.Context, comfyui *comfyv1alpha1.ComfyUI, secretName string) error {
	log := logf.FromContext(ctx)

	// Check if Secret already exists
	found := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: comfyui.Namespace}, found)

	if err == nil {
		// Secret exists
		log.Info("OAuth2 cookie Secret already exists", "name", secretName)
		return nil
	}

	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get OAuth2 cookie Secret: %w", err)
	}

	// Secret doesn't exist - create it with random cookie secret
	// OAuth2 Proxy requires a 16, 24, or 32 byte base64-encoded secret
	cookieSecret, err := generateCookieSecret(32)
	if err != nil {
		return fmt.Errorf("failed to generate cookie secret: %w", err)
	}

	// Build standard labels
	labels := map[string]string{
		"app.kubernetes.io/name":       "comfyui",
		"app.kubernetes.io/instance":   comfyui.Name,
		"app.kubernetes.io/managed-by": "comfyui-operator",
		"app.kubernetes.io/component":  "oauth2-proxy",
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: comfyui.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(comfyui, comfyv1alpha1.GroupVersion.WithKind("ComfyUI")),
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"cookie-secret": cookieSecret,
		},
	}

	if err := r.Create(ctx, secret); err != nil {
		return fmt.Errorf("failed to create OAuth2 cookie Secret: %w", err)
	}

	log.Info("OAuth2 cookie Secret created", "name", secretName)
	return nil
}

// generateCookieSecret creates a cryptographically secure random secret for OAuth2 Proxy cookies
// OAuth2 Proxy requires the decoded secret to be 16, 24, or 32 bytes
// We return base64-encoded string for storage, but the decoded value must be the right length
func generateCookieSecret(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	// Return base64 URL encoding (what oauth2-proxy expects)
	return base64.URLEncoding.EncodeToString(bytes), nil
}
