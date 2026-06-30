# Envoy Gateway Architecture

This cluster uses Envoy Gateway as the shared public edge for `*.uvoo.io`.

## Namespaces

- `envoy-gateway-system`: Envoy Gateway controller and generated Envoy data-plane Deployments and Services.
- `edge-gateway`: shared `Gateway` objects and public TLS Secrets.
- application namespaces such as `uvoo-io`, `uapp`, and `grafana`: application Deployments, Services, HTTPRoutes, and BackendTLSPolicies.

Keep application workloads out of `edge-gateway`. That namespace is the TLS and edge-routing control plane.

## Shared Gateway

The public shared Gateway is:

```text
edge-gateway/uvoo-gateway
```

The generated Envoy LoadBalancer Service runs in `envoy-gateway-system` and should keep the firewall-routed address:

```text
192.168.238.16
```

The Gateway listeners should admit routes with a namespace selector instead of `from: All`. App namespaces must be explicitly labeled:

```bash
kubectl label namespace uvoo-io shared-gateway-access=true --overwrite
kubectl label namespace uapp shared-gateway-access=true --overwrite
kubectl label namespace grafana shared-gateway-access=true --overwrite
```

## TLS Policy

Use individual Let's Encrypt certificates for production hostnames when practical. The conversion script emits `Certificate` resources in the Gateway namespace when an Ingress has TLS configured or when `--tls-secret` is supplied.

Use the wildcard certificate as the default fallback for simple migrations and HTTP-only Ingresses:

```text
edge-gateway/wildcard-uvoo-io-tls
```

That Secret is copied from:

```text
ingress-nginx/default-tls-certificate
```

The wildcard covers `*.uvoo.io` and `uvoo.io`. It is useful during migration, but app-specific certificates are easier to rotate, audit, and revoke independently.

## Backend TLS

Use a private cert-manager CA for TLS between Envoy Gateway and backend Services. Public ACME certificates are not a good fit for internal pod endpoints because the backend certificate should identify the Kubernetes Service, rotate automatically, and avoid depending on public DNS validation.

Current internal issuer resources:

```text
ClusterIssuer/uvoo-internal-selfsigned
ClusterIssuer/uvoo-internal-ca
cert-manager/Certificate/uvoo-internal-root-ca
```

For `ucontrol-ws`, the staged backend certificate is:

```text
uapp/Certificate/ucontrol-ws-backend-tls
uapp/Secret/ucontrol-ws-backend-tls
uapp/ConfigMap/uvoo-internal-root-ca
```

The certificate includes these SANs:

```text
ucontrol-ws
ucontrol-ws.uapp
ucontrol-ws.uapp.svc
ucontrol-ws.uapp.svc.cluster.local
uapp-ws.uvoo.io
```

After the workload is serving that Secret, configure Envoy validation with `BackendTLSPolicy`:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: BackendTLSPolicy
metadata:
  name: ucontrol-ws-ucontrol-ws-backend-tls
  namespace: uapp
spec:
  targetRefs:
    - group: ""
      kind: Service
      name: ucontrol-ws
  validation:
    hostname: ucontrol-ws.uapp.svc.cluster.local
    caCertificateRefs:
      - group: ""
        kind: ConfigMap
        name: uvoo-internal-root-ca
```

Do not patch the workload and BackendTLSPolicy in the opposite order. If Envoy starts validating the private CA before the pod serves the matching private cert, public traffic receives 5xx responses.

## Cilium Policy

Cilium should enforce which workloads may reach backend TLS ports. For the shared Envoy Gateway, the source identity is the generated Envoy data-plane pod in `envoy-gateway-system`, not the Envoy Gateway controller.

For `ucontrol-ws`, a restrictive policy should allow only the shared Envoy Gateway data plane to the pod HTTPS port:

```yaml
apiVersion: cilium.io/v2
kind: CiliumNetworkPolicy
metadata:
  name: ucontrol-ws-from-shared-envoy
  namespace: uapp
