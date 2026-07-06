#!/bin/bash
set -e

# Build and push ComfyUI GPU image to OpenShift internal registry
#
# Usage:
#   ./build-and-push.sh [IMAGE_NAME] [TAG]
#
# Default: image-registry.openshift-image-registry.svc:5000/default/comfyui:latest

IMAGE_NAME=${1:-"image-registry.openshift-image-registry.svc:5000/default/comfyui"}
TAG=${2:-"latest"}
FULL_IMAGE="${IMAGE_NAME}:${TAG}"

echo "======================================"
echo "Building ComfyUI GPU Image"
echo "======================================"
echo "Image: ${FULL_IMAGE}"
echo ""

# Build the image
echo "Building Docker image..."
docker build -t "${FULL_IMAGE}" .

echo ""
echo "======================================"
echo "Build Complete!"
echo "======================================"
echo ""
echo "To push to OpenShift internal registry:"
echo "  1. Login to OpenShift: oc login"
echo "  2. Get registry route: oc get route -n openshift-image-registry"
echo "  3. Login to registry: docker login <registry-route>"
echo "  4. Tag for external registry:"
echo "     docker tag ${FULL_IMAGE} <registry-route>/default/comfyui:${TAG}"
echo "  5. Push:"
echo "     docker push <registry-route>/default/comfyui:${TAG}"
echo ""
echo "Or use Podman (recommended for OpenShift):"
echo "  podman build -t ${FULL_IMAGE} ."
echo "  podman push ${FULL_IMAGE}"
echo ""
