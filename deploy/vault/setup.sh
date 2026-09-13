#!/bin/bash
# NPDMS Vault Setup Script
# This script configures Vault with all required secrets engines and policies

set -e

VAULT_ADDR=${VAULT_ADDR:-"http://localhost:8200"}
VAULT_TOKEN=${VAULT_TOKEN:-""}

if [ -z "$VAULT_TOKEN" ]; then
    echo "Error: VAULT_TOKEN environment variable is required"
    exit 1
fi

export VAULT_ADDR
export VAULT_TOKEN

echo "=== NPDMS Vault Configuration ==="
echo "Vault Address: $VAULT_ADDR"
echo ""

# Enable KV secrets engine v2
echo "1. Enabling KV secrets engine v2..."
vault secrets enable -path=secret -version=2 kv 2>/dev/null || echo "KV engine already enabled"

# Enable Transit secrets engine
echo "2. Enabling Transit secrets engine..."
vault secrets enable transit 2>/dev/null || echo "Transit engine already enabled"

# Create encryption keys
echo "3. Creating encryption keys..."
vault write -f transit/keys/npdms-data-encryption type=aes256-gcm96 2>/dev/null || echo "Data encryption key already exists"
vault write -f transit/keys/npdms-aadhaar-encryption type=aes256-gcm96 2>/dev/null || echo "Aadhaar encryption key already exists"

# Enable Database secrets engine
echo "4. Enabling Database secrets engine..."
vault secrets enable database 2>/dev/null || echo "Database engine already enabled"

# Configure PostgreSQL connection (update with your values)
echo "5. Configuring PostgreSQL connection..."
vault write database/config/npdms-postgres \
    plugin_name=postgresql-database-plugin \
    allowed_roles="npdms-api,npdms-readonly" \
    connection_url="postgresql://{{username}}:{{password}}@postgres:5432/npdms?sslmode=require" \
    username="vault_admin" \
    password="vault_admin_password" \
    password_authentication="scram-sha-256" 2>/dev/null || echo "PostgreSQL config already exists"

# Create database role for API
echo "6. Creating database role for API..."
vault write database/roles/npdms-api \
    db_name=npdms-postgres \
    creation_statements="CREATE ROLE \"{{name}}\" WITH LOGIN PASSWORD '{{password}}' VALID UNTIL '{{expiration}}'; \
        GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO \"{{name}}\"; \
        GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO \"{{name}}\";" \
    revocation_statements="REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM \"{{name}}\"; \
        DROP ROLE IF EXISTS \"{{name}}\";" \
    default_ttl="1h" \
    max_ttl="24h" 2>/dev/null || echo "API database role already exists"

# Create read-only database role
echo "7. Creating read-only database role..."
vault write database/roles/npdms-readonly \
    db_name=npdms-postgres \
    creation_statements="CREATE ROLE \"{{name}}\" WITH LOGIN PASSWORD '{{password}}' VALID UNTIL '{{expiration}}'; \
        GRANT SELECT ON ALL TABLES IN SCHEMA public TO \"{{name}}\";" \
    revocation_statements="REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM \"{{name}}\"; \
        DROP ROLE IF EXISTS \"{{name}}\";" \
    default_ttl="1h" \
    max_ttl="24h" 2>/dev/null || echo "Readonly database role already exists"

# Enable AppRole auth method
echo "8. Enabling AppRole auth method..."
vault auth enable approle 2>/dev/null || echo "AppRole already enabled"

# Create AppRole for API
echo "9. Creating AppRole for API..."
vault write auth/approle/role/npdms-api \
    secret_id_ttl=0 \
    token_num_uses=0 \
    token_ttl=1h \
    token_max_ttl=4h \
    secret_id_num_uses=0 \
    token_policies="npdms-api"

# Apply policies
echo "10. Applying policies..."
vault policy write npdms-api policies/npdms-api.hcl
vault policy write npdms-admin policies/npdms-admin.hcl
vault policy write npdms-readonly policies/npdms-readonly.hcl

# Store initial secrets (update with actual values in production)
echo "11. Storing initial secrets..."

# Database configuration
vault kv put secret/npdms/database \
    host="postgres" \
    port=5432 \
    database="npdms" \
    sslmode="require"

# JWT configuration
vault kv put secret/npdms/jwt \
    secret="$(openssl rand -base64 64)"

# Redis configuration
vault kv put secret/npdms/redis \
    password="$(openssl rand -base64 32)"

# S3/MinIO configuration
vault kv put secret/npdms/s3 \
    endpoint="minio:9000" \
    access_key_id="npdms-access-key" \
    secret_access_key="$(openssl rand -base64 32)" \
    bucket="npdms-evidence" \
    region="ap-south-1"

# Email configuration
vault kv put secret/npdms/email \
    smtp_host="smtp.example.com" \
    smtp_port=587 \
    username="npdms@example.com" \
    password="change-me" \
    from_address="npdms@example.com" \
    from_name="NPDMS System"

# SMS configuration
vault kv put secret/npdms/sms \
    provider="msg91" \
    api_key="change-me" \
    sender_id="NPDMS"

# Payment gateway (Razorpay)
vault kv put secret/npdms/payment/razorpay \
    key_id="change-me" \
    key_secret="change-me" \
    webhook_secret="change-me"

# Get AppRole credentials for API
echo ""
echo "=== AppRole Credentials ==="
echo "Role ID:"
vault read -field=role_id auth/approle/role/npdms-api/role-id
echo ""
echo "Secret ID:"
vault write -f -field=secret_id auth/approle/role/npdms-api/secret-id
echo ""

echo ""
echo "=== Vault Configuration Complete ==="
echo "Make sure to:"
echo "1. Update the PostgreSQL connection with real credentials"
echo "2. Update all 'change-me' values with real credentials"
echo "3. Store the AppRole credentials securely for the API service"
echo "4. Enable TLS in production"