spec:
  endpointSelector:
    matchLabels:
      app: ucontrol-ws
  ingress:
    - fromEndpoints:
        - matchLabels:
            k8s:io.kubernetes.pod.namespace: envoy-gateway-system
            k8s:app.kubernetes.io/name: envoy
            k8s:gateway.envoyproxy.io/owning-gateway-name: uvoo-gateway
            k8s:gateway.envoyproxy.io/owning-gateway-namespace: edge-gateway
      toPorts:
        - ports:
            - port: "8444"
              protocol: TCP
```

Adding `authentication.mode: required` to that rule is a separate step. It should only be enabled after Cilium mutual-authentication infrastructure is confirmed working, because a missing or broken auth backend can drop otherwise allowed traffic.

## Migrating An Ingress

Preview first:

```bash
scripts/ingress-to-envoy-gateway.sh -n grafana grafana \
  --use-existing-gateway \
  --gateway uvoo-gateway \
  --gateway-namespace edge-gateway \
  --force-tls
```

Apply with the wildcard fallback:

```bash
scripts/ingress-to-envoy-gateway.sh -n grafana grafana \
  --use-existing-gateway \
  --gateway uvoo-gateway \
  --gateway-namespace edge-gateway \
  --force-tls \
  --apply
```

Apply with an individual Let's Encrypt certificate:

```bash
scripts/ingress-to-envoy-gateway.sh -n grafana grafana \
  --use-existing-gateway \
  --gateway uvoo-gateway \
  --gateway-namespace edge-gateway \
  --force-tls \
  --tls-secret grafana-tls \
  --apply
```

For HTTPS backends, add one of:

```bash
--backend-tls system
--backend-tls ca --backend-tls-ca-configmap my-backend-ca
```

## Basic HTTP Backend Example

This creates a small HTTP app in its own namespace and exposes it through the shared Envoy Gateway.

The scripted form is:

```bash
scripts/deploy-hello-edge.sh
```

To upgrade that example to backend TLS, Envoy-to-backend mTLS, and a Cilium allow policy:

```bash
scripts/hello-edge-enable-mtls.sh --yes
```

The mTLS script uses Envoy Gateway's `Backend` extension API, so Envoy Gateway must have `extensionApis.enableBackend=true`. This changes the `hello` route from a Kubernetes `Service` backend to an Envoy Gateway `Backend` backend with `clientCertificateRef`.

Cilium is not required for backend TLS or mTLS. It is used here for network-layer enforcement so only the shared Envoy data-plane pods can connect to the backend TLS port.

For TLS backends, configure SNI deliberately. If a backend server validates or routes based on SNI, the SNI value must match a name in the backend certificate and what the backend expects. For the Caddy-based `hello` example, the backend SNI is `hello.uvoo.io` so it matches the HTTP Host header and avoids Caddy returning `421 Misdirected Request`.

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: hello-edge
  labels:
    shared-gateway-access: "true"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hello
  namespace: hello-edge
spec:
  replicas: 2
  selector:
    matchLabels:
      app: hello
  template:
    metadata:
      labels:
        app: hello
    spec:
      containers:
        - name: hello
          image: hashicorp/http-echo:1.0
          args:
            - -text=hello from envoy
          ports:
            - containerPort: 5678
---
apiVersion: v1
kind: Service
metadata:
  name: hello
  namespace: hello-edge
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
  namespace: hello-edge
spec:
  hostnames:
    - hello.uvoo.io
  parentRefs:
    - name: uvoo-gateway
      namespace: edge-gateway
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
  namespace: hello-edge
spec:
  hostnames:
    - hello.uvoo.io
  parentRefs:
    - name: uvoo-gateway
      namespace: edge-gateway
      sectionName: http
  rules:
    - filters:
        - type: RequestRedirect
          requestRedirect:
            scheme: https
            statusCode: 301
```

## Verification

```bash
kubectl get gateway -A
kubectl -n envoy-gateway-system get svc
kubectl get httproute -A
kubectl -n edge-gateway get certificate,secret
```

Test certificate selection against the firewall-routed Envoy address:

```bash
curl -vkI --resolve grafana.uvoo.io:443:192.168.238.16 https://grafana.uvoo.io/
openssl s_client -connect 192.168.238.16:443 -servername grafana.uvoo.io </dev/null 2>/dev/null | openssl x509 -noout -subject -issuer -dates
```
