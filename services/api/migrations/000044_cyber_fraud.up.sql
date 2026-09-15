-- Phase 05 — Cybercrime & Financial Fraud, functional without AI.
--
-- Complaints stay in `cyber_crimes`. What they name — phone numbers, UPI IDs,
-- bank accounts, wallets, URLs, email addresses — is recorded once in a shared
-- register, so "two complaints share this UPI ID" is a stored fact rather than
-- something a model infers. The money trail, freeze requests and recoveries
-- hang off the complaint. Money is integer paise throughout.

-- ------------------------------------------------------------ complaints --

ALTER TABLE cyber_crimes
    ADD COLUMN IF NOT EXISTS ncrp_reference      VARCHAR(40),
    ADD COLUMN IF NOT EXISTS helpline_reference  VARCHAR(40),
    ADD COLUMN IF NOT EXISTS reported_loss_paise BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS station_id          UUID REFERENCES stations(id),
    ADD COLUMN IF NOT EXISTS registered_by       UUID REFERENCES users(id) ON DELETE SET NULL;

-- `financial_loss` was NUMERIC rupees read into a float64. Carry any value
-- across as paise, then remove it so there is one money column.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'cyber_crimes' AND column_name = 'financial_loss') THEN
        UPDATE cyber_crimes
           SET reported_loss_paise = ROUND(financial_loss * 100)::BIGINT
         WHERE financial_loss IS NOT NULL AND reported_loss_paise = 0;
        ALTER TABLE cyber_crimes DROP COLUMN financial_loss;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'cyber_crimes' AND column_name = 'currency') THEN
        ALTER TABLE cyber_crimes DROP COLUMN currency;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'cyber_crimes_loss_non_negative') THEN
        ALTER TABLE cyber_crimes ADD CONSTRAINT cyber_crimes_loss_non_negative CHECK (reported_loss_paise >= 0);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_cyber_crimes_ncrp ON cyber_crimes (ncrp_reference);

-- Complaint numbers were COUNT(*)+1; move them onto the atomic counters
-- (000033), starting above every number already issued.
INSERT INTO record_counters (scope, year, last_value)
SELECT 'CYBER', (m)[1]::int, MAX((m)[2]::bigint)
  FROM (SELECT regexp_match(case_number, '^CYBER/(\d{4})/(\d+)$') AS m FROM cyber_crimes) s
 WHERE m IS NOT NULL
 GROUP BY (m)[1]
ON CONFLICT (scope, year) DO UPDATE
    SET last_value = GREATEST(record_counters.last_value, EXCLUDED.last_value);

-- -------------------------------------------------------------- entities --

CREATE TABLE IF NOT EXISTS fraud_entities (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    entity_type       VARCHAR(20)  NOT NULL
                      CHECK (entity_type IN ('PHONE', 'UPI', 'BANK_ACCOUNT', 'WALLET', 'URL', 'EMAIL')),
    -- Canonical form used for matching: +91 and spacing stripped from phones,
    -- lower-cased UPI IDs and emails, IFSC:account for bank accounts.
    value_normalized  VARCHAR(512) NOT NULL,
    display_value     VARCHAR(512) NOT NULL,
    ifsc              VARCHAR(11),
    provider          VARCHAR(120),
    created_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fraud_entities_unique UNIQUE (entity_type, value_normalized),
    CONSTRAINT fraud_entities_ifsc_for_accounts CHECK ((entity_type = 'BANK_ACCOUNT') = (ifsc IS NOT NULL)),
    CONSTRAINT fraud_entities_ifsc_format CHECK (ifsc IS NULL OR ifsc ~ '^[A-Z]{4}0[A-Z0-9]{6}$')
);

-- How a complaint names an entity. VICTIM_OWN is the complainant's own phone
-- or account; it does not connect complaints to one another.
CREATE TABLE IF NOT EXISTS complaint_entities (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    complaint_id UUID NOT NULL REFERENCES cyber_crimes(id) ON DELETE CASCADE,
    entity_id    UUID NOT NULL REFERENCES fraud_entities(id),
    role         VARCHAR(20) NOT NULL
                 CHECK (role IN ('SUSPECT_CONTACT', 'BENEFICIARY', 'INFRASTRUCTURE', 'VICTIM_OWN')),
    note         TEXT,
    recorded_by  UUID NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT complaint_entities_unique UNIQUE (complaint_id, entity_id, role)
);
CREATE INDEX IF NOT EXISTS idx_complaint_entities_entity ON complaint_entities (entity_id);

