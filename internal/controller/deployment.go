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
	"strconv"
	"strings"

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

// reconcileDeployment ensures the ComfyUI Deployment exists and is properly configured
func (r *ComfyUIReconciler) reconcileDeployment(ctx context.Context, comfyui *comfyv1alpha1.ComfyUI) error {
	log := logf.FromContext(ctx)

	// Set default replicas if not specified
	replicas := int32(1)
	if comfyui.Spec.Replicas != nil {
		replicas = *comfyui.Spec.Replicas
	}

	// Build standard labels
	labels := map[string]string{
		"app":                          comfyui.Name,
		"app.kubernetes.io/name":       "comfyui",
		"app.kubernetes.io/instance":   comfyui.Name,
		"app.kubernetes.io/managed-by": "comfyui-operator",
	}

	// Build the desired Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      comfyui.Name,
			Namespace: comfyui.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(comfyui, comfyv1alpha1.GroupVersion.WithKind("ComfyUI")),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": comfyui.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					// Node selector for targeting specific nodes (e.g., GPU nodes)
					NodeSelector:    comfyui.Spec.NodeSelector,
					SecurityContext: r.buildPodSecurityContext(ctx, comfyui),
					Volumes: []corev1.Volume{
						{
							Name: "filebrowser-tmp",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									Medium: corev1.StorageMediumMemory,
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "comfyui",
							Image:           comfyui.Spec.Image,
							ImagePullPolicy: corev1.PullAlways,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8188,
									Name:          "http",
									Protocol:      corev1.ProtocolTCP,
								},
							},
							// Container-level security context for OpenShift restricted SCC
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								// RunAsNonRoot: ptr.To(true), // TODO: Re-enable for production
							},
							// Start with empty resources, add if specified
							Resources: corev1.ResourceRequirements{},
						},
						// Filebrowser sidecar container for web-based file management
						{
							Name:            "filebrowser",
							Image:           "filebrowser/filebrowser:latest",
							ImagePullPolicy: corev1.PullIfNotPresent,
							// Auth is set via database init because the JSON config file doesn't support auth settings.
							// OAuth2-proxy handles authentication, so filebrowser uses noauth.
							Command: []string{"/bin/sh", "-c"},
							Args: []string{
								"rm -f /tmp/filebrowser.db && " +
									"filebrowser config init --database /tmp/filebrowser.db && " +
									"filebrowser users add admin adminpassword12 --database /tmp/filebrowser.db --perm.admin && " +
									"filebrowser config set --database /tmp/filebrowser.db --auth.method=noauth && " +
									"filebrowser --database /tmp/filebrowser.db --root /data --port 8085 --address 0.0.0.0 --log stdout",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "filebrowser-tmp",
									MountPath: "/tmp",
								},
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8085,
									Name:          "filebrowser",
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
									corev1.ResourceMemory: resource.MustParse("128Mi"),
									corev1.ResourceCPU:    resource.MustParse("100m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
									corev1.ResourceCPU:    resource.MustParse("200m"),
								},
							},
						},
					},
				},
			},
		},
	}

	// Add OAuth2 Proxy sidecar if OAuth2 is configured
	if comfyui.Spec.OAuth2 != nil {
		if err := r.addOAuth2Proxy(ctx, comfyui, deployment); err != nil {
			return err
		}
	}

	// Add user-specified resources if provided
	if comfyui.Spec.Resources != nil {
		deployment.Spec.Template.Spec.Containers[0].Resources = *comfyui.Spec.Resources
	}

	// Add GPU resources if enabled
	if comfyui.Spec.EnableGPU {
		if err := r.configureGPU(comfyui, deployment); err != nil {
			return err
		}
	}

	// Add volume mounts if storage is specified
	if err := r.mountVolume(comfyui, deployment); err != nil {
		return err
	}

	// Check if Deployment already exists
	found := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: comfyui.Name, Namespace: comfyui.Namespace}, found)

	if err != nil {
		if errors.IsNotFound(err) {
			// Deployment doesn't exist - create it
			if err := r.Create(ctx, deployment); err != nil {
				return fmt.Errorf("failed to create Deployment: %w", err)
			}
			log.Info("Deployment created", "name", comfyui.Name)
			return nil
		}
		return fmt.Errorf("failed to get Deployment: %w", err)
	}

	// Deployment exists - update it
	found.Spec = deployment.Spec
	if err := r.Update(ctx, found); err != nil {
		return fmt.Errorf("failed to update Deployment: %w", err)
	}

	return nil
}

// configureGPU adds GPU resource requests and limits to the deployment
func (r *ComfyUIReconciler) configureGPU(comfyui *comfyv1alpha1.ComfyUI, deployment *appsv1.Deployment) error {
	// Default to 1 GPU if count not specified
	gpuCount := int64(1)
	if comfyui.Spec.GPUCount != nil && *comfyui.Spec.GPUCount > 0 {
		gpuCount = int64(*comfyui.Spec.GPUCount)
	}

	// Initialize resource maps if not already set
	if deployment.Spec.Template.Spec.Containers[0].Resources.Limits == nil {
		deployment.Spec.Template.Spec.Containers[0].Resources.Limits = corev1.ResourceList{}
	}
	if deployment.Spec.Template.Spec.Containers[0].Resources.Requests == nil {
		deployment.Spec.Template.Spec.Containers[0].Resources.Requests = corev1.ResourceList{}
	}

	// Add GPU resource requests and limits
	// NVIDIA GPUs are the most common, but this could be made configurable
	deployment.Spec.Template.Spec.Containers[0].Resources.Limits["nvidia.com/gpu"] = *resource.NewQuantity(gpuCount, resource.DecimalSI)
	deployment.Spec.Template.Spec.Containers[0].Resources.Requests["nvidia.com/gpu"] = *resource.NewQuantity(gpuCount, resource.DecimalSI)

	return nil
}

func (r *ComfyUIReconciler) buildPodSecurityContext(ctx context.Context, comfyui *comfyv1alpha1.ComfyUI) *corev1.PodSecurityContext {
	log := logf.FromContext(ctx)

	if comfyui.Spec.FSGroup != nil {
		return &corev1.PodSecurityContext{
			FSGroup: comfyui.Spec.FSGroup,
		}
	}

	// Auto-detect from OpenShift namespace annotation
	ns := &corev1.Namespace{}
	if err := r.Get(ctx, types.NamespacedName{Name: comfyui.Namespace}, ns); err == nil {
		if rangeStr, ok := ns.Annotations["openshift.io/sa.scc.supplemental-groups"]; ok {
			base := strings.Split(rangeStr, "/")[0]
			if gid, err := strconv.ParseInt(base, 10, 64); err == nil {
				log.Info("Using fsGroup from namespace annotation", "fsGroup", gid)
				return &corev1.PodSecurityContext{
					FSGroup: ptr.To(gid),
				}
			}
		}
	}

	return &corev1.PodSecurityContext{
		FSGroup: ptr.To(int64(1000)),
	}
}
