#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/ingress-to-envoy-gateway.sh -n NAMESPACE INGRESS [options]

Converts one Kubernetes Ingress into Gateway API resources for Envoy Gateway.
By default the script prints YAML. Use --apply to apply it.

Options:
  --gateway NAME                 Gateway name. Default: INGRESS-gateway
  --gateway-namespace NAMESPACE  Gateway namespace. Default: Ingress namespace,
                                  or edge-gateway with --use-existing-gateway
  --use-existing-gateway         Do not emit a Gateway; attach routes to --gateway
  --gateway-class NAME           GatewayClass name. Default: eg
  --issuer NAME                  ClusterIssuer name for generated Certificates. Default: letsencrypt-prod
  --emit-certificate             Emit cert-manager Certificate resources for Ingress TLS secrets.
                                  With --use-existing-gateway, Certificates are emitted in
                                  the Gateway namespace and --apply patches the Gateway
                                  HTTPS listener certificateRefs.
  --force-tls                    Route to the HTTPS listener even if the Ingress has no TLS block
  --tls-secret NAME              TLS Secret/Certificate name to use with --force-tls.
                                  This requests an individual ACME certificate unless
                                  --no-emit-certificate is also set.
  --wildcard-tls-secret NAME     Existing wildcard TLS Secret for --force-tls when
                                  --tls-secret is not set. Default: wildcard-uvoo-io-tls
  --no-emit-certificate          Do not emit cert-manager Certificate resources
  --backend-tls none|system|ca   Handle nginx backend-protocol=HTTPS. Default: none
  --backend-tls-hostname NAME    SNI/validation hostname for BackendTLSPolicy. Default: first Ingress host
  --backend-tls-ca-configmap NAME
                                  ConfigMap with ca.crt when --backend-tls=ca
  --apply                        Apply generated YAML instead of printing it
  -h, --help                     Show this help

Examples:
  # Preview conversion for uapp/ucontrol-ui.
  scripts/ingress-to-envoy-gateway.sh -n uapp ucontrol-ui --backend-tls system

  # Apply conversion.
  scripts/ingress-to-envoy-gateway.sh -n uapp ucontrol-ui --backend-tls system --apply

  # Attach uapp/ucontrol-ui to an existing central Gateway.
  scripts/ingress-to-envoy-gateway.sh -n uapp ucontrol-ui \
    --use-existing-gateway \
    --gateway uvoo-gateway \
    --gateway-namespace edge-gateway \
    --backend-tls system

  # Attach an HTTP-only Ingress to the central Gateway and use the default
  # wildcard certificate as a fallback.
  scripts/ingress-to-envoy-gateway.sh -n grafana grafana \
    --use-existing-gateway \
    --gateway uvoo-gateway \
    --force-tls \
    --apply

  # Prefer an individual Let's Encrypt certificate for the same case.
  scripts/ingress-to-envoy-gateway.sh -n grafana grafana \
    --use-existing-gateway \
    --gateway uvoo-gateway \
    --force-tls \
    --tls-secret grafana-tls \
    --apply

Notes:
  - The original Ingress is not changed or deleted.
  - The shared Gateway namespace should hold TLS Secrets and should admit
    HTTPRoutes with an allowedRoutes namespace selector, not from: All.
  - nginx.ingress.kubernetes.io/backend-protocol=HTTPS requires BackendTLSPolicy.
    Use --backend-tls system for publicly trusted backend certs, or --backend-tls ca
    with --backend-tls-ca-configmap for private CAs.
USAGE
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

default_kubectl() {
  if [[ -x /snap/kubectl/current/kubectl ]]; then
    printf '%s\n' /snap/kubectl/current/kubectl
  else
    printf '%s\n' kubectl
  fi
}

yaml_quote() {
  printf '%s' "$1" | sed "s/'/''/g; s/^/'/; s/$/'/"
}

resource_name() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9.-]+/-/g; s/^-+//; s/-+$//' | cut -c1-63
}

resolve_service_port() {
  local svc="$1"
  local port_name="$2"

  "$kubectl_cmd" get svc -n "$namespace" "$svc" -o json |
    jq -r --arg name "$port_name" '
      .spec.ports[] | select(.name == $name) | .port
    ' | head -n 1
}

