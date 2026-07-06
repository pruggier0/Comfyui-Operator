FROM docker.io/python:3.12-slim

ENV DEBIAN_FRONTEND=noninteractive

WORKDIR /app

# Install git, wget, nano, and curl
RUN apt-get update && \
    apt-get install -y git wget nano curl && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Clone ComfyUI
RUN git clone https://github.com/comfyanonymous/ComfyUI.git /app/ComfyUI

WORKDIR /app/ComfyUI

# Install PyTorch CPU-only version (NO CUDA, no triton)
RUN pip install --no-cache-dir torch torchvision torchaudio --index-url https://download.pytorch.org/whl/cpu && \
    pip install --no-cache-dir -r requirements.txt

# Install ComfyUI Manager dependencies (built-in manager)
RUN pip install --no-cache-dir -r manager_requirements.txt

# Clone standalone ComfyUI-Manager into custom_nodes for legacy UI
RUN git clone https://github.com/ltdrdata/ComfyUI-Manager.git /app/ComfyUI/custom_nodes/ComfyUI-Manager && \
    pip install --no-cache-dir -r /app/ComfyUI/custom_nodes/ComfyUI-Manager/requirements.txt

# OpenShift runs containers with arbitrary UIDs but always includes GID 0.
# Make /app group-writable so any UID in the root group can write.
RUN useradd -m -u 1000 -s /bin/bash comfyui && \
    chown -R 1000:0 /app && \
    chmod -R g=u /app

# Set required environment variables
ENV USER=comfyui
ENV HOME=/app

COPY --chown=1000:0 entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Switch to non-root user
USER 1000

EXPOSE 8188

ENTRYPOINT ["/app/entrypoint.sh"]
