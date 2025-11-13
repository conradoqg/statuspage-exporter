#!/usr/bin/env bash
set -euo pipefail

IMAGE=${IMAGE:-statuspage-exporter:latest}

echo "Applying statuspage-exporter manifests..."
kubectl create namespace statuspage --dry-run=client -o yaml | kubectl apply -f -
kubectl -n statuspage apply -f k8s/statuspage-exporter.yaml

echo "If using Minikube, you can load the image via:\n  minikube image load ${IMAGE}"

