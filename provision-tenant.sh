#!/bin/bash
# provision-tenant.sh
# Provisions a new ComfyUI tenant with all necessary resources

set -e

# ============================================================================
# CONFIGURATION - Customize these values for your environment
# ============================================================================
#
# Before using this script, you MUST configure:
#
# 1. OAUTH2_CLIENT_ID: Your OAuth2 provider's client ID
#    - For Google: Get from https://console.cloud.google.com/apis/credentials
#    - Set to empty string to disable OAuth by default
#
# 2. DEFAULT_IMAGE: Your ComfyUI container image
#    - Example: "registry.example.com/comfyui:latest"
#    - Can have separate CPU and GPU variants
#
# 3. API_GROUP: Replace "comfy.example.com" throughout this file with your
#    actual CRD API group (e.g., "comfy.mycompany.com")
#
# 4. Domain defaults: Update ".example.com" references to your domain
#
# ============================================================================

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m' # No Color

# Print banner
print_banner() {
    clear
    echo -e "${CYAN}"
    cat <<'EOF'
╔══════════════════════════════════════════════════════════════════════╗
║                                                                      ║
║              ComfyUI Multi-Tenant Provisioning System                ║
║                                                                      ║
║                    Powered by ComfyUI Operator                       ║
║                                                                      ║
╚══════════════════════════════════════════════════════════════════════╝
EOF
    echo -e "${NC}"
}

# Print section header
print_header() {
    local title=$1
    echo ""
    echo -e "${BOLD}${BLUE}╔══════════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}${BLUE}║${NC}  ${BOLD}$title${NC}"
    echo -e "${BOLD}${BLUE}╚══════════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
}

# Print progress step
print_step() {
    local step=$1
    local total=$2
    local description=$3
    echo -e "${CYAN}[${step}/${total}]${NC} ${BOLD}${description}${NC}"
}

# Print success
print_success() {
    echo -e "  ${GREEN}[✓] $1${NC}"
}

# Print warning
print_warning() {
    echo -e "  ${YELLOW}[!] $1${NC}"
}

# Print error
print_error() {
    echo -e "  ${RED}[✗] $1${NC}"
}

# Print info
print_info() {
    echo -e "  ${BLUE}[→] $1${NC}"
}

# Spinner for long operations
spinner() {
    local pid=$1
    local message=$2
    local spin='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
    local i=0

    echo -n "  "
    while kill -0 $pid 2>/dev/null; do
        i=$(( (i+1) %10 ))
        echo -ne "\r  ${CYAN}${spin:$i:1}${NC} ${message}..."
        sleep .1
    done
    echo -ne "\r"
}

# Configuration defaults
DEFAULT_STORAGE_CLASS=""  # Empty string uses cluster default storage class
DEFAULT_OAUTH_PROVIDER="google"
OAUTH2_CLIENT_ID=""  # Set your OAuth2 client ID here
INTERNAL_REGISTRY="image-registry.openshift-image-registry.svc:5000"
DEFAULT_IMAGE=""  # Set your default ComfyUI image here