resolve_service_port_number() {
  local svc="$1"
  local port_number="$2"
  local resolved=""

  resolved="$(
    "$kubectl_cmd" get svc -n "$namespace" "$svc" -o json |
      jq -r --argjson port "$port_number" '
        .spec.ports[] | select(.port == $port) | .port
      ' | head -n 1
  )"
  if [[ -n "$resolved" ]]; then
    printf '%s\n' "$resolved"
    return
  fi

  resolved="$(
    "$kubectl_cmd" get svc -n "$namespace" "$svc" -o json |
      jq -r --arg port "$port_number" '
        .spec.ports[] | select((.targetPort | tostring) == $port) | .port
      ' | head -n 1
  )"
  if [[ -n "$resolved" ]]; then
    printf 'warning: %s/%s backend references targetPort %s; using Service port %s for HTTPRoute.\n' "$namespace" "$svc" "$port_number" "$resolved" >&2
    printf '%s\n' "$resolved"
  fi
}

gateway_has_cert_ref() {
  local secret="$1"

  "$kubectl_cmd" get gateway -n "$gateway_namespace" "$gateway" -o json |
    jq -e --arg secret "$secret" '
      .spec.listeners[]
      | select(.name == "https")
      | (.tls.certificateRefs // [])
      | any(.name == $secret)
    ' >/dev/null
}

patch_gateway_cert_ref() {
  local secret="$1"

  if gateway_has_cert_ref "$secret"; then
    printf 'Gateway %s/%s already references TLS secret %s\n' "$gateway_namespace" "$gateway" "$secret" >&2
    return
  fi

  "$kubectl_cmd" patch gateway "$gateway" -n "$gateway_namespace" --type json \
    -p="[{\"op\":\"add\",\"path\":\"/spec/listeners/1/tls/certificateRefs/-\",\"value\":{\"name\":\"$secret\"}}]"
}

namespace=""
ingress=""
gateway=""
gateway_namespace=""
create_gateway="true"
gateway_class="eg"
issuer="letsencrypt-prod"
emit_certificate="false"
emit_certificate_forced="false"
wildcard_tls_secret="${ENVOY_WILDCARD_TLS_SECRET:-wildcard-uvoo-io-tls}"
backend_tls="none"
backend_tls_hostname=""
backend_tls_ca_configmap=""
force_tls="false"
tls_secret_override=""
tls_secret_from_wildcard="false"
apply="false"
kubectl_cmd="${KUBECTL:-$(default_kubectl)}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -n|--namespace)
      namespace="${2:-}"
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
    --use-existing-gateway)
      create_gateway="false"
      shift
      ;;
    --gateway-class)
      gateway_class="${2:-}"
      shift 2
      ;;
    --issuer)
      issuer="${2:-}"
      shift 2
      ;;
    --emit-certificate)
      emit_certificate="true"
      emit_certificate_forced="true"
      shift
      ;;
    --no-emit-certificate)
      emit_certificate="false"
      emit_certificate_forced="true"
      shift
      ;;
    --force-tls)
      force_tls="true"
      shift
      ;;
    --tls-secret)
      tls_secret_override="${2:-}"
      shift 2
      ;;
    --wildcard-tls-secret)
      wildcard_tls_secret="${2:-}"
      shift 2
      ;;
    --backend-tls)
      backend_tls="${2:-}"
      shift 2
      ;;
    --backend-tls-hostname)
      backend_tls_hostname="${2:-}"
      shift 2
      ;;
    --backend-tls-ca-configmap)
      backend_tls_ca_configmap="${2:-}"
      shift 2
      ;;
    --apply)
      apply="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    -*)
      die "unknown option: $1"
      ;;
    *)
      if [[ -n "$ingress" ]]; then
        die "unexpected extra argument: $1"
      fi
      ingress="$1"
      shift
      ;;
  esac
done

[[ -n "$namespace" ]] || die "namespace is required"
[[ -n "$ingress" ]] || die "ingress name is required"
case "$backend_tls" in
  none|system|ca) ;;
  *) die "--backend-tls must be one of: none, system, ca" ;;
esac
if [[ "$backend_tls" == "ca" && -z "$backend_tls_ca_configmap" ]]; then
  die "--backend-tls-ca-configmap is required when --backend-tls=ca"
