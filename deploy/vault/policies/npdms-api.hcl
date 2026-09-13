# NPDMS API Policy
# This policy grants the API service access to its required secrets

# KV Secrets Engine v2 - Read access to NPDMS secrets
path "secret/data/npdms/*" {
  capabilities = ["read", "list"]
}

# List secrets paths
path "secret/metadata/npdms/*" {
  capabilities = ["list"]
}

# Database dynamic credentials
path "database/creds/npdms-api" {
  capabilities = ["read"]
}

# Database static credentials (for read-replicas)
path "database/static-creds/npdms-readonly" {
  capabilities = ["read"]
}

# Transit secrets engine - Encrypt/Decrypt for sensitive fields
path "transit/encrypt/npdms-data-encryption" {
  capabilities = ["update"]
}

path "transit/decrypt/npdms-data-encryption" {
  capabilities = ["update"]
}

# Aadhaar encryption (separate key for regulatory compliance)
path "transit/encrypt/npdms-aadhaar-encryption" {
  capabilities = ["update"]
}

path "transit/decrypt/npdms-aadhaar-encryption" {
  capabilities = ["update"]
}

# Generate data keys for envelope encryption
path "transit/datakey/plaintext/npdms-data-encryption" {
  capabilities = ["update"]
}

# Token self-renewal
path "auth/token/renew-self" {
  capabilities = ["update"]
}

# Token lookup
path "auth/token/lookup-self" {
  capabilities = ["read"]
}