# Usage function
usage() {
    cat <<EOF
Usage: $0 <tenant-name> [OPTIONS]

Provisions a new ComfyUI tenant with namespace, resources, and configuration.

Required Arguments:
  tenant-name           Name of the tenant (e.g., 'data-science', 'marketing')

Options:
  --gpu                 Enable GPU support (default: false)
  --gpu-count N         Number of GPUs (default: 1)
  --storage SIZE        Storage size (default: 50Gi)
  --storage-class NAME  Storage class (default: $DEFAULT_STORAGE_CLASS)
  --replicas N          Number of replicas (default: 1)
  --cpu-image           Use CPU image (default)
  --gpu-image           Use GPU image
  --custom-image IMG    Use custom image
  --domain DOMAIN       OAuth2 allowed domain (default: <tenant>.example.com)
  --email EMAIL         OAuth2 allowed email (can specify multiple times)
  --oauth-secret SEC    OAuth2 client secret (default: prompt)
  --no-oauth            Disable OAuth2 authentication
  --cpu-request CPU     CPU request (default: 500m)
  --cpu-limit CPU       CPU limit (default: 2000m)
  --mem-request MEM     Memory request (default: 4Gi)
  --mem-limit MEM       Memory limit (default: 8Gi)
  --namespace-prefix    Namespace prefix (default: tenant-)
  --resource-quota      Create ResourceQuota (default: true)
  --node-selector KEY=VALUE  Add node selector (can specify multiple times)
  --dry-run             Print resources without applying
  --help                Show this help message

Examples:
  # Simple CPU tenant
  $0 marketing

  # GPU-enabled tenant with custom storage
  $0 data-science --gpu --gpu-count 2 --storage 100Gi

  # Custom configuration
  $0 research --gpu --custom-image registry/comfyui:v2.0 \\
    --domain research.example.com --email researcher@example.com \\
    --cpu-limit 4000m --mem-limit 16Gi

EOF
    exit 1
}

# Logging functions (backward compatible)
log_info() {
    print_success "$1"
}

log_warn() {
    print_warning "$1"
}

log_error() {
    print_error "$1"
    exit 1
}

log_step() {
    echo ""
    echo -e "${BOLD}${CYAN}▸ $1${NC}"
}

# Check prerequisites
check_prerequisites() {
    log_step "Checking prerequisites..."

    if ! command -v oc &> /dev/null && ! command -v kubectl &> /dev/null; then
        log_error "Neither oc nor kubectl found. Please install OpenShift or Kubernetes CLI."
    fi

    # Prefer oc if available
    if command -v oc &> /dev/null; then
        K8S_CLI="oc"
    else
        K8S_CLI="kubectl"
    fi

    if ! $K8S_CLI whoami &> /dev/null; then
        log_error "Not logged into cluster. Please login first."
    fi

    # Check if ComfyUI CRD exists
    if ! $K8S_CLI get crd comfyuis.comfy.example.com &> /dev/null; then
        log_error "ComfyUI CRD not found. Please install the operator first."
    fi

    log_info "Prerequisites OK (using $K8S_CLI)"
}

