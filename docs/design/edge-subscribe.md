# Edge subscription manifest API

`GET /api/edge/v1/manifest` is a private API for the `ppanel-edge-subscribe`
Cloudflare Worker. It is separate from the normal subscription endpoint:

- It has its own `/api/edge/v1/*` route and HMAC credentials.
- It returns data (`Manifest v1`), not a client configuration or template output.
- It does not invoke `adapter.NewAdapter`, `SubscribeHandler`, or the existing
  user-agent/template selection code.
- It reads the same user, plan and node records, so traffic and entitlement
  state remain authoritative in PPanel.

## Enable it

Add a key for each Worker deployment to the server's runtime YAML and restart
PPanel:

```yaml
EdgeSubscribe:
  Enabled: true
  MaxClockSkewSeconds: 300
  Keys:
    - ID: edge-production
      Secret: replace-with-a-long-random-value
```

Set `EDGE_KID=edge-production` and the same `EDGE_SECRET` in the Worker. Keep
the secret out of Git, logs and the PPanel web API. Rotation is additive: add a
new key, deploy Workers using it, then remove the old key after its Workers are
gone.

## Request authentication

The Worker sends the opaque subscription token as `?token=` and proves that it
is a configured edge deployment:

```http
Authorization: PPanel-Edge-HMAC kid=edge-production, ts=1735689600, sig=<hex>
```

The signed bytes are exactly:

```text
v2\nGET\n/api/edge/v1/manifest\nSHA256(token)\nUnixTimestamp\nSHA256(X-Request-ID)
```

`X-Request-ID` must be a UUID, is covered by the signature, and can be used
only once for the full signature validity window. PPanel records it in Redis;
Redis unavailability therefore fails the request closed with `503` rather than
silently weakening replay protection. Timestamps outside
`MaxClockSkewSeconds` are rejected (the server caps it at 300 seconds). Invalid
HMAC, unknown key, replayed request, disabled endpoint, malformed requests,
and unknown/disabled user tokens all return `404`; this avoids turning the API
into a token or key oracle.

For immediate entitlement revocation, leave the Worker's
`CACHE_TTL_SECONDS=0` (the default). A positive Worker cache TTL is an explicit
availability/performance trade-off: permission changes can otherwise take up to
that TTL to reach clients.

## V1 compatibility boundary

The API returns only proxies the current Worker can render faithfully:
Shadowsocks (excluding 2022/plugins), VMess, VLESS and Trojan; TLS, WebSocket,
gRPC and HTTPUpgrade are supported. Nodes using REALITY, XHTTP, Shadowsocks
2022, plugins or other protocols are omitted and reported in `notices`.
They are deliberately never downgraded into incorrect client configuration.
