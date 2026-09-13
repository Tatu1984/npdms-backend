# Vault Server Configuration for NPDMS

# Storage backend - using Raft for production
storage "raft" {
  path    = "/vault/data"
  node_id = "vault-node-1"
}

# Listener configuration
listener "tcp" {
  address     = "0.0.0.0:8200"
  tls_disable = false

  # TLS Configuration (production)
  tls_cert_file = "/vault/tls/vault.crt"
  tls_key_file  = "/vault/tls/vault.key"

  # Optional: Client certificate authentication
  # tls_require_and_verify_client_cert = true
  # tls_client_ca_file = "/vault/tls/ca.crt"
}

# Cluster configuration for HA
cluster_addr = "https://127.0.0.1:8201"
api_addr     = "https://127.0.0.1:8200"

# UI configuration
ui = true

# Telemetry
telemetry {
  prometheus_retention_time = "30s"
  disable_hostname          = true
}

# Seal configuration - using auto-unseal with AWS KMS for production
# Uncomment for AWS KMS auto-unseal
# seal "awskms" {
#   region     = "ap-south-1"
#   kms_key_id = "alias/npdms-vault-unseal"
# }

# Audit logging
# audit "file" {
#   file_path = "/vault/logs/audit.log"
# }

# Disable mlock for containerized environments
disable_mlock = true

# Log level
log_level = "Info"

# Maximum lease TTL
max_lease_ttl = "768h"

# Default lease TTL
default_lease_ttl = "768h"