# Parse arguments
parse_args() {
    if [ $# -eq 0 ]; then
        usage
    fi

    TENANT_NAME=""
    ENABLE_GPU=false
    GPU_COUNT=1
    STORAGE_SIZE="50Gi"
    STORAGE_CLASS="$DEFAULT_STORAGE_CLASS"
    REPLICAS=1
    IMAGE=""
    OAUTH_DOMAIN=""
    OAUTH_EMAILS=()
    OAUTH_SECRET=""
    ENABLE_OAUTH=true
    CPU_REQUEST="500m"
    CPU_LIMIT="2000m"
    MEM_REQUEST="4Gi"
    MEM_LIMIT="8Gi"
    NAMESPACE_PREFIX="tenant-"
    CREATE_QUOTA=true
    NODE_SELECTORS=()
    DRY_RUN=false

    # First argument is tenant name
    TENANT_NAME=$1
    shift

    # Parse options
    while [[ $# -gt 0 ]]; do
        case $1 in
            --gpu)
                ENABLE_GPU=true
                shift
                ;;
            --gpu-count)
                GPU_COUNT=$2
                ENABLE_GPU=true
                shift 2
                ;;
            --storage)
                STORAGE_SIZE=$2
                shift 2
                ;;
            --storage-class)
                STORAGE_CLASS=$2
                shift 2
                ;;
            --replicas)
                REPLICAS=$2
                shift 2
                ;;
            --cpu-image)
                IMAGE="$DEFAULT_IMAGE"
                shift
                ;;
            --gpu-image)
                IMAGE="${DEFAULT_IMAGE/cpu/gpu}"
                ENABLE_GPU=true
                shift
                ;;
            --custom-image)
                IMAGE=$2
                shift 2
                ;;
            --domain)
                OAUTH_DOMAIN=$2
                shift 2
                ;;
            --email)
                OAUTH_EMAILS+=("$2")
                shift 2
                ;;
            --oauth-secret)
                OAUTH_SECRET=$2
                shift 2
                ;;
            --no-oauth)
                ENABLE_OAUTH=false
                shift
                ;;
            --cpu-request)
                CPU_REQUEST=$2
                shift 2
                ;;
            --cpu-limit)
                CPU_LIMIT=$2
                shift 2
                ;;
            --mem-request)
                MEM_REQUEST=$2
                shift 2
                ;;
            --mem-limit)
                MEM_LIMIT=$2
                shift 2
                ;;
            --namespace-prefix)
                NAMESPACE_PREFIX=$2
                shift 2
                ;;
            --resource-quota)
                CREATE_QUOTA=true
                shift
                ;;
            --node-selector)
                NODE_SELECTORS+=("$2")
                shift 2
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            --help)
                usage
                ;;
            *)
                log_error "Unknown option: $1"
                ;;
        esac
    done

    # Validate tenant name
    if [[ ! $TENANT_NAME =~ ^[a-z0-9-]+$ ]]; then
        log_error "Invalid tenant name. Must be lowercase alphanumeric with hyphens."
    fi

    # Set defaults
    NAMESPACE="${NAMESPACE_PREFIX}${TENANT_NAME}"

    if [ -z "$IMAGE" ]; then
        if [ "$ENABLE_GPU" = true ]; then
            IMAGE="${DEFAULT_IMAGE/cpu/gpu}"
        else
            IMAGE="$DEFAULT_IMAGE"
        fi
    fi

    if [ -z "$OAUTH_DOMAIN" ]; then
        OAUTH_DOMAIN="${TENANT_NAME}.example.com"
    fi
}

