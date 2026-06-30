#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/deploy-hello-edge.sh [options]

Creates a basic HTTP backend exposed through the shared Envoy Gateway.

Options:
  --namespace NAME          Namespace to create/use. Default: hello-edge
  --host HOST               Public hostname. Default: hello.uvoo.io
  --gateway NAME            Shared Gateway name. Default: uvoo-gateway
  --gateway-namespace NAME  Shared Gateway namespace. Default: edge-gateway
  --text TEXT               Response text. Default: hello from gateway
  --replicas N              Deployment replicas. Default: 2
  --dry-run                 Print YAML instead of applying it
  -h, --help                Show this help
USAGE
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

default_kubectl() {
  if [[ -x /snap/kubectl/current/kubectl ]]; then
    printf '%s\n' /snap/kubectl/current/kubectl
  else
    printf '%s\n' kubectl
  fi
}

namespace="hello-edge"
host="hello.uvoo.io"
gateway="uvoo-gateway"
gateway_namespace="edge-gateway"
text="hello from gateway"
replicas="2"
dry_run="false"
kubectl_cmd="${KUBECTL:-$(default_kubectl)}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --namespace)
      namespace="${2:-}"
      shift 2
      ;;
    --host)
      host="${2:-}"
      shift 2
      ;;
    --gateway)
      gateway="${2:-}"
      shift 2
      ;;
    --gateway-namespace)
      gateway_namespace="${2:-}"
      shift 2
      ;;
    --text)
      text="${2:-}"
      shift 2
      ;;
    --replicas)
      replicas="${2:-}"
      shift 2
      ;;
    --dry-run)
      dry_run="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown option: $1"
      ;;
  esac
done

[[ -n "$namespace" ]] || die "--namespace cannot be empty"
[[ -n "$host" ]] || die "--host cannot be empty"
[[ -n "$gateway" ]] || die "--gateway cannot be empty"
[[ -n "$gateway_namespace" ]] || die "--gateway-namespace cannot be empty"
[[ "$replicas" =~ ^[0-9]+$ ]] || die "--replicas must be a number"

manifest="$(mktemp)"
trap 'rm -f "$manifest"' EXIT

cat >"$manifest" <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: $namespace
  labels:
    shared-gateway-access: "true"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hello
  namespace: $namespace
spec:
  replicas: $replicas
  selector:
    matchLabels:
      app: hello
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0
      maxSurge: 1
  template:
    metadata:
      labels:
        app: hello
    spec:
      containers:
        - name: hello
          image: hashicorp/http-echo:1.0
          args:
            - -text=$text
          ports:
            - containerPort: 5678
              name: http
---
apiVersion: v1
kind: Service
metadata:
  name: hello
  namespace: $namespace
spec:
  selector:
    app: hello
  ports:
    - name: http
      port: 80
      targetPort: 5678
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: hello
  namespace: $namespace
spec:
  hostnames:
    - $host
  parentRefs:
    - name: $gateway
      namespace: $gateway_namespace
      sectionName: https
  rules:
    - backendRefs:
        - name: hello
          port: 80
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: hello-https-redirect
  namespace: $namespace
spec:
  hostnames:
    - $host
  parentRefs:
    - name: $gateway
      namespace: $gateway_namespace
      sectionName: http
  rules:
    - filters:
        - type: RequestRedirect
          requestRedirect:
            scheme: https
            statusCode: 301
      matches:
        - path:
            type: PathPrefix
            value: /
EOF

if [[ "$dry_run" == "true" ]]; then
  cat "$manifest"
else
  "$kubectl_cmd" apply -f "$manifest"
  "$kubectl_cmd" -n "$namespace" rollout status deployment/hello --timeout=180s
  "$kubectl_cmd" -n "$namespace" get httproute hello hello-https-redirect
fi
