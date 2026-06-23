# kube-tailor

A Kubernetes mutating admission webhook that automatically tailors CPU and memory resources for DaemonSet pods based on the actual size of the node they are scheduled on.

DaemonSet agents (Fluent Bit, Datadog, Prometheus exporters, etc.) need resources proportional to the node they run on — a 64-core node needs far more log-shipping capacity than a 2-core one. Cluster autoscalers (Karpenter, CA) do not account for DaemonSet overhead when binpacking, which leads to pod evictions and disrupted workloads. kube-tailor solves this by injecting the right `requests` and `limits` at admission time, **per node**, with no manual intervention.

## How it works

```
Pod CREATE → kube-apiserver → kube-tailor webhook
                                      │
                              Is pod owned by a DaemonSet?
                              Is the DaemonSet opted in?
                                      │
                              Fetch node capacity
                              (Karpenter labels → node status.capacity fallback)
                                      │
                              Calculate % of node CPU / memory
                              Apply optional min/max clamp
                                      │
                              Return JSON patch → pod created with correct resources
```

Mutation is **fail-open**: if anything goes wrong (webhook unreachable, parse error, node lookup failure) the pod is admitted unchanged. Set `failurePolicy: Ignore` on the `MutatingWebhookConfiguration` to match this behaviour.

## Opt-in labels

Only DaemonSets that explicitly opt in are mutated. Add labels to the DaemonSet `metadata` (pod template labels are used as a fallback):

| Label | Required | Description |
|---|---|---|
| `kube-tailor/enabled` | yes | `"true"` to enable resource injection |
| `kube-tailor/cpu-percent` | one of cpu/memory | % of node CPU allocated as a **request only** (no CPU limit set) |
| `kube-tailor/cpu-min` | no | Floor for the calculated CPU request (e.g. `500m`) |
| `kube-tailor/cpu-max` | no | Ceiling for the calculated CPU request (e.g. `2`) |
| `kube-tailor/memory-percent` | one of cpu/memory | % of node memory allocated as both request and limit |
| `kube-tailor/memory-min` | no | Floor for calculated memory (e.g. `512Mi`) |
| `kube-tailor/memory-max` | no | Ceiling for calculated memory (e.g. `2Gi`) |
| `kube-tailor/targetted-containers` | no | Comma-separated container names to target. Omit to apply to all containers. |

At least one of `cpu-percent` or `memory-percent` must be set when `enabled: "true"`. Min/max bounds require the corresponding percent label.

### Example — Fluent Bit

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: fluent-bit
  namespace: logging
  labels:
    kube-tailor/enabled: "true"
    kube-tailor/cpu-percent: "10"
    kube-tailor/cpu-min: "500m"
    kube-tailor/cpu-max: "2"
    kube-tailor/memory-percent: "5"
    kube-tailor/memory-min: "512Mi"
    kube-tailor/memory-max: "2Gi"
spec:
  # ...
```

On a **2 vCPU / 4096 MiB** node:
- CPU request → `200m` (10% of 2000m), clamped up to `500m` by `cpu-min`
- Memory request/limit → `~204Mi` (5% of 4096 MiB)

On a **32 vCPU / 65536 MiB** node:
- CPU request → `3200m`, clamped down to `2` by `cpu-max`
- Memory request/limit → `~3276Mi`, clamped down to `2Gi` by `memory-max`

### Targeting specific containers

```yaml
labels:
  kube-tailor/enabled: "true"
  kube-tailor/cpu-percent: "5"
  kube-tailor/memory-percent: "3"
  kube-tailor/targetted-containers: "agent,collector"
```

Only the `agent` and `collector` containers in the pod are mutated; sidecars are left untouched.

## Node capacity resolution

kube-tailor reads node capacity in this order:

1. **Karpenter labels** — `karpenter.k8s.aws/instance-cpu` (vCPUs) and `karpenter.k8s.aws/instance-memory` (MiB). These are set by Karpenter before the node is fully Ready, so resources are accurate even for brand-new nodes.
2. **`node.status.capacity`** — standard Kubernetes node capacity, used as a fallback for non-Karpenter clusters.

## Deployment

### Prerequisites

- Kubernetes 1.24+
- TLS certificate for the webhook service (see [TLS setup](#tls-setup))
- `kubectl` access with cluster-admin

### 1. Namespace

```bash
kubectl create namespace kube-tailor
```

### 2. TLS secret

```bash
kubectl create secret tls kube-tailor-tls \
  --cert=server.crt \
  --key=server.key \
  --namespace=kube-tailor