# Display configuration
show_config() {
    print_header "Configuration Review"

    echo -e "${BOLD}Tenant Details:${NC}"
    echo -e "  ${CYAN}Name:${NC}           $TENANT_NAME"
    echo -e "  ${CYAN}Namespace:${NC}      $NAMESPACE"
    echo ""

    echo -e "${BOLD}Compute Resources:${NC}"
    if [ "$ENABLE_GPU" = true ]; then
        echo -e "  ${CYAN}GPU:${NC}            ${GREEN}Enabled${NC} (${GPU_COUNT} GPU)"
    else
        echo -e "  ${CYAN}GPU:${NC}            ${DIM}Disabled${NC}"
    fi
    echo -e "  ${CYAN}Replicas:${NC}       $REPLICAS"
    echo -e "  ${CYAN}CPU:${NC}            ${CPU_REQUEST} → ${CPU_LIMIT}"
    echo -e "  ${CYAN}Memory:${NC}         ${MEM_REQUEST} → ${MEM_LIMIT}"
    echo ""

    echo -e "${BOLD}Storage:${NC}"
    echo -e "  ${CYAN}Size:${NC}           $STORAGE_SIZE"
    echo -e "  ${CYAN}Class:${NC}          $STORAGE_CLASS"
    echo ""

    echo -e "${BOLD}Image:${NC}"
    echo -e "  ${DIM}$IMAGE${NC}"
    echo ""

    if [ "$ENABLE_OAUTH" = true ]; then
        echo -e "${BOLD}OAuth2 Authentication:${NC}"
        echo -e "  ${CYAN}Provider:${NC}       $DEFAULT_OAUTH_PROVIDER"
        echo -e "  ${CYAN}Domain:${NC}         $OAUTH_DOMAIN"
        if [ ${#OAUTH_EMAILS[@]} -gt 0 ]; then
            echo -e "  ${CYAN}Emails:${NC}         ${OAUTH_EMAILS[@]}"
        fi
    else
        echo -e "${BOLD}OAuth2 Authentication:${NC} ${DIM}Disabled${NC}"
    fi
    echo ""

    if [ ${#NODE_SELECTORS[@]} -gt 0 ]; then
        echo -e "${BOLD}Node Selectors:${NC}"
        for selector in "${NODE_SELECTORS[@]}"; do
            echo -e "  ${CYAN}→${NC} $selector"
        done
        echo ""
    fi

    if [ "$DRY_RUN" = true ]; then
        echo -e "${YELLOW}╔══════════════════════════════════════════════════════════════════════╗${NC}"
        echo -e "${YELLOW}║  DRY RUN MODE - No resources will be created                        ║${NC}"
        echo -e "${YELLOW}╚══════════════════════════════════════════════════════════════════════╝${NC}"
        echo ""
    fi

    echo -e "${BOLD}${YELLOW}Proceed with tenant creation?${NC} ${DIM}(y/N)${NC} "
    read -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo ""
        print_warning "Provisioning cancelled by user"
        exit 0
    fi
}

# Create namespace
create_namespace() {
    log_step "Creating namespace: $NAMESPACE"

    if $K8S_CLI get namespace "$NAMESPACE" &> /dev/null; then
        log_warn "Namespace $NAMESPACE already exists"
    else
        if [ "$DRY_RUN" = false ]; then
            $K8S_CLI create namespace "$NAMESPACE"
            $K8S_CLI label namespace "$NAMESPACE" \
                tenant=true \
                tenant-name="$TENANT_NAME" \
                managed-by=comfyui-operator
            log_info "Namespace created and labeled"
        else
            log_info "[DRY RUN] Would create namespace: $NAMESPACE"
        fi
    fi
}

# Create OAuth2 secret
create_oauth_secret() {
    if [ "$ENABLE_OAUTH" = false ]; then
        log_info "OAuth disabled, skipping secret creation"
        return
    fi

    log_step "Creating OAuth2 secret"

    # Check if secret already exists
    if $K8S_CLI get secret oauth2-client-secret -n "$NAMESPACE" &> /dev/null; then
        log_warn "OAuth2 secret already exists in $NAMESPACE"
        return
    fi

    # Prompt for secret if not provided
    if [ -z "$OAUTH_SECRET" ]; then
        read -sp "Enter OAuth2 client secret (or press Enter to generate): " OAUTH_SECRET
        echo

        if [ -z "$OAUTH_SECRET" ]; then
            log_info "Generating random OAuth2 client secret..."
            OAUTH_SECRET=$(openssl rand -base64 32)
        fi
    fi

    if [ "$DRY_RUN" = false ]; then
        $K8S_CLI create secret generic oauth2-client-secret \
            --from-literal=client-secret="$OAUTH_SECRET" \
            -n "$NAMESPACE"
        log_info "OAuth2 secret created"
    else
        log_info "[DRY RUN] Would create OAuth2 secret"
    fi
}

# Create ResourceQuota
create_resource_quota() {
    if [ "$CREATE_QUOTA" = false ]; then
        log_info "ResourceQuota creation disabled"
        return
    fi

    log_step "Creating ResourceQuota"

    # Calculate quota based on replicas and resources (allow 3x requested)
    local cpu_req_num=$(echo "$CPU_REQUEST" | sed 's/m$//')
    local cpu_lim_num=$(echo "$CPU_LIMIT" | sed 's/m$//')
    local mem_req_num=$(echo "$MEM_REQUEST" | sed 's/Gi$//')
    local mem_lim_num=$(echo "$MEM_LIMIT" | sed 's/Gi$//')

    QUOTA_CPU_REQUEST=$((cpu_req_num * 3))
    QUOTA_CPU_LIMIT=$((cpu_lim_num * 3))
    QUOTA_MEM_REQUEST=$((mem_req_num * 3))
    QUOTA_MEM_LIMIT=$((mem_lim_num * 3))

    if [ "$ENABLE_GPU" = true ]; then
        GPU_QUOTA=$((GPU_COUNT * 3))
    else
        GPU_QUOTA=0
    fi

    local quota_yaml="apiVersion: v1
kind: ResourceQuota
metadata:
  name: tenant-quota
  namespace: $NAMESPACE
spec:
  hard:
    requests.cpu: \"${QUOTA_CPU_REQUEST}m\"
    requests.memory: \"${QUOTA_MEM_REQUEST}Gi\"
    limits.cpu: \"${QUOTA_CPU_LIMIT}m\"
    limits.memory: \"${QUOTA_MEM_LIMIT}Gi\"
    persistentvolumeclaims: \"5\"
    count/comfyuis.comfy.example.com: \"3\""

    if [ "$ENABLE_GPU" = true ]; then
        quota_yaml="${quota_yaml}
    requests.nvidia.com/gpu: \"$GPU_QUOTA\""
    fi

    if [ "$DRY_RUN" = false ]; then
        echo "$quota_yaml" | $K8S_CLI apply -f -
        log_info "ResourceQuota created"
    else
        echo "$quota_yaml"
        log_info "[DRY RUN] Would create ResourceQuota"
    fi
}

# Grant image pull permissions
grant_image_pull_permissions() {
    log_step "Granting image pull permissions from source image namespace"

    # Extract namespace from image URL
    if [[ $IMAGE =~ image-registry\.openshift-image-registry\.svc:5000/([^/]+)/ ]]; then
        local source_namespace="${BASH_REMATCH[1]}"

        if [ "$source_namespace" != "$NAMESPACE" ]; then
            log_info "Granting pull access from namespace: $source_namespace"

            if [ "$DRY_RUN" = false ]; then
                $K8S_CLI policy add-role-to-user system:image-puller \
                    system:serviceaccount:${NAMESPACE}:default \
                    -n "$source_namespace" 2>&1 | grep -v "Warning:" || true
                log_info "Image pull permission granted"
            else
                log_info "[DRY RUN] Would grant image-puller role for $NAMESPACE from $source_namespace"
            fi
        else
            log_info "Image is in same namespace, no cross-namespace permissions needed"
        fi
    else
        log_info "Image not from internal registry, skipping cross-namespace permissions"
    fi
}

# Create RBAC
create_rbac() {
    log_step "Creating RBAC for tenant self-service"

    cat <<EOF | if [ "$DRY_RUN" = false ]; then $K8S_CLI apply -f -; else cat; fi
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: comfyui-manager
  namespace: $NAMESPACE
rules:
- apiGroups: ["comfy.example.com"]
  resources: ["comfyuis"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get", "list", "create"]
- apiGroups: [""]
  resources: ["pods", "services", "persistentvolumeclaims"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["pods/log"]
  verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: comfyui-manager-binding
  namespace: $NAMESPACE
subjects:
- kind: Group
  name: tenant-${TENANT_NAME}-admins
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: Role
  name: comfyui-manager
  apiGroup: rbac.authorization.k8s.io
EOF

    if [ "$DRY_RUN" = false ]; then
        log_info "RBAC created (bind users to group: tenant-${TENANT_NAME}-admins)"
    else
        log_info "[DRY RUN] Would create RBAC"
    fi
}

# Create ComfyUI CR
create_comfyui() {
    log_step "Creating ComfyUI custom resource"

    # Build node selector YAML
    NODE_SELECTOR_YAML=""
    if [ ${#NODE_SELECTORS[@]} -gt 0 ]; then
        NODE_SELECTOR_YAML="  nodeSelector:"
        for selector in "${NODE_SELECTORS[@]}"; do
            key="${selector%%=*}"
            value="${selector#*=}"
            NODE_SELECTOR_YAML="${NODE_SELECTOR_YAML}
    ${key}: \"${value}\""
        done
    fi

    # Build OAuth2 section
    OAUTH_YAML=""
    if [ "$ENABLE_OAUTH" = true ]; then
        OAUTH_YAML="  oauth2:
    provider: \"$DEFAULT_OAUTH_PROVIDER\"
    clientID: \"$OAUTH2_CLIENT_ID\"
    clientSecretRef:
      name: oauth2-client-secret
      key: client-secret
    allowedDomains:
    - \"$OAUTH_DOMAIN\""

        for email in "${OAUTH_EMAILS[@]}"; do
            OAUTH_YAML="${OAUTH_YAML}
    allowedEmails:
    - \"$email\""
        done
    fi

    # Build GPU section
    GPU_YAML=""
    if [ "$ENABLE_GPU" = true ]; then
        GPU_YAML="  enableGPU: true
  gpuCount: $GPU_COUNT"
    fi

    cat <<EOF | if [ "$DRY_RUN" = false ]; then $K8S_CLI apply -f -; else cat; fi
apiVersion: comfy.example.com/v1alpha1
kind: ComfyUI
metadata:
  name: comfyui
  namespace: $NAMESPACE
  labels:
    tenant: $TENANT_NAME
spec:
  image: "$IMAGE"
  replicas: $REPLICAS
  serviceType: ClusterIP

$GPU_YAML

  resources:
    requests:
      cpu: "$CPU_REQUEST"
      memory: "$MEM_REQUEST"
    limits:
      cpu: "$CPU_LIMIT"
      memory: "$MEM_LIMIT"

  storage:
    size: "$STORAGE_SIZE"
    storageClassName: "$STORAGE_CLASS"
    accessMode: ReadWriteOnce

$NODE_SELECTOR_YAML

$OAUTH_YAML
EOF

    if [ "$DRY_RUN" = false ]; then
        log_info "ComfyUI CR created"
    else
        log_info "[DRY RUN] Would create ComfyUI CR"
    fi
}

# Wait for deployment
wait_for_ready() {
    if [ "$DRY_RUN" = true ]; then
        return
    fi

    echo ""
    log_step "Waiting for ComfyUI deployment to become ready"

    local timeout=300
    local elapsed=0
    local spin='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
    local i=0

    while [ $elapsed -lt $timeout ]; do
        local phase=$($K8S_CLI get comfyui comfyui -n "$NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
        local ready_replicas=$($K8S_CLI get comfyui comfyui -n "$NAMESPACE" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")

        if [ "$phase" = "Running" ] && [ "$ready_replicas" -ge "$REPLICAS" ]; then
            echo -ne "\r"
            print_success "ComfyUI is Running ($ready_replicas/$REPLICAS replicas ready)"
            return 0
        fi

        i=$(( (i+1) %10 ))
        echo -ne "\r  ${CYAN}${spin:$i:1}${NC} Phase: ${YELLOW}${phase}${NC} | Replicas: ${ready_replicas}/${REPLICAS} | Elapsed: ${elapsed}s    "
        sleep 5
        elapsed=$((elapsed + 5))
    done

    echo ""
    print_warning "Timeout waiting for ComfyUI. Check status manually:"
    echo -e "  ${DIM}$K8S_CLI get comfyui comfyui -n $NAMESPACE${NC}"
    echo -e "  ${DIM}$K8S_CLI get pods -n $NAMESPACE${NC}"
}

# Show tenant info
show_tenant_info() {
    if [ "$DRY_RUN" = true ]; then
        log_info "Dry run completed. No resources were created."
        return
    fi

    log_step "Tenant Information"

    # Get actual routes from cluster
    local comfyui_route=$($K8S_CLI get route comfyui -n "$NAMESPACE" -o jsonpath='{.spec.host}' 2>/dev/null || echo "")
    local filebrowser_route=$($K8S_CLI get route comfyui-filebrowser -n "$NAMESPACE" -o jsonpath='{.spec.host}' 2>/dev/null || echo "")
    local phase=$($K8S_CLI get comfyui comfyui -n "$NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")

    # Build URLs
    local comfyui_url=""
    local filebrowser_url=""
    local filebrowser_oauth_redirect=""

    if [ -n "$comfyui_route" ]; then
        comfyui_url="https://${comfyui_route}"
    fi

    if [ -n "$filebrowser_route" ]; then
        filebrowser_url="https://${filebrowser_route}"
        filebrowser_oauth_redirect="https://${filebrowser_route}/oauth2/callback"
    fi

    echo -e ""
    echo -e "${GREEN}✓ Tenant provisioned successfully!${NC}"
    echo -e ""
    echo -e "Namespace:        $NAMESPACE"
    echo -e "ComfyUI Status:   $phase"
    echo -e "Tenant Name:      $TENANT_NAME"
    echo -e ""
    echo -e "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo -e "${BLUE}Access URLs:${NC}"
    echo -e "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    if [ "$ENABLE_OAUTH" = true ]; then
        echo -e ""
        echo -e "  ${GREEN}ComfyUI Application:${NC}"
        echo -e "    $comfyui_url"
        echo -e ""
        echo -e "  ${GREEN}Filebrowser (OAuth2 Protected):${NC}"
        echo -e "    $filebrowser_url"
        echo -e ""
        echo -e "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo -e "${YELLOW}OAuth2 Configuration:${NC}"
        echo -e "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo -e ""
        echo -e "  ${GREEN}Add this Redirect URI to Google OAuth:${NC}"
        echo -e "    ${RED}$filebrowser_oauth_redirect${NC}"
        echo -e ""
        echo -e "  Steps:"
        echo -e "    1. Go to: https://console.cloud.google.com/apis/credentials"
        echo -e "    2. Edit your OAuth 2.0 Client ID"
        echo -e "    3. Under \"Authorized redirect URIs\", click \"ADD URI\""
        echo -e "    4. Paste the URL above"
        echo -e "    5. Click \"Save\""
        echo -e ""
        echo -e "  Allowed Users:"
        echo -e "    - Email: ${OAUTH_EMAILS[@]:-none}"
        echo -e "    - Domain: $OAUTH_DOMAIN"
        echo -e ""
        echo -e "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    else
        echo -e ""
        echo -e "  ${GREEN}ComfyUI Application:${NC}"
        echo -e "    $comfyui_url"
        echo -e ""
        echo -e "  ${GREEN}Filebrowser:${NC}"
        echo -e "    $filebrowser_url"
        echo -e ""
        echo -e "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    fi

    cat <<EOF
${BLUE}Management Commands:${NC}
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  View status:    $K8S_CLI get comfyui comfyui -n $NAMESPACE
  View logs:      $K8S_CLI logs -n $NAMESPACE -l app.kubernetes.io/name=comfyui
  View all:       $K8S_CLI get all -n $NAMESPACE
  Delete tenant:  $K8S_CLI delete namespace $NAMESPACE

  Grant admin access to user:
    $K8S_CLI adm groups add-users tenant-${TENANT_NAME}-admins <username>

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
EOF
}

# Main execution
main() {
    print_banner
    check_prerequisites
    parse_args "$@"
    show_config

    print_header "Provisioning Tenant: $TENANT_NAME"

    local step=1
    local total=7

    print_step $step $total "Creating namespace"
    create_namespace
    ((step++))

    print_step $step $total "Configuring image pull permissions"
    grant_image_pull_permissions
    ((step++))

    print_step $step $total "Creating OAuth2 secrets"
    create_oauth_secret
    ((step++))

    print_step $step $total "Creating resource quotas"
    create_resource_quota
    ((step++))

    print_step $step $total "Configuring RBAC"
    create_rbac
    ((step++))

    print_step $step $total "Deploying ComfyUI instance"
    create_comfyui
    ((step++))

    print_step $step $total "Waiting for deployment"
    wait_for_ready

    show_tenant_info
}

# Run main
main "$@"