fi
need jq
need sed
need tr
need cut
need head
if [[ "$kubectl_cmd" == */* ]]; then
  [[ -x "$kubectl_cmd" ]] || die "kubectl is not executable: $kubectl_cmd"
else
  need "$kubectl_cmd"
fi

gateway="${gateway:-$(resource_name "${ingress}-gateway")}"
if [[ -z "$gateway_namespace" ]]; then
  if [[ "$create_gateway" == "false" ]]; then
    gateway_namespace="${ENVOY_SHARED_GATEWAY_NAMESPACE:-edge-gateway}"
  else
    gateway_namespace="$namespace"
  fi
fi

ingress_json="$("$kubectl_cmd" get ingress -n "$namespace" "$ingress" -o json)"

mapfile -t hosts < <(jq -r '.spec.rules[]?.host // empty' <<<"$ingress_json" | awk 'NF' | sort -u)
[[ "${#hosts[@]}" -gt 0 ]] || die "Ingress has no host rules"

if [[ -z "$backend_tls_hostname" ]]; then
  backend_tls_hostname="${hosts[0]}"
fi

mapfile -t tls_secrets < <(jq -r '.spec.tls[]?.secretName // empty' <<<"$ingress_json" | awk 'NF' | sort -u)
if [[ -n "$tls_secret_override" ]]; then
  tls_secrets=("$tls_secret_override")
elif [[ "$force_tls" == "true" && "${#tls_secrets[@]}" -eq 0 ]]; then
  tls_secrets=("$wildcard_tls_secret")
  tls_secret_from_wildcard="true"
fi
has_tls="false"
if [[ "${#tls_secrets[@]}" -gt 0 || "$force_tls" == "true" ]]; then
  has_tls="true"
fi
if [[ "$emit_certificate_forced" == "false" && "$create_gateway" == "false" && "$has_tls" == "true" && "$tls_secret_from_wildcard" == "false" ]]; then
  emit_certificate="true"
fi

ssl_redirect="$(
  jq -r '
    .metadata.annotations["nginx.ingress.kubernetes.io/ssl-redirect"] //
    .metadata.annotations["nginx.ingress.kubernetes.io/force-ssl-redirect"] //
    "false"
  ' <<<"$ingress_json"
)"
backend_protocol="$(jq -r '.metadata.annotations["nginx.ingress.kubernetes.io/backend-protocol"] // "HTTP"' <<<"$ingress_json" | tr '[:lower:]' '[:upper:]')"

if [[ "$backend_protocol" == "HTTPS" && "$backend_tls" == "none" ]]; then
  printf 'warning: %s/%s uses nginx backend-protocol=HTTPS; generated route will need BackendTLSPolicy or backend HTTP support.\n' "$namespace" "$ingress" >&2
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

{
  if [[ "$create_gateway" == "true" ]]; then
    cat <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: $gateway
  namespace: $gateway_namespace
  labels:
    app.kubernetes.io/managed-by: ingress-to-envoy-gateway
    app.kubernetes.io/source-ingress: $ingress
spec:
  gatewayClassName: $gateway_class
  listeners:
    - name: http
      port: 80
      protocol: HTTP
      allowedRoutes:
        namespaces:
          from: Same
EOF

    if [[ "$has_tls" == "true" ]]; then
      cat <<'EOF'
    - name: https
      port: 443
      protocol: HTTPS
      tls:
        mode: Terminate
        certificateRefs:
EOF
      for secret in "${tls_secrets[@]}"; do
        printf '          - name: %s\n' "$secret"
      done
      cat <<'EOF'
      allowedRoutes:
        namespaces:
          from: Same
EOF
    fi
    printf '%s\n' '---'
  fi

  cat <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: $ingress
  namespace: $namespace
  labels:
    app.kubernetes.io/managed-by: ingress-to-envoy-gateway
    app.kubernetes.io/source-ingress: $ingress
spec:
  hostnames:
EOF
  for host in "${hosts[@]}"; do
    printf '    - %s\n' "$(yaml_quote "$host")"
  done
  cat <<EOF
  parentRefs:
    - name: $gateway
      namespace: $gateway_namespace
      sectionName: $([[ "$has_tls" == "true" ]] && printf 'https' || printf 'http')
  rules:
EOF

  route_count="$(jq '[.spec.rules[]?.http.paths[]?] | length' <<<"$ingress_json")"
  [[ "$route_count" != "0" ]] || die "Ingress has no HTTP paths"

  for i in $(seq 0 "$((route_count - 1))"); do
    item="$(jq -c "[.spec.rules[]?.http.paths[]?][$i]" <<<"$ingress_json")"
    path="$(jq -r '.path // "/"' <<<"$item")"
    path_type="$(jq -r '.pathType // "Prefix"' <<<"$item")"
    svc="$(jq -r '.backend.service.name // empty' <<<"$item")"
    port_number="$(jq -r '.backend.service.port.number // empty' <<<"$item")"
    port_name="$(jq -r '.backend.service.port.name // empty' <<<"$item")"

    [[ -n "$svc" ]] || die "path $i has no Service backend"
    if [[ -z "$port_number" && -n "$port_name" ]]; then
      port_number="$(resolve_service_port "$svc" "$port_name")"
      [[ -n "$port_number" ]] || die "could not resolve named port $svc/$port_name"
    elif [[ -n "$port_number" ]]; then
      resolved_port_number="$(resolve_service_port_number "$svc" "$port_number")"
      [[ -n "$resolved_port_number" ]] || die "Service $namespace/$svc does not expose port $port_number"
      port_number="$resolved_port_number"
    fi
    [[ -n "$port_number" ]] || die "path $i has no Service port"

    case "$path_type" in
      Prefix) gateway_path_type="PathPrefix" ;;
      Exact) gateway_path_type="Exact" ;;
      ImplementationSpecific) gateway_path_type="PathPrefix" ;;
      *) gateway_path_type="PathPrefix" ;;
    esac

    cat <<EOF
    - matches:
        - path:
            type: $gateway_path_type
            value: $(yaml_quote "$path")
      backendRefs:
        - name: $svc
          port: $port_number
EOF
  done

  if [[ "$has_tls" == "true" ]] && { [[ "$ssl_redirect" == "true" ]] || [[ "$force_tls" == "true" ]]; }; then
    cat <<EOF
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: $(resource_name "${ingress}-https-redirect")
  namespace: $namespace
  labels:
    app.kubernetes.io/managed-by: ingress-to-envoy-gateway
    app.kubernetes.io/source-ingress: $ingress
spec:
  hostnames:
EOF
    for host in "${hosts[@]}"; do
      printf '    - %s\n' "$(yaml_quote "$host")"
    done
    cat <<EOF
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
  fi

  if [[ "$backend_protocol" == "HTTPS" && "$backend_tls" != "none" ]]; then
    mapfile -t https_services < <(
      jq -r '.spec.rules[]?.http.paths[]?.backend.service.name // empty' <<<"$ingress_json" |
        awk 'NF' | sort -u
    )
    for svc in "${https_services[@]}"; do
      cat <<EOF
---
apiVersion: gateway.networking.k8s.io/v1
kind: BackendTLSPolicy
metadata:
  name: $(resource_name "${ingress}-${svc}-backend-tls")
  namespace: $namespace
  labels:
    app.kubernetes.io/managed-by: ingress-to-envoy-gateway
    app.kubernetes.io/source-ingress: $ingress
spec:
  targetRefs:
    - group: ""
      kind: Service
      name: $svc
  validation:
    hostname: $(yaml_quote "$backend_tls_hostname")
EOF
      if [[ "$backend_tls" == "system" ]]; then
        cat <<'EOF'
    wellKnownCACertificates: System
EOF
      else
        cat <<EOF
    caCertificateRefs:
      - group: ""
        kind: ConfigMap
        name: $backend_tls_ca_configmap
EOF
      fi
    done
  fi

  if [[ "$emit_certificate" == "true" && "$has_tls" == "true" ]]; then
    for secret in "${tls_secrets[@]}"; do
      mapfile -t cert_hosts < <(
        jq -r --arg secret "$secret" '
          .spec.tls[]? | select(.secretName == $secret) | .hosts[]?
        ' <<<"$ingress_json" | awk 'NF' | sort -u
      )
      [[ "${#cert_hosts[@]}" -gt 0 ]] || cert_hosts=("${hosts[@]}")
      cat <<EOF
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: $secret
  namespace: $([[ "$create_gateway" == "false" ]] && printf '%s' "$gateway_namespace" || printf '%s' "$namespace")
  labels:
    app.kubernetes.io/managed-by: ingress-to-envoy-gateway
    app.kubernetes.io/source-ingress: $ingress
spec:
  secretName: $secret
  dnsNames:
EOF
      for host in "${cert_hosts[@]}"; do
        printf '    - %s\n' "$(yaml_quote "$host")"
      done
      cat <<EOF
  issuerRef:
    group: cert-manager.io
    kind: ClusterIssuer
    name: $issuer
EOF
    done
  fi
} >"$tmp"

if [[ "$apply" == "true" ]]; then
  "$kubectl_cmd" apply -f "$tmp"
  if [[ "$create_gateway" == "false" && "$has_tls" == "true" ]]; then
    for secret in "${tls_secrets[@]}"; do
      patch_gateway_cert_ref "$secret"
    done
  fi
else
  cat "$tmp"
fi
