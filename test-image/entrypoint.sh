#!/bin/bash
set -e
echo "=== Setting up ComfyUI persistent storage ==="

mkdir -p /app/ComfyUI/models/checkpoints \
         /app/ComfyUI/models/vae \
         /app/ComfyUI/models/loras \
         /app/ComfyUI/models/embeddings \
         /app/ComfyUI/models/controlnet \
         /app/ComfyUI/models/clip \
         /app/ComfyUI/models/clip_vision \
         /app/ComfyUI/models/text_encoders \
         /app/ComfyUI/models/diffusion_models \
         /app/ComfyUI/models/unet \
         /app/ComfyUI/models/upscale_models \
         /app/ComfyUI/models/hypernetworks \
         /app/ComfyUI/models/style_models

mkdir -p /app/ComfyUI/user \
         /app/ComfyUI/output \
         /app/ComfyUI/input \
         /app/ComfyUI/temp \
         /app/ComfyUI/custom_nodes

echo "✓ Directory structure created"

# Clone ComfyUI-Manager if not already present (for legacy UI)
if [ ! -d "/app/ComfyUI/custom_nodes/ComfyUI-Manager" ]; then
  echo "=== Installing ComfyUI-Manager ==="
  git clone https://github.com/ltdrdata/ComfyUI-Manager.git /app/ComfyUI/custom_nodes/ComfyUI-Manager
  pip install --no-cache-dir -r /app/ComfyUI/custom_nodes/ComfyUI-Manager/requirements.txt
  echo "✓ ComfyUI-Manager installed"
else
  echo "✓ ComfyUI-Manager already exists"
fi

# Install dependencies for custom nodes
echo "=== Installing custom node dependencies ==="
custom_nodes_with_deps=0
for node_dir in /app/ComfyUI/custom_nodes/*; do
  if [ ! -d "$node_dir" ]; then continue; fi
  node_name=$(basename "$node_dir")

  if [ -f "$node_dir/requirements.txt" ]; then
    echo "→ Installing dependencies for: $node_name"
    pip install --no-cache-dir -r "$node_dir/requirements.txt" || {
      echo "⚠ Warning: Failed to install dependencies for $node_name" >&2
    }
    custom_nodes_with_deps=$((custom_nodes_with_deps + 1))
  fi

  if [ -f "$node_dir/install.py" ]; then
    echo "→ Running install script for: $node_name"
    (cd "$node_dir" && python install.py) || {
      echo "⚠ Warning: install.py failed for $node_name" >&2
    }
  fi
done

if [ $custom_nodes_with_deps -eq 0 ]; then
  echo "✓ No custom node dependencies to install"
else
  echo "✓ Installed dependencies for $custom_nodes_with_deps custom node(s)"
fi

# Detect GPU or fall back to CPU
echo "=== Detecting hardware ==="
EXTRA_ARGS=""
if python -c "import torch; assert torch.cuda.is_available()" 2>/dev/null; then
  GPU_NAME=$(python -c "import torch; print(torch.cuda.get_device_name(0))")
  echo "✓ GPU detected: $GPU_NAME"
else
  echo "✓ No GPU detected, running in CPU-only mode"
  EXTRA_ARGS="--cpu"
fi

echo "=== Starting ComfyUI ==="
exec python main.py --listen 0.0.0.0 --port 8188 $EXTRA_ARGS