-- ----------------------------------------------------------- money trail --

CREATE TABLE IF NOT EXISTS fraud_transactions (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    complaint_id   UUID NOT NULL REFERENCES cyber_crimes(id) ON DELETE CASCADE,
    from_entity_id UUID NOT NULL REFERENCES fraud_entities(id),
    to_entity_id   UUID NOT NULL REFERENCES fraud_entities(id),
    amount_paise   BIGINT NOT NULL CHECK (amount_paise > 0),
    reference      VARCHAR(64),
    occurred_at    TIMESTAMPTZ NOT NULL,
    note           TEXT,
    recorded_by    UUID NOT NULL REFERENCES users(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fraud_transactions_distinct_parties CHECK (from_entity_id <> to_entity_id)
);
CREATE INDEX IF NOT EXISTS idx_fraud_transactions_complaint ON fraud_transactions (complaint_id, occurred_at);

-- ------------------------------------------------------- freeze requests --

-- Sending is an officer's recorded action; the platform does not transmit to
-- a bank. Each stage's columns must be present exactly when the stage is.
CREATE TABLE IF NOT EXISTS freeze_requests (
    id                      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    request_number          VARCHAR(40) NOT NULL UNIQUE,
    complaint_id            UUID NOT NULL REFERENCES cyber_crimes(id) ON DELETE CASCADE,
    entity_id               UUID NOT NULL REFERENCES fraud_entities(id),
    addressee               VARCHAR(255) NOT NULL,
    amount_requested_paise  BIGINT NOT NULL CHECK (amount_requested_paise > 0),
    grounds                 TEXT NOT NULL,
    status                  VARCHAR(20) NOT NULL DEFAULT 'DRAFTED'
                            CHECK (status IN ('DRAFTED', 'SENT', 'ACKNOWLEDGED', 'FROZEN', 'REJECTED')),
    sent_at                 TIMESTAMPTZ,
    sent_by                 UUID REFERENCES users(id),
    sent_via                VARCHAR(120),
    acknowledged_at         TIMESTAMPTZ,
    acknowledgement_ref     VARCHAR(80),
    amount_frozen_paise     BIGINT,
    resolved_at             TIMESTAMPTZ,
    resolved_by             UUID REFERENCES users(id),
    rejection_reason        TEXT,
    created_by              UUID NOT NULL REFERENCES users(id),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT freeze_sent_fields CHECK (
        (status = 'DRAFTED' AND sent_at IS NULL AND sent_by IS NULL AND sent_via IS NULL)
        OR (status <> 'DRAFTED' AND sent_at IS NOT NULL AND sent_by IS NOT NULL AND sent_via IS NOT NULL)
    ),
    CONSTRAINT freeze_acknowledged_fields CHECK (status <> 'ACKNOWLEDGED' OR acknowledged_at IS NOT NULL),
    CONSTRAINT freeze_frozen_fields CHECK (
        (status = 'FROZEN' AND amount_frozen_paise IS NOT NULL AND amount_frozen_paise > 0
             AND amount_frozen_paise <= amount_requested_paise AND resolved_at IS NOT NULL)
        OR (status <> 'FROZEN' AND amount_frozen_paise IS NULL)
    ),
    CONSTRAINT freeze_rejected_fields CHECK (
        (status = 'REJECTED' AND rejection_reason IS NOT NULL AND resolved_at IS NOT NULL)
        OR (status <> 'REJECTED' AND rejection_reason IS NULL)
    )
);
CREATE INDEX IF NOT EXISTS idx_freeze_requests_complaint ON freeze_requests (complaint_id);
CREATE INDEX IF NOT EXISTS idx_freeze_requests_status ON freeze_requests (status);

-- ------------------------------------------------------------ recoveries --

CREATE TABLE IF NOT EXISTS fraud_recoveries (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    complaint_id      UUID NOT NULL REFERENCES cyber_crimes(id) ON DELETE CASCADE,
    freeze_request_id UUID REFERENCES freeze_requests(id),
    amount_paise      BIGINT NOT NULL CHECK (amount_paise > 0),
    recovered_on      DATE NOT NULL,
    reference         VARCHAR(80),
    note              TEXT,
    recorded_by       UUID NOT NULL REFERENCES users(id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_fraud_recoveries_complaint ON fraud_recoveries (complaint_id);
