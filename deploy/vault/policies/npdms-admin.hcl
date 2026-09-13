# NPDMS Admin Policy
# This policy grants administrators full access to NPDMS secrets management

# Full access to NPDMS secrets
path "secret/data/npdms/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "secret/metadata/npdms/*" {
  capabilities = ["read", "list", "delete"]
}

path "secret/delete/npdms/*" {
  capabilities = ["update"]
}

path "secret/undelete/npdms/*" {
  capabilities = ["update"]
}

path "secret/destroy/npdms/*" {
  capabilities = ["update"]
}

# Database secrets engine management
path "database/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

# Transit secrets engine management
path "transit/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

# Key rotation
path "transit/keys/npdms-*/rotate" {
  capabilities = ["update"]
}

# Policy management
path "sys/policies/acl/npdms-*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

# Auth method management
path "auth/*" {
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}

# Audit devices
path "sys/audit/*" {
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}

# Secrets engines management
path "sys/mounts/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

# Health and status
path "sys/health" {
  capabilities = ["read", "sudo"]
}

path "sys/seal-status" {
  capabilities = ["read"]
}

# Token management
path "auth/token/*" {
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}

# Leases management
path "sys/leases/*" {
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}
