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

The default deployment strategy is `Recreate` because Uvoo-MiniCMS uses SQLite on a persistent volume. Keep `replicaCount: 1` for normal installs. This avoids Kubernetes `Multi-Attach` errors with the default `ReadWriteOnce` PVC and avoids concurrent writes to one SQLite database file.

## SQLite and Replicas

SQLite is best treated as single-writer application storage in Kubernetes. The preferred deployment is:

```yaml
replicaCount: 1
deploymentStrategy:
  type: Recreate
persistence:
  enabled: true
  accessModes:
    - ReadWriteOnce
```

Do not set `replicaCount: 2` with the default PVC. A `ReadWriteOnce` volume can only attach to one node at a time, so Kubernetes may report `Multi-Attach error for volume ... Volume is already used by pod`.

If you deliberately accept the risk, you can force multiple replicas only with storage that supports shared mounts and correct cross-node file locking, such as a tested `ReadWriteMany` filesystem:

```yaml
replicaCount: 2
sqlite:
  allowUnsafeMultiReplica: true
persistence:
  accessModes:
    - ReadWriteMany
  storageClass: your-rwx-storage-class
```

This is not recommended as an HA design for SQLite. Expect writer contention, and test page saves, uploads, imports, and admin changes under concurrent traffic before using it. For high availability without changing databases, prefer one application replica with reliable persistent storage, fast backups, and Kubernetes restarting the pod on failure.

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
