# IBM Cloud Internet Service Webhook for cert-manager

A [cert-manager](https://cert-manager.io/) webhook solver for [IBM Cloud Internet Services (CIS)](https://cloud.ibm.com/catalog/services/internet-services) DNS-01 challenges.

Inspired by and rewritten from [cert-manager-webhook-ibmcis v1](https://github.ibm.com/hzhihui/cert-manager-webhook-ibmcis) by Jørgen Borup and hughhuangzh. This v2 rewrites the codebase with modern Go, the official IBM networking SDK, proper error handling, security hardening, and multi-arch container images.

## What's different from v1

- **IBM SDK**: Uses `IBM/networking-go-sdk` instead of deprecated `bluemix-go`
- **Go 1.26**: Updated from Go 1.21 (out of support)
- **cert-manager v1.17+**: Updated from v1.13
- **Error handling**: Present() and CleanUp() now properly return errors instead of silently failing
- **Race condition fix**: No shared mutable state between concurrent challenge requests
- **Security**: Non-root container (UID 65532), securityContext, resource limits, NetworkPolicy
- **Logging**: Structured JSON logging via `log/slog` (replaces mixed klog + logrus)
- **Multi-arch**: AMD64 and ARM64 container images
- **Helm chart v2**: Proper `.Release.Namespace`, configurable credentials secret, `deploy` flag

## Prerequisites

- Kubernetes or OpenShift cluster with [cert-manager](https://cert-manager.io/docs/installation/) installed
- IBM Cloud account with a CIS instance
- IBM Cloud API key with DNS management permissions

## Setting up IBM Cloud credentials

### 1. Create a Service ID

```bash
ibmcloud iam service-id-create cert-manager-webhook-ibmcis \
  -d "Service ID for cert-manager DNS-01 webhook"
```

### 2. Grant DNS management permissions

Grant the Service ID **Manager** role on your CIS instance:

```bash
ibmcloud iam service-policy-create cert-manager-webhook-ibmcis \
  --service-name internet-svcs \
  --service-instance SERVICE_INSTANCE_GUID \
  --roles Manager
```

To find your CIS instance GUID:

```bash
ibmcloud resource service-instance <CIS_INSTANCE_NAME> --output json | jq -r '.[0].guid'
```

### 3. Generate an API key

```bash
ibmcloud iam service-api-key-create webhook-apikey cert-manager-webhook-ibmcis \
  -d "API key for cert-manager DNS-01 webhook"
```

Save the API key — it will not be shown again.

### 4. Get the CIS instance CRN

```bash
ibmcloud resource service-instance <CIS_INSTANCE_NAME> --output json | jq -r '.[0].crn'
```

The CRN looks like: `crn:v1:bluemix:public:internet-svcs:global:a/<account-id>:<instance-id>::`

### 5. Store the API key in a Kubernetes secret

```bash
kubectl create namespace cert-manager-webhook-ibmcis

kubectl create secret generic ibmcis-credentials \
  --namespace cert-manager-webhook-ibmcis \
  --from-literal=api-token='<YOUR_API_KEY>'
```

The secret name and key are configurable via `credentials.secretName` and `credentials.secretKey` in `values.yaml`. This means you can use any secret management tool (Bitwarden Secrets Manager, HashiCorp Vault, External Secrets Operator, etc.) to provision the secret — the Helm chart only needs to know the secret's name and key.

## Installation

```bash
helm install cert-manager-webhook-ibmcis helm/ \
  --namespace cert-manager-webhook-ibmcis \
  --set groupName=acme.yourdomain.com
```

The pod will not become fully ready until cert-manager issues its serving certificate (usually takes a few seconds).

### Helm values

| Parameter | Default | Description |
|---|---|---|
| `deploy` | `true` | Set to `false` to skip creating all resources (useful in umbrella charts) |
| `groupName` | `acme.borup.work` | API group for the webhook, must match your Issuer config |
| `image.repository` | `quay.io/rhpds/cert-manager-webhook-ibmcis` | Container image repository |
| `image.tag` | `""` (uses appVersion) | Container image tag |
| `credentials.secretName` | `ibmcis-credentials` | Name of the Kubernetes secret containing the IBM Cloud API key |
| `credentials.secretKey` | `api-token` | Key within the secret |
| `certManager.namespace` | `cert-manager` | Namespace where cert-manager is installed |
| `certManager.serviceAccountName` | `cert-manager` | cert-manager's service account name |
| `replicaCount` | `1` | Number of webhook replicas |
| `networkPolicy.enabled` | `true` | Create a NetworkPolicy restricting traffic |
| `resources.requests.cpu` | `50m` | CPU request |
| `resources.requests.memory` | `64Mi` | Memory request |
| `resources.limits.cpu` | `200m` | CPU limit |
| `resources.limits.memory` | `128Mi` | Memory limit |

## Configuring Issuers

### Staging Issuer (recommended for testing)

Use the Let's Encrypt staging server to avoid rate limits while testing. Certificates from staging are not trusted by browsers but the flow is identical.

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-staging
spec:
  acme:
    server: https://acme-staging-v02.api.letsencrypt.org/directory
    email: your-email@example.com
    privateKeySecretRef:
      name: letsencrypt-staging-account-key
    solvers:
      - dns01:
          webhook:
            groupName: acme.yourdomain.com
            solverName: ibmcis
            config:
              cisCRN:
                - "crn:v1:bluemix:public:internet-svcs:global:a/<account-id>:<instance-id>::"
        selector:
          dnsZones:
            - "example.com"
```

### Production ClusterIssuer

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: your-email@example.com
    privateKeySecretRef:
      name: letsencrypt-account-key
    solvers:
      - dns01:
          webhook:
            groupName: acme.yourdomain.com
            solverName: ibmcis
            config:
              cisCRN:
                - "crn:v1:bluemix:public:internet-svcs:global:a/<account-id>:<instance-id>::"
        selector:
          dnsZones:
            - "example.com"
```

You can list multiple CRNs in `cisCRN` if you manage zones across multiple CIS instances. The webhook will search all of them to find the matching zone.

### Namespace-scoped Issuer

If you prefer namespace-scoped issuers instead of ClusterIssuers, change `kind: ClusterIssuer` to `kind: Issuer` and add `namespace` to the metadata.

## Requesting certificates

### Simple certificate

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example-com
spec:
  secretName: example-com-tls
  dnsNames:
    - example.com
    - "*.example.com"
  issuerRef:
    name: letsencrypt
    kind: ClusterIssuer
```

### Checking certificate status

```bash
kubectl get certificate example-com
kubectl describe certificate example-com
kubectl get secret example-com-tls
```

### Automatically creating certificates for Ingress resources

cert-manager can automatically create certificates for Ingress resources. See the [cert-manager Ingress documentation](https://cert-manager.io/docs/usage/ingress/).

## Development

```bash
make test          # run unit tests
make build         # build binary
make lint          # run go vet
make helm-lint     # lint Helm chart
make helm-template # render Helm templates
make docker-build  # build container image
```

### Running tests

```bash
go test ./... -v -count=1
```

### Building on OpenShift

A `build-template.yaml` is provided for building on OpenShift clusters using BuildConfig:

```bash
oc process -f build-template.yaml | oc apply -n <namespace> -f -
oc start-build cert-manager-webhook-ibmcis --from-dir=. --follow -n <namespace>
```

## Releasing

```bash
./bump-version.sh           # auto-increment patch version
./bump-version.sh v1.2.0    # specific version
./bump-version.sh --dry-run # preview without changes
```

This updates `helm/Chart.yaml` (version + appVersion), commits, tags, and pushes. The GitHub Actions release workflow then builds multi-arch images (AMD64 + ARM64) and pushes them to the container registry.
