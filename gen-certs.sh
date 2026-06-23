#!/bin/bash -e
#
# Generate a self-signed TLS certificate for the kube-tailor webhook.
#
# Usage:
#   NAMESPACE=kube-tailor SERVICE_NAME=kube-tailor ./gen-certs.sh
#
# Outputs:
#   webhook.tls.secret.yaml  — kubectl-ready TLS secret (apply to the cluster)
#   caBundle value printed to stdout — paste into deploy/webhook.yaml

NAMESPACE="${NAMESPACE:-kube-tailor}"
SERVICE_NAME="${SERVICE_NAME:-kube-tailor}"
TLS_SECRET_NAME="${TLS_SECRET_NAME:-kube-tailor-tls}"
TLS_SECRET_OUTPUT="${TLS_SECRET_OUTPUT:-webhook.tls.secret.yaml}"

SERVICE_DNS="${SERVICE_NAME}.${NAMESPACE}.svc"
SERVICE_DNS_CLUSTER_LOCAL="${SERVICE_NAME}.${NAMESPACE}.svc.cluster.local"

echo ">> Generating CA..."
openssl genrsa -out ca.key 2048
openssl req -new -x509 -days 365 -key ca.key \
  -subj "/CN=${SERVICE_NAME}-ca" \
  -out ca.crt

echo ">> Generating server key and CSR..."
openssl req -newkey rsa:2048 -nodes -keyout server.key \
  -subj "/CN=${SERVICE_NAME}" \
  -out server.csr

echo ">> Signing certificate with SAN..."
openssl x509 -req \
  -extfile <(printf "subjectAltName=DNS:%s,DNS:%s" "${SERVICE_DNS}" "${SERVICE_DNS_CLUSTER_LOCAL}") \
  -days 365 \
  -in server.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out server.crt

echo ">> Generating Kubernetes TLS secret..."
kubectl create secret tls "${TLS_SECRET_NAME}" \
  --cert=server.crt \
  --key=server.key \
  --namespace="${NAMESPACE}" \
  --dry-run=client -o yaml \
  > "${TLS_SECRET_OUTPUT}"

echo ""
echo ">> Certificate SANs:"
echo "   ${SERVICE_DNS}"
echo "   ${SERVICE_DNS_CLUSTER_LOCAL}"
echo ""
echo ">> caBundle for deploy/webhook.yaml (paste over CA_BUNDLE_BASE64):"
cat ca.crt | base64 | tr -d '\n'
echo ""
echo ""
echo ">> Next steps:"
echo "   kubectl apply -f ${TLS_SECRET_OUTPUT} -n ${NAMESPACE}"
echo "   # Update caBundle in deploy/webhook.yaml, then:"
echo "   kubectl apply -f deploy/webhook.yaml"

rm -f ca.key ca.srl server.csr
echo ">> Done. ca.crt, server.crt, server.key, and ${TLS_SECRET_OUTPUT} written."
