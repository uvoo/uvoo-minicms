#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/hello-edge-enable-mtls.sh --yes [options]

Upgrades the hello-edge example to end-to-end TLS with Envoy-to-backend mTLS
and a Cilium allow policy. This changes live routing for the hello app.

Prerequisites:
  - cert-manager ClusterIssuer/uvoo-internal-ca exists
  - cert-manager Secret cert-manager/uvoo-internal-root-ca exists
  - Envoy Gateway Backend extension API is enabled

Options:
  --yes                    Required. Acknowledge traffic-affecting changes.
  --namespace NAME          Namespace to update. Default: hello-edge
  --host HOST               Public hostname. Default: hello.uvoo.io
  --gateway NAME            Shared Gateway name. Default: uvoo-gateway
  --gateway-namespace NAME  Shared Gateway namespace. Default: edge-gateway
  --envoy-namespace NAME    Envoy data-plane namespace. Default: envoy-gateway-system
  --issuer NAME             Internal ClusterIssuer. Default: uvoo-internal-ca
  --root-ca-namespace NAME  Namespace containing root CA Secret. Default: cert-manager
  --root-ca-secret NAME     Root CA Secret. Default: uvoo-internal-root-ca
  --replicas N              Deployment replicas. Default: 2
  --text TEXT               Response text. Default: hello from gateway
  --dry-run                 Print YAML instead of applying it
  -h, --help                Show this help

To enable the required Envoy Gateway Backend API:
  helm upgrade eg oci://docker.io/envoyproxy/gateway-helm \
    --version v1.8.1 \
    -n envoy-gateway-system \
    --reuse-values \
    --set config.envoyGateway.extensionApis.enableBackend=true
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
envoy_namespace="envoy-gateway-system"
issuer="uvoo-internal-ca"
root_ca_namespace="cert-manager"
root_ca_secret="uvoo-internal-root-ca"
replicas="2"
text="hello from gateway"
dry_run="false"
confirmed="false"
kubectl_cmd="${KUBECTL:-$(default_kubectl)}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --yes)
      confirmed="true"
      shift
      ;;
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
    --envoy-namespace)
      envoy_namespace="${2:-}"
      shift 2
      ;;
    --issuer)
      issuer="${2:-}"
      shift 2
      ;;
    --root-ca-namespace)
      root_ca_namespace="${2:-}"
      shift 2
      ;;
    --root-ca-secret)
      root_ca_secret="${2:-}"
      shift 2
      ;;
    --replicas)
      replicas="${2:-}"
      shift 2
      ;;
    --text)
      text="${2:-}"
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

if [[ "$confirmed" != "true" && "$dry_run" != "true" ]]; then
  die "this changes live hello routing and Cilium policy; rerun with --yes"
fi

if [[ "$dry_run" != "true" ]]; then
  "$kubectl_cmd" get clusterissuer "$issuer" >/dev/null
  "$kubectl_cmd" -n "$root_ca_namespace" get secret "$root_ca_secret" >/dev/null

  enable_backend="$(
    "$kubectl_cmd" -n "$envoy_namespace" get configmap envoy-gateway-config \
      -o jsonpath='{.data.envoy-gateway\.yaml}' |
      grep -E 'enableBackend:[[:space:]]*true' || true
  )"
  if [[ -z "$enable_backend" ]]; then
    die "Envoy Gateway Backend API is not enabled. See --help for the helm upgrade command."
  fi
fi

manifest="$(mktemp)"
ca_tmp=""
trap 'rm -f "$manifest" "$ca_tmp"' EXIT
ca_tmp="$(mktemp)"

cat >"$manifest" <<EOF
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: hello-backend-tls
  namespace: $namespace
spec:
  secretName: hello-backend-tls
  duration: 2160h
  renewBefore: 720h
  privateKey:
    algorithm: ECDSA
    size: 256
    rotationPolicy: Always
  usages:
    - digital signature
    - key encipherment
    - server auth
  dnsNames:
    - hello
    - hello.$namespace
    - hello.$namespace.svc
    - hello.$namespace.svc.cluster.local
    - $host
  issuerRef:
    group: cert-manager.io
    kind: ClusterIssuer
    name: $issuer
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: hello-envoy-client-tls
  namespace: $namespace
