# Deploy using envoy gateway instead of ingress
```bash
helm upgrade --install cms ./charts/uvoo-minicms \
  --namespace my-namespace \
  --set ingress.enabled=false \
  --set gateway.enabled=true \
  --set gateway.className=eg \
  --set gateway.host=www.example.com \
  --set gateway.redirect.fromToWWW=true \
  --set gateway.tls.secretName=cms-uvoo-minicms-tls \
  --set gateway.certManager.enabled=true \
  --set gateway.certManager.clusterIssuer=letsencrypt-prod
```


Use existing gateway if wanted.

```bash
--set gateway.create=false \
--set gateway.name=my-gateway
```

Verification:

Check that Gateway, HTTPRoute, redirect routes, and cert-manager Certificate deploy cleanly.
