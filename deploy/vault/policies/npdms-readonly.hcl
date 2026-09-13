# NPDMS Read-Only Policy
# This policy grants read-only access to NPDMS secrets for monitoring/audit

# Read-only access to NPDMS secrets
path "secret/data/npdms/*" {
  capabilities = ["read", "list"]
}

path "secret/metadata/npdms/*" {
  capabilities = ["read", "list"]
}

# Read database configuration (not credentials)
path "database/config/*" {
  capabilities = ["read", "list"]
}

path "database/roles/*" {
  capabilities = ["read", "list"]
}

# Transit key info (no encrypt/decrypt)
path "transit/keys/*" {
  capabilities = ["read", "list"]
}

# Token self-operations
path "auth/token/lookup-self" {
  capabilities = ["read"]
}

path "auth/token/renew-self" {
  capabilities = ["update"]
}