spec:
  secretName: hello-envoy-client-tls
  duration: 2160h
  renewBefore: 720h
  privateKey:
    algorithm: ECDSA
    size: 256
    rotationPolicy: Always
  usages:
    - digital signature
    - key encipherment
    - client auth
  commonName: envoy-gateway.$gateway_namespace
  dnsNames:
    - envoy-gateway.$gateway_namespace
  issuerRef:
    group: cert-manager.io
    kind: ClusterIssuer
    name: $issuer
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: hello-caddy
  namespace: $namespace
data:
  Caddyfile: |
    {
      auto_https off
    }

    :8443 {
      tls /etc/server-tls/tls.crt /etc/server-tls/tls.key {
        client_auth {
          mode require_and_verify
          trusted_ca_cert_file /etc/client-ca/ca.crt
        }
      }

      reverse_proxy 127.0.0.1:5678
    }
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
              name: app-http
        - name: tls-proxy
          image: caddy:2-alpine
          ports:
            - containerPort: 8443
              name: https
          volumeMounts:
            - name: caddy-config
              mountPath: /etc/caddy
            - name: server-tls
              mountPath: /etc/server-tls
              readOnly: true
            - name: client-ca
              mountPath: /etc/client-ca
              readOnly: true
      volumes:
        - name: caddy-config
          configMap:
            name: hello-caddy
        - name: server-tls
          secret:
            secretName: hello-backend-tls
        - name: client-ca
          configMap:
            name: uvoo-internal-root-ca
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
    - name: https
      port: 443
      targetPort: 8443
      protocol: TCP
---
apiVersion: gateway.envoyproxy.io/v1alpha1
kind: Backend
metadata:
  name: hello-mtls
  namespace: $namespace
spec:
  endpoints:
    - fqdn:
        hostname: hello.$namespace.svc.cluster.local
        port: 443
  tls:
    sni: $host
    caCertificateRefs:
      - group: ""
        kind: ConfigMap
        name: uvoo-internal-root-ca
    clientCertificateRef:
      group: ""
      kind: Secret
      name: hello-envoy-client-tls
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
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - group: gateway.envoyproxy.io
          kind: Backend
          name: hello-mtls
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
---
apiVersion: cilium.io/v2
kind: CiliumNetworkPolicy
metadata:
  name: hello-from-shared-envoy
  namespace: $namespace
spec:
  endpointSelector:
    matchLabels:
      app: hello
  ingress:
    - fromEndpoints:
        - matchLabels:
            "k8s:io.kubernetes.pod.namespace": $envoy_namespace
            "k8s:app.kubernetes.io/name": envoy
            "k8s:gateway.envoyproxy.io/owning-gateway-name": $gateway
            "k8s:gateway.envoyproxy.io/owning-gateway-namespace": $gateway_namespace
      toPorts:
        - ports:
            - port: "8443"
              protocol: TCP
EOF

if [[ "$dry_run" == "true" ]]; then
  cat "$manifest"
  exit 0
fi

"$kubectl_cmd" -n "$root_ca_namespace" get secret "$root_ca_secret" \
  -o jsonpath='{.data.tls\.crt}' | base64 -d >"$ca_tmp"
"$kubectl_cmd" create namespace "$namespace" --dry-run=client -o yaml | "$kubectl_cmd" apply -f -
"$kubectl_cmd" label namespace "$namespace" shared-gateway-access=true --overwrite
"$kubectl_cmd" -n "$namespace" create configmap uvoo-internal-root-ca \
  --from-file=ca.crt="$ca_tmp" \
  --dry-run=client -o yaml | "$kubectl_cmd" apply -f -

"$kubectl_cmd" apply -f "$manifest"
"$kubectl_cmd" -n "$namespace" wait --for=condition=Ready certificate/hello-backend-tls --timeout=120s
"$kubectl_cmd" -n "$namespace" wait --for=condition=Ready certificate/hello-envoy-client-tls --timeout=120s
"$kubectl_cmd" -n "$namespace" rollout status deployment/hello --timeout=180s
"$kubectl_cmd" -n "$namespace" get httproute hello hello-https-redirect
"$kubectl_cmd" -n "$namespace" get backend hello-mtls
"$kubectl_cmd" -n "$namespace" get ciliumnetworkpolicy hello-from-shared-envoy