```

See [TLS setup](#tls-setup) for how to generate the certificate.

### 3. Deployment and Service

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kube-tailor
  namespace: kube-tailor
spec:
  replicas: 2
  selector:
    matchLabels:
      app: kube-tailor
  template:
    metadata:
      labels:
        app: kube-tailor
    spec:
      serviceAccountName: kube-tailor
      containers:
        - name: kube-tailor
          image: ghcr.io/a-kash-singh/kube-tailor:latest
          ports:
            - containerPort: 443
          env:
            - name: TLS
              value: "true"
            - name: LOG_LEVEL
              value: "info"
          volumeMounts:
            - name: tls
              mountPath: /etc/admission-webhook/tls
              readOnly: true
          resources:
            requests:
              cpu: 100m
              memory: 64Mi
            limits:
              memory: 128Mi
      volumes:
        - name: tls
          secret:
            secretName: kube-tailor-tls
---
apiVersion: v1
kind: Service
metadata:
  name: kube-tailor
  namespace: kube-tailor
spec:
  selector:
    app: kube-tailor
  ports:
    - port: 443
      targetPort: 443
```

### 4. RBAC

kube-tailor needs read access to DaemonSets and Nodes at runtime.

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kube-tailor
  namespace: kube-tailor
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kube-tailor
rules:
  - apiGroups: ["apps"]
    resources: ["daemonsets"]
    verbs: ["get"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kube-tailor
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kube-tailor
subjects:
  - kind: ServiceAccount
    name: kube-tailor
    namespace: kube-tailor
```

### 5. MutatingWebhookConfiguration

Replace `<CA_BUNDLE_BASE64>` with the base64-encoded CA certificate used to sign the webhook TLS cert.

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: kube-tailor
webhooks:
  - name: kube-tailor.kube-tailor.svc
    admissionReviewVersions: ["v1"]
    sideEffects: None
    failurePolicy: Ignore
    clientConfig:
      service:
        name: kube-tailor
        namespace: kube-tailor
        path: /mutate-pods
      caBundle: <CA_BUNDLE_BASE64>
    rules:
      - apiGroups: [""]
        apiVersions: ["v1"]
        operations: ["CREATE"]
        resources: ["pods"]
    # Scope to specific namespaces if desired:
    # namespaceSelector:
    #   matchLabels:
    #     kube-tailor/inject: "true"
```

> **Note:** `failurePolicy: Ignore` ensures pod creation is never blocked if the webhook is unavailable — kube-tailor is designed to be fail-open.

## TLS setup

The webhook endpoint must be served over HTTPS. The certificate's SAN must match the in-cluster Service DNS:

```
kube-tailor.kube-tailor.svc
kube-tailor.kube-tailor.svc.cluster.local
```

### Generating a self-signed certificate

```bash
# Generate CA
openssl genrsa -out ca.key 2048
openssl req -new -x509 -days 365 -key ca.key \
  -subj "/CN=kube-tailor-ca" -out ca.crt

# Generate server key and CSR
openssl req -newkey rsa:2048 -nodes -keyout server.key \
  -subj "/CN=kube-tailor" -out server.csr

# Sign with SAN
openssl x509 -req \
  -extfile <(printf "subjectAltName=DNS:kube-tailor.kube-tailor.svc,DNS:kube-tailor.kube-tailor.svc.cluster.local") \
  -days 365 -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out server.crt

# Get caBundle for MutatingWebhookConfiguration
cat ca.crt | base64 | tr -d '\n'
```

### Using cert-manager

If you use [cert-manager](https://cert-manager.io), create a `Certificate` resource targeting the webhook service and annotate the `MutatingWebhookConfiguration` with `cert-manager.io/inject-ca-from` for automatic CA injection.

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `TLS` | `""` | Set to `"true"` to enable HTTPS on port 443. Without it, the server listens on port 8080 (plain HTTP — local dev only). |
| `LOG_LEVEL` | `debug` | Logrus level: `trace`, `debug`, `info`, `warn`, `error` |
| `LOG_JSON` | `""` | Set to `"true"` for JSON log output |

## Local development

```bash
# Run tests
go test ./...

# Build binary
make build   # outputs bin/kube-tailor

# Run locally (HTTP, no TLS — for local testing only)
go run .
```

To test against a real cluster without in-cluster TLS, use `kubectl port-forward` to proxy the webhook service and point the `MutatingWebhookConfiguration` at your local address (requires a tunnel tool like [ngrok](https://ngrok.com) or [telepresence](https://www.telepresence.io)).

## Fail-open behaviour

kube-tailor never blocks pod creation. In any error path — unresolvable node, missing DaemonSet, label parse error — the webhook returns `allowed: true` without a patch and logs the reason. Pair this with `failurePolicy: Ignore` on the `MutatingWebhookConfiguration` to ensure webhook downtime also cannot block scheduling.

## Scope and targeting

Control is at two independent levels:

**Webhook scope** (`MutatingWebhookConfiguration`) — defines which pod CREATE requests reach kube-tailor at all. Use `namespaceSelector`, `objectSelector`, and `rules` to narrow or widen the scope. No namespace labels are required by default.

**Mutation scope** (DaemonSet labels) — once a pod reaches the webhook, only DaemonSets with `kube-tailor/enabled: "true"` are mutated. Non-DaemonSet pods and un-labelled DaemonSets pass through unchanged.

Deploy kube-tailor in its own namespace (`kube-tailor`) and exclude that namespace from the webhook scope to avoid cyclic dependency during pod startup.

## License

[Apache 2.0](LICENSE)
