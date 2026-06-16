# Uvoo-MiniCMS Helm Chart

This chart runs Uvoo-MiniCMS behind a Kubernetes Service and optional HTTPS Ingress. The default Ingress class is `nginx`.

## Install

```bash
helm install cms ./charts/uvoo-minicms \
  --set image.tag=latest \
  --set ingress.host=cms.example.com
```

The chart defaults to `ghcr.io/uvoo/uvoo-minicms:latest` with `image.pullPolicy=Always`. For repeatable deployments, set `image.tag` to a release, branch, or short-SHA tag published by the GHCR workflow. Override `image.repository` only if you mirror or fork the image.

The chart generates and preserves an admin password unless `admin.password` or `admin.existingSecret` is set.

The default deployment strategy is `Recreate` because Uvoo-MiniCMS uses SQLite on a persistent volume. Keep `replicaCount: 1` unless you have deliberately moved storage to a setup that is safe for concurrent writers.

## TLS Options

Use an existing TLS Secret:

```bash
kubectl create secret tls cms-tls --cert=site.crt --key=site.key
helm upgrade --install cms ./charts/uvoo-minicms \
  --set ingress.host=cms.example.com \
  --set ingress.tls.secretName=cms-tls
```

Create the TLS Secret from PEM values:

```yaml
ingress:
  host: cms.example.com
  tls:
    enabled: true
    secretName: cms-tls
    crt: |
      -----BEGIN CERTIFICATE-----
      ...
      -----END CERTIFICATE-----
    key: |
      -----BEGIN PRIVATE KEY-----
      ...
      -----END PRIVATE KEY-----
```

Use cert-manager ACME:

```yaml
ingress:
  host: cms.example.com
  className: nginx
  tls:
    enabled: true
    secretName: cms-tls
  certManager:
    enabled: true
    clusterIssuer: letsencrypt-prod
```

With Ingress enabled, `CMS_TRUST_PROXY_HEADERS` defaults to `true` so the app can correctly evaluate HTTPS, host, and client IP headers from nginx. Only use that behind a trusted proxy that strips and rewrites forwarded headers.
