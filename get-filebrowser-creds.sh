#!/bin/bash
# Script to retrieve FileBrowser credentials for a ComfyUI instance

if [ -z "$1" ]; then
  echo "Usage: $0 <comfyui-name> [namespace]"
  echo "Example: $0 comfyui-kind default"
  exit 1
fi

COMFYUI_NAME=$1
NAMESPACE=${2:-default}
SECRET_NAME="${COMFYUI_NAME}-filebrowser"

echo "Retrieving FileBrowser credentials for ComfyUI: $COMFYUI_NAME"
echo "Namespace: $NAMESPACE"
echo ""

if ! kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" &>/dev/null; then
  echo "Error: Secret '$SECRET_NAME' not found in namespace '$NAMESPACE'"
  exit 1
fi

USERNAME=$(kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" -o jsonpath='{.data.username}' | base64 -d)
PASSWORD=$(kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" -o jsonpath='{.data.password}' | base64 -d)

echo "FileBrowser Credentials:"
echo "========================"
echo "Username: $USERNAME"
echo "Password: $PASSWORD"
echo ""
echo "Save these credentials securely!"
