#!/bin/bash
# Helper script to create Kubernetes secret for weather-service
# Usage: ./create-secret.sh YOUR_API_KEY

set -e

API_KEY="$1"

if [ -z "$API_KEY" ]; then
    echo "Error: API key is required"
    echo "Usage: $0 YOUR_OPENWEATHERMAP_API_KEY"
    echo ""
    echo "Example:"
    echo "  $0 abc123def456"
    exit 1
fi

# Validate API key format (basic check - at least 8 characters)
if [ ${#API_KEY} -lt 8 ]; then
    echo "Error: API key seems too short (expected at least 8 characters)"
    exit 1
fi

echo "Creating Kubernetes secret 'weather-api-secret'..."

# Create the secret
kubectl create secret generic weather-api-secret \
    --from-literal=api-key="$API_KEY" \
    --dry-run=client -o yaml | kubectl apply -f -

echo "✓ Secret created successfully"
echo ""
echo "Next steps:"
echo "  1. Deploy the application: kubectl apply -f deployments/kubernetes/deployment.yaml"
echo "  2. Verify pods are running: kubectl get pods -l app=weather-service"
echo "  3. Access the service: kubectl port-forward svc/weather-service 8080:80"
