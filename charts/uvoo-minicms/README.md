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

If you deliberately accept the risk, you can force multiple identical writable replicas only with storage that supports shared mounts and correct cross-node file locking, such as a tested `ReadWriteMany` filesystem:

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

### Experimental public-read HA mode

For a safer multi-pod shape without Postgres, enable `ha.enabled`. This creates:

- one writer Deployment and writer-only Service
- one or more read-only reader pods
- public traffic routed to all pods
- `/admin` and `/cms.v1.CMSService` routed only to the writer Service
- `CMS_READ_ONLY=true` on reader pods so mutating API calls fail if a reader is reached directly

Install it with a release name such as `uvoo-minicms-ha`:

```bash
helm upgrade --install uvoo-minicms-ha ./charts/uvoo-minicms \
  --set ingress.host=cms.example.com \
  --set ha.enabled=true \
  --set ha.readerReplicaCount=2 \
  --set persistence.accessModes[0]=ReadWriteMany \
  --set persistence.storageClass=your-rwx-storage-class
```

This mode still requires shared `ReadWriteMany` storage for the SQLite database and uploads. It improves public-read availability during reader pod restarts, but the writer remains a single pod. Admin edits and imports are only as available as the writer pod and shared storage.

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

Redirect the bare domain to `www` with nginx Ingress:

```yaml
ingress:
  host: www.example.com
  redirect:
    fromToWWW: true
  tls:
    enabled: true
  certManager:
    enabled: true
    clusterIssuer: letsencrypt-prod
```

When `redirect.fromToWWW` is enabled, the chart adds the nginx `from-to-www-redirect` annotation and includes both `www.example.com` and `example.com` in the Ingress TLS hosts. Point DNS for both names at the Ingress controller. For HTTPS redirects, the certificate must cover both names.

With Ingress enabled, `CMS_TRUST_PROXY_HEADERS` defaults to `true` so the app can correctly evaluate HTTPS, host, and client IP headers from nginx. Only use that behind a trusted proxy that strips and rewrites forwarded headers.
