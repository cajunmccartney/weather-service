# Kubernetes Deployment with Kustomize

This directory uses Kustomize to manage API version-specific deployments.

## Directory Structure

```
deployments/kubernetes/
├── base/
│   ├── deployment.yaml       # Base deployment (no API version)
│   └── kustomization.yaml    # Base kustomization
├── overlays/
│   ├── api-2.5/
│   │   ├── kustomization.yaml
│   │   └── patch-version.yaml  # Sets WEATHER_API_VERSION=2.5
│   └── api-3.0/
│       ├── kustomization.yaml
│       └── patch-version.yaml  # Sets WEATHER_API_VERSION=3.0
└── deployment.yaml           # Legacy file (kept for backward compatibility)
```

## Usage

### Deploy with API 2.5 (Default - Free)

```bash
kubectl apply -k deployments/kubernetes/overlays/api-2.5
```

### Deploy with API 3.0 (Requires Payment Method)

```bash
kubectl apply -k deployments/kubernetes/overlays/api-3.0
```

### View Generated Manifest (without applying)

```bash
# View 2.5 manifest
kubectl kustomize deployments/kubernetes/overlays/api-2.5

# View 3.0 manifest
kubectl kustomize deployments/kubernetes/overlays/api-3.0
```

## How It Works

1. **Base**: Contains the core deployment without API version
2. **Overlays**: Reference the base and apply patches to set the `WEATHER_API_VERSION` environment variable
3. **Kustomize**: Merges base + overlay at deployment time

**Note:** Overlays use `resources` (not the deprecated `bases`) to reference the base directory.

## Benefits

- ✅ No manual file editing required
- ✅ Clean separation of base config and version-specific settings
- ✅ Easy to add new API versions (just create a new overlay)
- ✅ Version-controlled configuration
- ✅ Standard Kubernetes practice

## Legacy Deployment

The root `deployment.yaml` file is kept for backward compatibility but is now deprecated. Use the kustomize overlays instead.
