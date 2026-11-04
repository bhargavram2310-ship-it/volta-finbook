-- MySQL 8.4 port of the table, index, constraint and relationship DDL in
-- migrations/0001_foundation.sql. Apply only to an empty finbook database.
-- UUID values are application-generated and stored as 16-byte binary values.
-- MySQL has no native equivalent for PostgreSQL domains or enums; fixed-scale
-- decimals plus enforced CHECK constraints and VARCHAR checks are used instead.
-- No data is inserted, including no tax rules, roles, currencies or accounts.

SET NAMES utf8mb4 COLLATE utf8mb4_0900_ai_ci;

CREATE TABLE currencies (
    code VARCHAR(3) NOT NULL,
    name VARCHAR(191) NOT NULL,
    decimal_places SMALLINT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (code),
    CONSTRAINT currencies_code_format CHECK (REGEXP_LIKE(code, '^[A-Z]{3}$', 'c')),
    CONSTRAINT currencies_decimal_places_range CHECK (decimal_places BETWEEN 0 AND 6)
) ENGINE=InnoDB;

CREATE TABLE company (
    id BINARY(16) NOT NULL,
    singleton BOOLEAN NOT NULL DEFAULT TRUE,
    legal_name VARCHAR(255) NOT NULL,
    base_currency VARCHAR(3) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY company_singleton_uq (singleton),
    CONSTRAINT company_singleton_true CHECK (singleton = TRUE),
    CONSTRAINT company_legal_name_nonempty CHECK (CHAR_LENGTH(TRIM(legal_name)) > 0),
    CONSTRAINT company_base_currency_fk FOREIGN KEY (base_currency) REFERENCES currencies(code)
) ENGINE=InnoDB;

CREATE TABLE branches (
    id BINARY(16) NOT NULL,
    company_id BINARY(16) NOT NULL,
    code VARCHAR(100) NOT NULL,
    name VARCHAR(191) NOT NULL,
    state_code VARCHAR(32) NULL,
    gst_registration VARCHAR(100) NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY branches_code_uq (code),
    UNIQUE KEY branches_company_id_uq (company_id, id),
    CONSTRAINT branches_company_fk FOREIGN KEY (company_id) REFERENCES company(id)
) ENGINE=InnoDB;

CREATE TABLE users (
    id BINARY(16) NOT NULL,
    username VARCHAR(191) NOT NULL,
    display_name VARCHAR(191) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    username_active_key VARCHAR(191)
        GENERATED ALWAYS AS (IF(deleted_at IS NULL, LOWER(username), NULL)) STORED,
    PRIMARY KEY (id),
    UNIQUE KEY users_username_active_uq (username_active_key)
) ENGINE=InnoDB;

CREATE TABLE roles (
    id BINARY(16) NOT NULL,
    code VARCHAR(100) NOT NULL,
    name VARCHAR(191) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY roles_code_uq (code)
) ENGINE=InnoDB;

CREATE TABLE permissions (
    id BINARY(16) NOT NULL,
    module VARCHAR(100) NOT NULL,
    screen VARCHAR(100) NOT NULL,
    action VARCHAR(100) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY permissions_module_screen_action_uq (module, screen, action)
) ENGINE=InnoDB;

CREATE TABLE user_roles (
    user_id BINARY(16) NOT NULL,
    role_id BINARY(16) NOT NULL,
    PRIMARY KEY (user_id, role_id),
    CONSTRAINT user_roles_user_fk FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT user_roles_role_fk FOREIGN KEY (role_id) REFERENCES roles(id)
) ENGINE=InnoDB;

CREATE TABLE role_permissions (
    role_id BINARY(16) NOT NULL,
    permission_id BINARY(16) NOT NULL,
    PRIMARY KEY (role_id, permission_id),
    CONSTRAINT role_permissions_role_fk FOREIGN KEY (role_id) REFERENCES roles(id),
    CONSTRAINT role_permissions_permission_fk FOREIGN KEY (permission_id) REFERENCES permissions(id)
) ENGINE=InnoDB;

CREATE TABLE sessions (
    id BINARY(16) NOT NULL,
    user_id BINARY(16) NOT NULL,
    token_hash BINARY(32) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    revoked_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY sessions_token_hash_uq (token_hash),
    KEY sessions_user_expiry_idx (user_id, expires_at),
    CONSTRAINT sessions_token_hash_length CHECK (OCTET_LENGTH(token_hash) = 32),
    CONSTRAINT sessions_user_fk FOREIGN KEY (user_id) REFERENCES users(id)
) ENGINE=InnoDB;

CREATE TABLE parties (
    id BINARY(16) NOT NULL,
    company_id BINARY(16) NOT NULL,
    code VARCHAR(100) NOT NULL,
    legal_name VARCHAR(255) NOT NULL,
    gst_registration VARCHAR(100) NULL,
    state_code VARCHAR(32) NULL,
    country_code VARCHAR(32) NULL,
    email VARCHAR(254) NULL,
    phone VARCHAR(64) NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY parties_code_uq (code),
    CONSTRAINT parties_company_fk FOREIGN KEY (company_id) REFERENCES company(id)
) ENGINE=InnoDB;

CREATE TABLE customers (
    id BINARY(16) NOT NULL,
    party_id BINARY(16) NOT NULL,
    credit_days INT NULL,
    credit_limit DECIMAL(65,30) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY customers_party_uq (party_id),
    CONSTRAINT customers_credit_days_nonnegative CHECK (credit_days IS NULL OR credit_days >= 0),
    CONSTRAINT customers_credit_limit_precision CHECK (
        credit_limit IS NULL OR
        (ABS(credit_limit) < 1000000000000000000 AND credit_limit = TRUNCATE(credit_limit, 6))
    ),
    CONSTRAINT customers_credit_limit_nonnegative CHECK (credit_limit IS NULL OR credit_limit >= 0),
    CONSTRAINT customers_party_fk FOREIGN KEY (party_id) REFERENCES parties(id)
) ENGINE=InnoDB;

CREATE TABLE travel_agents (
    id BINARY(16) NOT NULL,
    party_id BINARY(16) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY travel_agents_party_uq (party_id),
    CONSTRAINT travel_agents_party_fk FOREIGN KEY (party_id) REFERENCES parties(id)
) ENGINE=InnoDB;

CREATE TABLE suppliers (
    id BINARY(16) NOT NULL,
    party_id BINARY(16) NOT NULL,
    settlement_channel VARCHAR(32) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY suppliers_party_uq (party_id),
    CONSTRAINT suppliers_settlement_channel CHECK (
        settlement_channel IS NULL OR settlement_channel IN ('airline','bsp','consolidator','other')
    ),
    CONSTRAINT suppliers_party_fk FOREIGN KEY (party_id) REFERENCES parties(id)
) ENGINE=InnoDB;

CREATE TABLE airlines (
    id BINARY(16) NOT NULL,
    code VARCHAR(32) NOT NULL,
    name VARCHAR(191) NOT NULL,
    ticket_prefix VARCHAR(32) NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY airlines_code_uq (code)
) ENGINE=InnoDB;

CREATE TABLE locations (
    id BINARY(16) NOT NULL,
    kind VARCHAR(16) NOT NULL,
    code VARCHAR(64) NOT NULL,
    name VARCHAR(191) NOT NULL,
    parent_id BINARY(16) NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY locations_kind_code_uq (kind, code),
    KEY locations_parent_idx (parent_id),
    CONSTRAINT locations_kind CHECK (kind IN ('country','city','airport')),
    CONSTRAINT locations_not_own_parent CHECK (parent_id IS NULL OR parent_id <> id),
    CONSTRAINT locations_parent_fk FOREIGN KEY (parent_id) REFERENCES locations(id)
) ENGINE=InnoDB;

CREATE TABLE cost_centres (
    id BINARY(16) NOT NULL,
    code VARCHAR(100) NOT NULL,
    name VARCHAR(191) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY cost_centres_code_uq (code)
) ENGINE=InnoDB;

CREATE TABLE employees (
    id BINARY(16) NOT NULL,
    branch_id BINARY(16) NULL,
    user_id BINARY(16) NULL,
    employee_code VARCHAR(100) NOT NULL,
    name VARCHAR(191) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY employees_user_uq (user_id),
    UNIQUE KEY employees_employee_code_uq (employee_code),
    KEY employees_branch_idx (branch_id),
    CONSTRAINT employees_branch_fk FOREIGN KEY (branch_id) REFERENCES branches(id),
    CONSTRAINT employees_user_fk FOREIGN KEY (user_id) REFERENCES users(id)
) ENGINE=InnoDB;

CREATE TABLE projects (
    id BINARY(16) NOT NULL,
    code VARCHAR(100) NOT NULL,
    name VARCHAR(191) NOT NULL,
    customer_id BINARY(16) NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY projects_code_uq (code),
    KEY projects_customer_idx (customer_id),
    CONSTRAINT projects_customer_fk FOREIGN KEY (customer_id) REFERENCES customers(id)
) ENGINE=InnoDB;

CREATE TABLE services (
    id BINARY(16) NOT NULL,
    code VARCHAR(100) NOT NULL,
    name VARCHAR(191) NOT NULL,
    sales_class VARCHAR(32) NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY services_code_uq (code),
    CONSTRAINT services_sales_class CHECK (
        sales_class IS NULL OR sales_class IN ('international','domestic','miscellaneous')
    )
) ENGINE=InnoDB;

CREATE TABLE exchange_rates (
    id BINARY(16) NOT NULL,
    from_currency VARCHAR(3) NOT NULL,
    to_currency VARCHAR(3) NOT NULL,
    effective_date DATE NOT NULL,
    rate DECIMAL(65,30) NOT NULL,
    verified_by BINARY(16) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY exchange_rates_pair_date_uq (from_currency, to_currency, effective_date),
    KEY exchange_rates_verified_by_idx (verified_by),
    CONSTRAINT exchange_rates_rate_precision CHECK (rate > 0 AND rate = TRUNCATE(rate, 12)),
    CONSTRAINT exchange_rates_different_currency CHECK (from_currency <> to_currency),
    CONSTRAINT exchange_rates_from_currency_fk FOREIGN KEY (from_currency) REFERENCES currencies(code),
    CONSTRAINT exchange_rates_to_currency_fk FOREIGN KEY (to_currency) REFERENCES currencies(code),
    CONSTRAINT exchange_rates_verified_by_fk FOREIGN KEY (verified_by) REFERENCES users(id)
) ENGINE=InnoDB;

CREATE TABLE accounts (
    id BINARY(16) NOT NULL,
    code VARCHAR(100) NOT NULL,
    name VARCHAR(191) NOT NULL,
    classification VARCHAR(32) NOT NULL,
    parent_id BINARY(16) NULL,
    postable BOOLEAN NOT NULL DEFAULT TRUE,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY accounts_code_uq (code),
    KEY accounts_parent_idx (parent_id),
    CONSTRAINT accounts_classification CHECK (
        classification IN ('asset','liability','equity','income','expense')
    ),
    CONSTRAINT accounts_not_own_parent CHECK (parent_id IS NULL OR parent_id <> id),
    CONSTRAINT accounts_parent_fk FOREIGN KEY (parent_id) REFERENCES accounts(id)
) ENGINE=InnoDB;

CREATE TABLE bank_cash_accounts (
    id BINARY(16) NOT NULL,
    ledger_account_id BINARY(16) NOT NULL,
    kind VARCHAR(16) NOT NULL,
    currency_code VARCHAR(3) NOT NULL,
    bank_name VARCHAR(191) NULL,
    account_number_ciphertext VARBINARY(1024) NULL,
    masked_last_four CHAR(4) NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY bank_cash_accounts_ledger_account_uq (ledger_account_id),
    KEY bank_cash_accounts_currency_idx (currency_code),
    CONSTRAINT bank_cash_accounts_kind CHECK (kind IN ('bank','cash')),
    CONSTRAINT bank_cash_accounts_mask_format CHECK (
        masked_last_four IS NULL OR REGEXP_LIKE(masked_last_four, '^[0-9]{4}$', 'c')
    ),
    CONSTRAINT bank_cash_accounts_cash_metadata CHECK (
        kind = 'bank' OR (bank_name IS NULL AND account_number_ciphertext IS NULL AND masked_last_four IS NULL)
    ),
    CONSTRAINT bank_cash_accounts_ledger_account_fk FOREIGN KEY (ledger_account_id) REFERENCES accounts(id),
    CONSTRAINT bank_cash_accounts_currency_fk FOREIGN KEY (currency_code) REFERENCES currencies(code)
) ENGINE=InnoDB;

CREATE TABLE financial_years (
    id BINARY(16) NOT NULL,
    label VARCHAR(100) NOT NULL,
    starts_on DATE NOT NULL,
    ends_on DATE NOT NULL,
    closed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY financial_years_label_uq (label),
    CONSTRAINT financial_years_date_range CHECK (starts_on <= ends_on)
) ENGINE=InnoDB;

CREATE TABLE accounting_periods (
    id BINARY(16) NOT NULL,
    financial_year_id BINARY(16) NOT NULL,
    label VARCHAR(100) NOT NULL,
    starts_on DATE NOT NULL,
    ends_on DATE NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'open',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY accounting_periods_year_label_uq (financial_year_id, label),
    KEY accounting_periods_date_idx (starts_on, ends_on),
    CONSTRAINT accounting_periods_status CHECK (status IN ('open','closed')),
    CONSTRAINT accounting_periods_date_range CHECK (starts_on <= ends_on),
    CONSTRAINT accounting_periods_financial_year_fk FOREIGN KEY (financial_year_id) REFERENCES financial_years(id)
) ENGINE=InnoDB;

CREATE TABLE voucher_types (
    id BINARY(16) NOT NULL,
    code VARCHAR(100) NOT NULL,
    name VARCHAR(191) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY voucher_types_code_uq (code)
) ENGINE=InnoDB;

CREATE TABLE numbering_rules (
    id BINARY(16) NOT NULL,
    branch_id BINARY(16) NOT NULL,
    document_kind VARCHAR(64) NOT NULL,
    financial_year_id BINARY(16) NOT NULL,
    prefix VARCHAR(64) NOT NULL,
    next_number BIGINT NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY numbering_rules_scope_uq (branch_id, document_kind, financial_year_id),
    KEY numbering_rules_year_idx (financial_year_id),
    CONSTRAINT numbering_rules_next_number_positive CHECK (next_number > 0),
    CONSTRAINT numbering_rules_branch_fk FOREIGN KEY (branch_id) REFERENCES branches(id),
    CONSTRAINT numbering_rules_financial_year_fk FOREIGN KEY (financial_year_id) REFERENCES financial_years(id)
) ENGINE=InnoDB;

CREATE TABLE tax_rules (
    id BINARY(16) NOT NULL,
    code VARCHAR(100) NOT NULL,
    version INT NOT NULL,
    component VARCHAR(100) NOT NULL,
    rate_percent DECIMAL(65,30) NULL,
    basis JSON NOT NULL,
    effective_from DATE NOT NULL,
    effective_to DATE NULL,
    verification_status VARCHAR(40) NOT NULL DEFAULT 'to_be_verified_by_finance',
    verified_by BINARY(16) NULL,
    verified_at DATETIME(6) NULL,
    active BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY tax_rules_code_version_uq (code, version),
    KEY tax_rules_verified_by_idx (verified_by),
    CONSTRAINT tax_rules_version_positive CHECK (version > 0),
    CONSTRAINT tax_rules_rate_precision CHECK (
        rate_percent IS NULL OR
        (rate_percent BETWEEN 0 AND 100 AND rate_percent = TRUNCATE(rate_percent, 6))
    ),
    CONSTRAINT tax_rules_basis_object CHECK (JSON_TYPE(basis) = 'OBJECT'),
    CONSTRAINT tax_rules_verification_status CHECK (
        verification_status IN ('to_be_verified_by_finance','verified','rejected')
    ),
    CONSTRAINT tax_rules_effective_range CHECK (effective_to IS NULL OR effective_to >= effective_from),
    CONSTRAINT tax_rules_active_verified CHECK (
        NOT active OR (
            verification_status = 'verified' AND verified_by IS NOT NULL
            AND verified_at IS NOT NULL AND rate_percent IS NOT NULL
        )
    ),
    CONSTRAINT tax_rules_verified_by_fk FOREIGN KEY (verified_by) REFERENCES users(id)
) ENGINE=InnoDB;

CREATE TABLE commercial_rules (
    id BINARY(16) NOT NULL,
    kind VARCHAR(32) NOT NULL,
    code VARCHAR(100) NOT NULL,
    configuration JSON NOT NULL,
    verification_status VARCHAR(40) NOT NULL DEFAULT 'to_be_verified_by_finance',
    active BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY commercial_rules_code_uq (code),
    CONSTRAINT commercial_rules_kind CHECK (
        kind IN ('commission','discount','markup','cancellation','refund','tds')
    ),
    CONSTRAINT commercial_rules_configuration_object CHECK (JSON_TYPE(configuration) = 'OBJECT'),
    CONSTRAINT commercial_rules_active_verified CHECK (NOT active OR verification_status = 'verified')
) ENGINE=InnoDB;

CREATE TABLE approval_rules (
    id BINARY(16) NOT NULL,
    code VARCHAR(100) NOT NULL,
    transaction_kind VARCHAR(64) NOT NULL,
    currency_code VARCHAR(3) NULL,
    minimum_amount DECIMAL(65,30) NULL,
    maximum_amount DECIMAL(65,30) NULL,
    required_role_id BINARY(16) NOT NULL,
    conditions JSON NOT NULL DEFAULT (JSON_OBJECT()),
    active BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY approval_rules_code_uq (code),
    KEY approval_rules_currency_idx (currency_code),
    KEY approval_rules_required_role_idx (required_role_id),
    CONSTRAINT approval_rules_amount_precision CHECK (
        (minimum_amount IS NULL OR
            (ABS(minimum_amount) < 1000000000000000000 AND minimum_amount = TRUNCATE(minimum_amount, 6)))
        AND
        (maximum_amount IS NULL OR
            (ABS(maximum_amount) < 1000000000000000000 AND maximum_amount = TRUNCATE(maximum_amount, 6)))
    ),
    CONSTRAINT approval_rules_amount_range CHECK (
        minimum_amount IS NULL OR maximum_amount IS NULL OR minimum_amount <= maximum_amount
    ),
    CONSTRAINT approval_rules_currency_fk FOREIGN KEY (currency_code) REFERENCES currencies(code),
    CONSTRAINT approval_rules_required_role_fk FOREIGN KEY (required_role_id) REFERENCES roles(id)
) ENGINE=InnoDB;

CREATE TABLE approval_requests (
    id BINARY(16) NOT NULL,
    rule_id BINARY(16) NOT NULL,
    entity_type VARCHAR(64) NOT NULL,
    entity_id BINARY(16) NOT NULL,
    entity_version INT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    requested_by BINARY(16) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY approval_requests_version_uq (rule_id, entity_type, entity_id, entity_version),
    KEY approval_requests_requested_by_idx (requested_by),
    CONSTRAINT approval_requests_entity_version_positive CHECK (entity_version > 0),
    CONSTRAINT approval_requests_status CHECK (status IN ('pending','approved','rejected','withdrawn')),
    CONSTRAINT approval_requests_rule_fk FOREIGN KEY (rule_id) REFERENCES approval_rules(id),
    CONSTRAINT approval_requests_requested_by_fk FOREIGN KEY (requested_by) REFERENCES users(id)
) ENGINE=InnoDB;

CREATE TABLE approval_decisions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    request_id BINARY(16) NOT NULL,
    actor_id BINARY(16) NOT NULL,
    decision VARCHAR(16) NOT NULL,
    reason VARCHAR(1000) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY approval_decisions_request_actor_uq (request_id, actor_id),
    KEY approval_decisions_actor_idx (actor_id),
    CONSTRAINT approval_decisions_reason_nonempty CHECK (CHAR_LENGTH(TRIM(reason)) > 0),
    CONSTRAINT approval_decisions_decision CHECK (decision IN ('approved','rejected')),
    CONSTRAINT approval_decisions_request_fk FOREIGN KEY (request_id) REFERENCES approval_requests(id),
    CONSTRAINT approval_decisions_actor_fk FOREIGN KEY (actor_id) REFERENCES users(id)
) ENGINE=InnoDB;

CREATE TABLE mir_files (
    id BINARY(16) NOT NULL,
    sha256 CHAR(64) NOT NULL,
    original_name VARCHAR(255) NOT NULL,
    raw_bytes LONGBLOB NOT NULL,
    source_path VARCHAR(1024) NULL,
    status VARCHAR(32) NOT NULL,
    imported_by BINARY(16) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY mir_files_sha256_uq (sha256),
    KEY mir_files_imported_by_idx (imported_by),
    CONSTRAINT mir_files_sha256_format CHECK (REGEXP_LIKE(sha256, '^[a-f0-9]{64}$', 'c')),
    CONSTRAINT mir_files_sha256_matches_bytes CHECK (sha256 = LOWER(SHA2(raw_bytes, 256))),
    CONSTRAINT mir_files_status CHECK (
        status IN ('uploaded','extracted','validation_required','validated','linked','posted','failed','duplicate','reprocessed')
    ),
    CONSTRAINT mir_files_imported_by_fk FOREIGN KEY (imported_by) REFERENCES users(id)
) ENGINE=InnoDB;

CREATE TABLE mir_source_lines (
    file_id BINARY(16) NOT NULL,
    line_number INT NOT NULL,
    raw_bytes LONGBLOB NOT NULL,
    terminator_bytes VARBINARY(16) NOT NULL,
    PRIMARY KEY (file_id, line_number),
    CONSTRAINT mir_source_lines_line_positive CHECK (line_number > 0),
    CONSTRAINT mir_source_lines_file_fk FOREIGN KEY (file_id) REFERENCES mir_files(id)
) ENGINE=InnoDB;

CREATE TABLE mir_parse_runs (
    id BINARY(16) NOT NULL,
    file_id BINARY(16) NOT NULL,
    parser_version VARCHAR(100) NOT NULL,
    run_key VARCHAR(191) NOT NULL,
    status VARCHAR(32) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    completed_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY mir_parse_runs_run_key_uq (run_key),
    UNIQUE KEY mir_parse_runs_id_file_uq (id, file_id),
    KEY mir_parse_runs_file_idx (file_id),
    CONSTRAINT mir_parse_runs_status CHECK (
        status IN ('uploaded','extracted','validation_required','validated','linked','posted','failed','duplicate','reprocessed')
    ),
    CONSTRAINT mir_parse_runs_file_fk FOREIGN KEY (file_id) REFERENCES mir_files(id)
) ENGINE=InnoDB;

CREATE TABLE mir_records (
    id BINARY(16) NOT NULL,
    file_id BINARY(16) NOT NULL,
    parse_run_id BINARY(16) NOT NULL,
    record_key VARCHAR(191) NOT NULL,
    passenger_key VARCHAR(191) NOT NULL,
    status VARCHAR(32) NOT NULL,
    issues JSON NOT NULL DEFAULT (JSON_ARRAY()),
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY mir_records_run_record_passenger_uq (parse_run_id, record_key, passenger_key),
    KEY mir_records_file_idx (file_id),
    CONSTRAINT mir_records_status CHECK (
        status IN ('uploaded','extracted','validation_required','validated','linked','posted','failed','duplicate','reprocessed')
    ),
    CONSTRAINT mir_records_issues_array CHECK (JSON_TYPE(issues) = 'ARRAY'),
    CONSTRAINT mir_records_parse_run_file_fk FOREIGN KEY (parse_run_id, file_id) REFERENCES mir_parse_runs(id, file_id),
    CONSTRAINT mir_records_file_fk FOREIGN KEY (file_id) REFERENCES mir_files(id)
) ENGINE=InnoDB;

CREATE TABLE mir_fields (
    id BINARY(16) NOT NULL,
    record_id BINARY(16) NOT NULL,
    source_field_id VARCHAR(191) NOT NULL,
    label VARCHAR(191) NOT NULL,
    raw_value TEXT NULL,
    extracted_value JSON NULL,
    line_number INT NOT NULL,
    byte_start BIGINT NOT NULL,
    byte_end BIGINT NOT NULL,
    extraction_status VARCHAR(64) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY mir_fields_record_source_field_uq (record_id, source_field_id),
    CONSTRAINT mir_fields_line_positive CHECK (line_number > 0),
    CONSTRAINT mir_fields_byte_start_nonnegative CHECK (byte_start >= 0),
    CONSTRAINT mir_fields_byte_range CHECK (byte_end >= byte_start),
    CONSTRAINT mir_fields_record_fk FOREIGN KEY (record_id) REFERENCES mir_records(id)
) ENGINE=InnoDB;

CREATE TABLE sales_documents (
    id BINARY(16) NOT NULL,
    company_id BINARY(16) NOT NULL,
    branch_id BINARY(16) NOT NULL,
    sales_class VARCHAR(32) NOT NULL,
    document_number VARCHAR(100) NULL,
    document_date DATE NOT NULL,
    due_date DATE NULL,
    customer_id BINARY(16) NULL,
    agent_id BINARY(16) NULL,
    cost_centre_id BINARY(16) NULL,
    employee_id BINARY(16) NULL,
    client_employee_code VARCHAR(100) NULL,
    project_id BINARY(16) NULL,
    tour_code VARCHAR(100) NULL,
    currency_code VARCHAR(3) NOT NULL,
    exchange_rate DECIMAL(65,30) NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'draft',
    version INT NOT NULL DEFAULT 1,
    reference VARCHAR(191) NULL,
    narration TEXT NULL,
    organization JSON NOT NULL DEFAULT (JSON_OBJECT()),
    classification JSON NOT NULL DEFAULT (JSON_OBJECT()),
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY sales_documents_branch_number_uq (branch_id, document_number),
    KEY sales_date_class_status_idx (document_date, sales_class, status),
    KEY sales_customer_due_idx (customer_id, due_date),
    KEY sales_documents_customer_idx (customer_id),
    KEY sales_documents_agent_idx (agent_id),
    KEY sales_documents_cost_centre_idx (cost_centre_id),
    KEY sales_documents_employee_idx (employee_id),
    KEY sales_documents_project_idx (project_id),
    KEY sales_documents_currency_idx (currency_code),
    CONSTRAINT sales_documents_sales_class CHECK (sales_class IN ('international','domestic','miscellaneous')),
    CONSTRAINT sales_documents_exchange_rate_precision CHECK (
        exchange_rate IS NULL OR (exchange_rate > 0 AND exchange_rate = TRUNCATE(exchange_rate, 12))
    ),
    CONSTRAINT sales_documents_status CHECK (
        status IN ('draft','validation','verification','verified','approval','approved','posted','cancelled','refunded','reversed')
    ),
    CONSTRAINT sales_documents_version_positive CHECK (version > 0),
    CONSTRAINT sales_documents_company_branch_fk FOREIGN KEY (company_id, branch_id) REFERENCES branches(company_id, id),
    CONSTRAINT sales_documents_customer_fk FOREIGN KEY (customer_id) REFERENCES customers(id),
    CONSTRAINT sales_documents_agent_fk FOREIGN KEY (agent_id) REFERENCES travel_agents(id),
    CONSTRAINT sales_documents_cost_centre_fk FOREIGN KEY (cost_centre_id) REFERENCES cost_centres(id),
    CONSTRAINT sales_documents_employee_fk FOREIGN KEY (employee_id) REFERENCES employees(id),
    CONSTRAINT sales_documents_project_fk FOREIGN KEY (project_id) REFERENCES projects(id),
    CONSTRAINT sales_documents_currency_fk FOREIGN KEY (currency_code) REFERENCES currencies(code)
) ENGINE=InnoDB;

CREATE TABLE tickets (
    id BINARY(16) NOT NULL,
    ticket_number VARCHAR(191) NOT NULL,
    ticket_number_key VARCHAR(191)
        GENERATED ALWAYS AS (REGEXP_REPLACE(UPPER(ticket_number), '[^A-Z0-9]', '')) STORED,
    airline_id BINARY(16) NULL,
    pnr VARCHAR(64) NULL,
    passenger_name VARCHAR(191) NULL,
    passport_ciphertext VARBINARY(4096) NULL,
    nationality VARCHAR(100) NULL,
    ticket_date DATE NULL,
    ticket_date_source VARCHAR(100) NULL,
    travel_date DATE NULL,
    travel_date_source VARCHAR(100) NULL,
    sector VARCHAR(191) NULL,
    crs VARCHAR(100) NULL,
    fare_basis VARCHAR(100) NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'draft',
    original_ticket_id BINARY(16) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY tickets_ticket_number_uq (ticket_number),
    UNIQUE KEY tickets_ticket_number_key_uq (ticket_number_key),
    KEY tickets_airline_idx (airline_id),
    KEY tickets_pnr_idx (pnr),
    KEY tickets_passenger_idx (passenger_name),
    KEY tickets_original_ticket_idx (original_ticket_id),
    CONSTRAINT tickets_number_canonical CHECK (
        CHAR_LENGTH(TRIM(ticket_number)) > 0 AND CAST(ticket_number AS BINARY) = CAST(UPPER(TRIM(ticket_number)) AS BINARY)
    ),
    CONSTRAINT tickets_status CHECK (
        status IN ('draft','issued','reissued','void','cancelled','refunded','no_show','unutilized')
    ),
    CONSTRAINT tickets_not_own_original CHECK (original_ticket_id IS NULL OR original_ticket_id <> id),
    CONSTRAINT tickets_airline_fk FOREIGN KEY (airline_id) REFERENCES airlines(id),
    CONSTRAINT tickets_original_ticket_fk FOREIGN KEY (original_ticket_id) REFERENCES tickets(id)
) ENGINE=InnoDB;

CREATE TABLE sales_lines (
    id BINARY(16) NOT NULL,
    document_id BINARY(16) NOT NULL,
    line_number INT NOT NULL,
    ticket_id BINARY(16) NULL,
    mir_record_id BINARY(16) NULL,
    supplier_id BINARY(16) NULL,
    service_id BINARY(16) NULL,
    service_category VARCHAR(100) NOT NULL,
    description TEXT NULL,
    service_from DATE NULL,
    service_to DATE NULL,
    quantity DECIMAL(18,6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY sales_lines_document_number_uq (document_id, line_number),
    UNIQUE KEY sales_lines_document_ticket_uq (document_id, ticket_id),
    UNIQUE KEY sales_lines_mir_record_uq (mir_record_id),
    UNIQUE KEY ticket_original_sale_uq (ticket_id),
    KEY sales_lines_supplier_idx (supplier_id),
    KEY sales_lines_service_idx (service_id),
    CONSTRAINT sales_lines_line_positive CHECK (line_number > 0),
    CONSTRAINT sales_lines_quantity_positive CHECK (quantity IS NULL OR quantity > 0),
    CONSTRAINT sales_lines_document_fk FOREIGN KEY (document_id) REFERENCES sales_documents(id),
    CONSTRAINT sales_lines_ticket_fk FOREIGN KEY (ticket_id) REFERENCES tickets(id),
    CONSTRAINT sales_lines_mir_record_fk FOREIGN KEY (mir_record_id) REFERENCES mir_records(id),
    CONSTRAINT sales_lines_supplier_fk FOREIGN KEY (supplier_id) REFERENCES suppliers(id),
    CONSTRAINT sales_lines_service_fk FOREIGN KEY (service_id) REFERENCES services(id)
) ENGINE=InnoDB;

CREATE TABLE ticket_segments (
    id BINARY(16) NOT NULL,
    ticket_id BINARY(16) NOT NULL,
    sequence INT NOT NULL,
    origin_id BINARY(16) NULL,
    destination_id BINARY(16) NULL,
    flight_number VARCHAR(64) NULL,
    departure_at DATETIME(6) NULL,
    arrival_at DATETIME(6) NULL,
    departure_source VARCHAR(100) NULL,
    arrival_source VARCHAR(100) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY ticket_segments_ticket_sequence_uq (ticket_id, sequence),
    KEY ticket_segments_origin_idx (origin_id),
    KEY ticket_segments_destination_idx (destination_id),
    CONSTRAINT ticket_segments_sequence_positive CHECK (sequence > 0),
    CONSTRAINT ticket_segments_ticket_fk FOREIGN KEY (ticket_id) REFERENCES tickets(id),
    CONSTRAINT ticket_segments_origin_fk FOREIGN KEY (origin_id) REFERENCES locations(id),
    CONSTRAINT ticket_segments_destination_fk FOREIGN KEY (destination_id) REFERENCES locations(id)
) ENGINE=InnoDB;

CREATE TABLE accounting_components (
    id BINARY(16) NOT NULL,
    code VARCHAR(100) NOT NULL,
    label VARCHAR(191) NOT NULL,
    effect VARCHAR(16) NOT NULL,
    category VARCHAR(100) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY accounting_components_code_uq (code),
    CONSTRAINT accounting_components_effect CHECK (effect IN ('add','subtract'))
) ENGINE=InnoDB;

CREATE TABLE line_accounting (
    id BINARY(16) NOT NULL,
    sales_line_id BINARY(16) NOT NULL,
    side VARCHAR(16) NOT NULL,
    currency_code VARCHAR(3) NOT NULL,
    total DECIMAL(65,30) NULL,
    provenance VARCHAR(32) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY line_accounting_line_side_uq (sales_line_id, side),
    KEY line_accounting_currency_idx (currency_code),
    CONSTRAINT line_accounting_side CHECK (side IN ('client','supplier')),
    CONSTRAINT line_accounting_total_precision CHECK (
        total IS NULL OR (ABS(total) < 1000000000000000000 AND total = TRUNCATE(total, 6))
    ),
    CONSTRAINT line_accounting_provenance CHECK (
        provenance IN ('user_entered','mir_extracted','system_calculated','user_overridden')
    ),
    CONSTRAINT line_accounting_sales_line_fk FOREIGN KEY (sales_line_id) REFERENCES sales_lines(id),
    CONSTRAINT line_accounting_currency_fk FOREIGN KEY (currency_code) REFERENCES currencies(code)
) ENGINE=InnoDB;

CREATE TABLE line_accounting_components (
    id BINARY(16) NOT NULL,
    accounting_id BINARY(16) NOT NULL,
    component_id BINARY(16) NOT NULL,
    amount DECIMAL(65,30) NOT NULL,
    tax_rule_id BINARY(16) NULL,
    source_field_id BINARY(16) NULL,
    provenance VARCHAR(32) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY line_accounting_components_account_component_uq (accounting_id, component_id),
    KEY line_accounting_components_component_idx (component_id),
    KEY line_accounting_components_tax_rule_idx (tax_rule_id),
    KEY line_accounting_components_source_field_idx (source_field_id),
    CONSTRAINT line_accounting_components_amount_precision CHECK (
        ABS(amount) < 1000000000000000000 AND amount = TRUNCATE(amount, 6)
    ),
    CONSTRAINT line_accounting_components_amount_nonnegative CHECK (amount >= 0),
    CONSTRAINT line_accounting_components_provenance CHECK (
        provenance IN ('user_entered','mir_extracted','system_calculated','user_overridden')
    ),
    CONSTRAINT line_accounting_components_source_required CHECK (
        provenance <> 'mir_extracted' OR source_field_id IS NOT NULL
    ),
    CONSTRAINT line_accounting_components_accounting_fk FOREIGN KEY (accounting_id) REFERENCES line_accounting(id),
    CONSTRAINT line_accounting_components_component_fk FOREIGN KEY (component_id) REFERENCES accounting_components(id),
    CONSTRAINT line_accounting_components_tax_rule_fk FOREIGN KEY (tax_rule_id) REFERENCES tax_rules(id),
    CONSTRAINT line_accounting_components_source_field_fk FOREIGN KEY (source_field_id) REFERENCES mir_fields(id)
) ENGINE=InnoDB;

CREATE TABLE field_corrections (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    sales_line_id BINARY(16) NULL,
    mir_field_id BINARY(16) NULL,
    field_name VARCHAR(191) NOT NULL,
    old_value JSON NULL,
    new_value JSON NULL,
    reason VARCHAR(1000) NOT NULL,
    actor_id BINARY(16) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    KEY field_corrections_sales_line_idx (sales_line_id),
    KEY field_corrections_mir_field_idx (mir_field_id),
    KEY field_corrections_actor_idx (actor_id),
    CONSTRAINT field_corrections_reason_nonempty CHECK (CHAR_LENGTH(TRIM(reason)) > 0),
    CONSTRAINT field_corrections_target_required CHECK (sales_line_id IS NOT NULL OR mir_field_id IS NOT NULL),
    CONSTRAINT field_corrections_sales_line_fk FOREIGN KEY (sales_line_id) REFERENCES sales_lines(id),
    CONSTRAINT field_corrections_mir_field_fk FOREIGN KEY (mir_field_id) REFERENCES mir_fields(id),
    CONSTRAINT field_corrections_actor_fk FOREIGN KEY (actor_id) REFERENCES users(id)
) ENGINE=InnoDB;

CREATE TABLE value_events (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    entity_type VARCHAR(64) NOT NULL,
    entity_id BINARY(16) NOT NULL,
    field_name VARCHAR(191) NOT NULL,
    value JSON NULL,
    provenance VARCHAR(32) NOT NULL,
    source_field_id BINARY(16) NULL,
    calculation_version VARCHAR(100) NULL,
    previous_event_id BIGINT UNSIGNED NULL,
    actor_id BINARY(16) NULL,
    reason VARCHAR(1000) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY value_events_previous_event_uq (previous_event_id),
    KEY value_events_field_history_idx (entity_type, entity_id, field_name, id),
    KEY value_events_source_field_idx (source_field_id),
    KEY value_events_actor_idx (actor_id),
    CONSTRAINT value_events_provenance CHECK (
        provenance IN ('user_entered','mir_extracted','system_calculated','user_overridden')
    ),
    CONSTRAINT value_events_source_required CHECK (
        provenance <> 'mir_extracted' OR source_field_id IS NOT NULL
    ),
    CONSTRAINT value_events_calculation_version_required CHECK (
        provenance <> 'system_calculated' OR NULLIF(TRIM(calculation_version), '') IS NOT NULL
    ),
    CONSTRAINT value_events_override_metadata_required CHECK (
        provenance <> 'user_overridden' OR
        (actor_id IS NOT NULL AND previous_event_id IS NOT NULL AND NULLIF(TRIM(reason), '') IS NOT NULL)
    ),
    CONSTRAINT value_events_source_field_fk FOREIGN KEY (source_field_id) REFERENCES mir_fields(id),
    CONSTRAINT value_events_previous_event_fk FOREIGN KEY (previous_event_id) REFERENCES value_events(id),
    CONSTRAINT value_events_actor_fk FOREIGN KEY (actor_id) REFERENCES users(id)
) ENGINE=InnoDB;

CREATE TABLE attachments (
    id BINARY(16) NOT NULL,
    entity_type VARCHAR(64) NOT NULL,
    entity_id BINARY(16) NOT NULL,
    storage_key VARCHAR(1024) NOT NULL,
    original_name VARCHAR(255) NOT NULL,
    sha256 CHAR(64) NOT NULL,
    content_type VARCHAR(191) NOT NULL,
    byte_length BIGINT NOT NULL,
    created_by BINARY(16) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    KEY attachments_entity_idx (entity_type, entity_id),
    KEY attachments_created_by_idx (created_by),
    CONSTRAINT attachments_sha256_format CHECK (REGEXP_LIKE(sha256, '^[a-f0-9]{64}$', 'c')),
    CONSTRAINT attachments_byte_length_nonnegative CHECK (byte_length >= 0),
    CONSTRAINT attachments_created_by_fk FOREIGN KEY (created_by) REFERENCES users(id)
) ENGINE=InnoDB;

CREATE TABLE after_sales (
    id BINARY(16) NOT NULL,
    original_line_id BINARY(16) NOT NULL,
    kind VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'requested',
    replacement_ticket_id BINARY(16) NULL,
    reason TEXT NOT NULL,
    currency_code VARCHAR(3) NOT NULL,
    supplier_penalty DECIMAL(65,30) NULL,
    agency_fee DECIMAL(65,30) NULL,
    refund_amount DECIMAL(65,30) NULL,
    rule_id BINARY(16) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    KEY after_sales_replacement_ticket_idx (replacement_ticket_id),
    KEY after_sales_currency_idx (currency_code),
    KEY after_sales_rule_idx (rule_id),
    CONSTRAINT after_sales_kind CHECK (
        kind IN ('cancellation','refund','reissue','exchange','void','adm','acm','no_show','adjustment')
    ),
    CONSTRAINT after_sales_status CHECK (
        status IN ('requested','confirmed','approved','received','paid','posted','rejected','reversed')
    ),
    CONSTRAINT after_sales_amount_precision CHECK (
        (supplier_penalty IS NULL OR
            (ABS(supplier_penalty) < 1000000000000000000 AND supplier_penalty = TRUNCATE(supplier_penalty, 6)))
        AND (agency_fee IS NULL OR
            (ABS(agency_fee) < 1000000000000000000 AND agency_fee = TRUNCATE(agency_fee, 6)))
        AND (refund_amount IS NULL OR
            (ABS(refund_amount) < 1000000000000000000 AND refund_amount = TRUNCATE(refund_amount, 6)))
    ),
    CONSTRAINT after_sales_original_line_fk FOREIGN KEY (original_line_id) REFERENCES sales_lines(id),
    CONSTRAINT after_sales_replacement_ticket_fk FOREIGN KEY (replacement_ticket_id) REFERENCES tickets(id),
    CONSTRAINT after_sales_currency_fk FOREIGN KEY (currency_code) REFERENCES currencies(code),
    CONSTRAINT after_sales_rule_fk FOREIGN KEY (rule_id) REFERENCES commercial_rules(id)
) ENGINE=InnoDB;

CREATE TABLE payments (
    id BINARY(16) NOT NULL,
    branch_id BINARY(16) NOT NULL,
    payment_number VARCHAR(100) NULL,
    direction VARCHAR(16) NOT NULL,
    payment_date DATE NOT NULL,
    posting_date DATE NULL,
    bank_cash_account_id BINARY(16) NOT NULL,
    currency_code VARCHAR(3) NOT NULL,
    method VARCHAR(16) NOT NULL,
    reference VARCHAR(191) NULL,
    cheque_number VARCHAR(100) NULL,
    cheque_date DATE NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'draft',
    narration TEXT NULL,
    version INT NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY payments_branch_number_uq (branch_id, payment_number),
    KEY payment_reference_review_idx (bank_cash_account_id, reference),
    KEY payments_currency_idx (currency_code),
    CONSTRAINT payments_direction CHECK (direction IN ('receipt','payment','transfer')),
    CONSTRAINT payments_method CHECK (method IN ('cash','transfer','cheque')),
    CONSTRAINT payments_status CHECK (
        status IN ('draft','validation','verification','verified','approval','approved','posted','cancelled','refunded','reversed')
    ),
    CONSTRAINT payments_version_positive CHECK (version > 0),
    CONSTRAINT payments_cheque_fields CHECK (
        method = 'cheque' OR (cheque_number IS NULL AND cheque_date IS NULL)
    ),
    CONSTRAINT payments_branch_fk FOREIGN KEY (branch_id) REFERENCES branches(id),
    CONSTRAINT payments_bank_cash_account_fk FOREIGN KEY (bank_cash_account_id) REFERENCES bank_cash_accounts(id),
    CONSTRAINT payments_currency_fk FOREIGN KEY (currency_code) REFERENCES currencies(code)
) ENGINE=InnoDB;

CREATE TABLE payment_lines (
    id BINARY(16) NOT NULL,
    payment_id BINARY(16) NOT NULL,
    line_number INT NOT NULL,
    party_id BINARY(16) NULL,
    ledger_account_id BINARY(16) NOT NULL,
    amount DECIMAL(65,30) NOT NULL,
    invoice_reference VARCHAR(191) NULL,
    narration TEXT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY payment_lines_payment_number_uq (payment_id, line_number),
    KEY payment_lines_party_idx (party_id),
    KEY payment_lines_ledger_account_idx (ledger_account_id),
    CONSTRAINT payment_lines_line_positive CHECK (line_number > 0),
    CONSTRAINT payment_lines_amount_precision CHECK (
        ABS(amount) < 1000000000000000000 AND amount = TRUNCATE(amount, 6)
    ),
    CONSTRAINT payment_lines_amount_positive CHECK (amount > 0),
    CONSTRAINT payment_lines_payment_fk FOREIGN KEY (payment_id) REFERENCES payments(id),
    CONSTRAINT payment_lines_party_fk FOREIGN KEY (party_id) REFERENCES parties(id),
    CONSTRAINT payment_lines_ledger_account_fk FOREIGN KEY (ledger_account_id) REFERENCES accounts(id)
) ENGINE=InnoDB;

CREATE TABLE vouchers (
    id BINARY(16) NOT NULL,
    branch_id BINARY(16) NOT NULL,
    voucher_type_id BINARY(16) NOT NULL,
    voucher_number VARCHAR(100) NOT NULL,
    posting_date DATE NOT NULL,
    period_id BINARY(16) NOT NULL,
    currency_code VARCHAR(3) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'draft',
    idempotency_key VARCHAR(191) NOT NULL,
    payload_sha256 BINARY(32) NOT NULL,
    source_sales_id BINARY(16) NULL,
    source_payment_id BINARY(16) NULL,
    source_after_sales_id BINARY(16) NULL,
    reversal_of BINARY(16) NULL,
    reversed_by BINARY(16) NULL,
    reversal_reason VARCHAR(1000) NULL,
    narration TEXT NULL,
    prepared_by BINARY(16) NULL,
    posted_by BINARY(16) NULL,
    posted_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY vouchers_idempotency_key_uq (idempotency_key),
    UNIQUE KEY vouchers_reversal_of_uq (reversal_of),
    UNIQUE KEY vouchers_reversed_by_uq (reversed_by),
    UNIQUE KEY vouchers_branch_number_uq (branch_id, voucher_number),
    KEY vouchers_period_status_idx (period_id, status, posting_date),
    KEY vouchers_voucher_type_idx (voucher_type_id),
    KEY vouchers_currency_idx (currency_code),
    KEY vouchers_source_sales_idx (source_sales_id),
    KEY vouchers_source_payment_idx (source_payment_id),
    KEY vouchers_source_after_sales_idx (source_after_sales_id),
    KEY vouchers_prepared_by_idx (prepared_by),
    KEY vouchers_posted_by_idx (posted_by),
    CONSTRAINT vouchers_status CHECK (status IN ('draft','approved','posted','reversed')),
    CONSTRAINT vouchers_idempotency_key_nonempty CHECK (CHAR_LENGTH(TRIM(idempotency_key)) > 0),
    CONSTRAINT vouchers_payload_hash_length CHECK (OCTET_LENGTH(payload_sha256) = 32),
    CONSTRAINT vouchers_single_source CHECK (
        (source_sales_id IS NOT NULL) + (source_payment_id IS NOT NULL) + (source_after_sales_id IS NOT NULL) <= 1
    ),
    CONSTRAINT vouchers_not_own_reversal CHECK (
        (reversal_of IS NULL OR reversal_of <> id) AND (reversed_by IS NULL OR reversed_by <> id)
    ),
    CONSTRAINT vouchers_posted_metadata CHECK (
        status NOT IN ('posted','reversed') OR (posted_by IS NOT NULL AND posted_at IS NOT NULL)
    ),
    CONSTRAINT vouchers_reversal_reason CHECK (
        status <> 'reversed' OR (reversed_by IS NOT NULL AND CHAR_LENGTH(TRIM(reversal_reason)) > 0)
    ),
    CONSTRAINT vouchers_branch_fk FOREIGN KEY (branch_id) REFERENCES branches(id),
    CONSTRAINT vouchers_voucher_type_fk FOREIGN KEY (voucher_type_id) REFERENCES voucher_types(id),
    CONSTRAINT vouchers_period_fk FOREIGN KEY (period_id) REFERENCES accounting_periods(id),
    CONSTRAINT vouchers_currency_fk FOREIGN KEY (currency_code) REFERENCES currencies(code),
    CONSTRAINT vouchers_source_sales_fk FOREIGN KEY (source_sales_id) REFERENCES sales_documents(id),
    CONSTRAINT vouchers_source_payment_fk FOREIGN KEY (source_payment_id) REFERENCES payments(id),
    CONSTRAINT vouchers_source_after_sales_fk FOREIGN KEY (source_after_sales_id) REFERENCES after_sales(id),
    CONSTRAINT vouchers_reversal_of_fk FOREIGN KEY (reversal_of) REFERENCES vouchers(id),
    CONSTRAINT vouchers_reversed_by_fk FOREIGN KEY (reversed_by) REFERENCES vouchers(id),
    CONSTRAINT vouchers_prepared_by_fk FOREIGN KEY (prepared_by) REFERENCES users(id),
    CONSTRAINT vouchers_posted_by_fk FOREIGN KEY (posted_by) REFERENCES users(id)
) ENGINE=InnoDB;

CREATE TABLE voucher_lines (
    id BINARY(16) NOT NULL,
    voucher_id BINARY(16) NOT NULL,
    line_number INT NOT NULL,
    account_id BINARY(16) NOT NULL,
    party_id BINARY(16) NULL,
    cost_centre_id BINARY(16) NULL,
    debit DECIMAL(65,30) NOT NULL DEFAULT 0,
    credit DECIMAL(65,30) NOT NULL DEFAULT 0,
    foreign_currency VARCHAR(3) NULL,
    foreign_amount DECIMAL(65,30) NULL,
    exchange_rate DECIMAL(65,30) NULL,
    narration TEXT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY voucher_lines_voucher_number_uq (voucher_id, line_number),
    KEY voucher_lines_account_idx (account_id, voucher_id),
    KEY voucher_lines_party_idx (party_id),
    KEY voucher_lines_cost_centre_idx (cost_centre_id),
    KEY voucher_lines_foreign_currency_idx (foreign_currency),
    CONSTRAINT voucher_lines_line_positive CHECK (line_number > 0),
    CONSTRAINT voucher_lines_amount_precision CHECK (
        ABS(debit) < 1000000000000000000 AND debit = TRUNCATE(debit, 6)
        AND ABS(credit) < 1000000000000000000 AND credit = TRUNCATE(credit, 6)
        AND (foreign_amount IS NULL OR
            (ABS(foreign_amount) < 1000000000000000000 AND foreign_amount = TRUNCATE(foreign_amount, 6)))
        AND (exchange_rate IS NULL OR (exchange_rate > 0 AND exchange_rate = TRUNCATE(exchange_rate, 12)))
    ),
    CONSTRAINT voucher_lines_amounts_nonnegative CHECK (debit >= 0 AND credit >= 0),
    CONSTRAINT voucher_lines_one_sided CHECK ((debit > 0 AND credit = 0) OR (credit > 0 AND debit = 0)),
    CONSTRAINT voucher_lines_foreign_fields CHECK (
        (foreign_currency IS NULL AND foreign_amount IS NULL AND exchange_rate IS NULL)
        OR (foreign_currency IS NOT NULL AND foreign_amount IS NOT NULL AND exchange_rate IS NOT NULL)
    ),
    CONSTRAINT voucher_lines_voucher_fk FOREIGN KEY (voucher_id) REFERENCES vouchers(id),
    CONSTRAINT voucher_lines_account_fk FOREIGN KEY (account_id) REFERENCES accounts(id),
    CONSTRAINT voucher_lines_party_fk FOREIGN KEY (party_id) REFERENCES parties(id),
    CONSTRAINT voucher_lines_cost_centre_fk FOREIGN KEY (cost_centre_id) REFERENCES cost_centres(id),
    CONSTRAINT voucher_lines_foreign_currency_fk FOREIGN KEY (foreign_currency) REFERENCES currencies(code)
) ENGINE=InnoDB;

CREATE TABLE invoices (
    id BINARY(16) NOT NULL,
    sales_document_id BINARY(16) NOT NULL,
    invoice_number VARCHAR(100) NOT NULL,
    invoice_date DATE NOT NULL,
    idempotency_key VARCHAR(191) NOT NULL,
    created_by BINARY(16) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY invoices_sales_document_uq (sales_document_id),
    UNIQUE KEY invoices_invoice_number_uq (invoice_number),
    UNIQUE KEY invoices_idempotency_key_uq (idempotency_key),
    KEY invoices_created_by_idx (created_by),
    CONSTRAINT invoices_sales_document_fk FOREIGN KEY (sales_document_id) REFERENCES sales_documents(id),
    CONSTRAINT invoices_created_by_fk FOREIGN KEY (created_by) REFERENCES users(id)
) ENGINE=InnoDB;

CREATE TABLE allocations (
    id BINARY(16) NOT NULL,
    payment_line_id BINARY(16) NOT NULL,
    invoice_id BINARY(16) NULL,
    supplier_line_id BINARY(16) NULL,
    side VARCHAR(16) NOT NULL,
    amount DECIMAL(65,30) NOT NULL,
    reversal_of BINARY(16) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY allocations_reversal_of_uq (reversal_of),
    KEY allocations_payment_line_idx (payment_line_id),
    KEY allocations_invoice_idx (invoice_id),
    KEY allocations_supplier_line_idx (supplier_line_id),
    CONSTRAINT allocations_side CHECK (side IN ('client','supplier')),
    CONSTRAINT allocations_amount_precision CHECK (
        ABS(amount) < 1000000000000000000 AND amount = TRUNCATE(amount, 6)
    ),
    CONSTRAINT allocations_amount_positive CHECK (amount > 0),
    CONSTRAINT allocations_exactly_one_target CHECK (
        (invoice_id IS NOT NULL) + (supplier_line_id IS NOT NULL) = 1
    ),
    CONSTRAINT allocations_payment_line_fk FOREIGN KEY (payment_line_id) REFERENCES payment_lines(id),
    CONSTRAINT allocations_invoice_fk FOREIGN KEY (invoice_id) REFERENCES invoices(id),
    CONSTRAINT allocations_supplier_line_fk FOREIGN KEY (supplier_line_id) REFERENCES sales_lines(id),
    CONSTRAINT allocations_reversal_of_fk FOREIGN KEY (reversal_of) REFERENCES allocations(id)
) ENGINE=InnoDB;

CREATE TABLE bank_statement_imports (
    id BINARY(16) NOT NULL,
    bank_cash_account_id BINARY(16) NOT NULL,
    sha256 CHAR(64) NOT NULL,
    original_name VARCHAR(255) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY bank_statement_imports_account_hash_uq (bank_cash_account_id, sha256),
    CONSTRAINT bank_statement_imports_sha256_format CHECK (REGEXP_LIKE(sha256, '^[a-f0-9]{64}$', 'c')),
    CONSTRAINT bank_statement_imports_account_fk FOREIGN KEY (bank_cash_account_id) REFERENCES bank_cash_accounts(id)
) ENGINE=InnoDB;

CREATE TABLE bank_statement_lines (
    id BINARY(16) NOT NULL,
    import_id BINARY(16) NOT NULL,
    sequence INT NOT NULL,
    transaction_date DATE NOT NULL,
    signed_amount DECIMAL(65,30) NOT NULL,
    reference VARCHAR(191) NULL,
    raw_row JSON NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY bank_statement_lines_import_sequence_uq (import_id, sequence),
    CONSTRAINT bank_statement_lines_sequence_positive CHECK (sequence > 0),
    CONSTRAINT bank_statement_lines_amount_precision CHECK (
        ABS(signed_amount) < 1000000000000000000 AND signed_amount = TRUNCATE(signed_amount, 6)
    ),
    CONSTRAINT bank_statement_lines_import_fk FOREIGN KEY (import_id) REFERENCES bank_statement_imports(id)
) ENGINE=InnoDB;

CREATE TABLE bank_matches (
    id BINARY(16) NOT NULL,
    statement_line_id BINARY(16) NOT NULL,
    voucher_line_id BINARY(16) NOT NULL,
    matched_by BINARY(16) NOT NULL,
    idempotency_key VARCHAR(191) NOT NULL,
    reversed_match_id BINARY(16) NULL,
    reason VARCHAR(1000) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY bank_matches_idempotency_key_uq (idempotency_key),
    UNIQUE KEY bank_matches_reversed_match_uq (reversed_match_id),
    KEY bank_match_pair_idx (statement_line_id, voucher_line_id),
    KEY bank_matches_voucher_line_idx (voucher_line_id),
    KEY bank_matches_matched_by_idx (matched_by),
    CONSTRAINT bank_matches_statement_line_fk FOREIGN KEY (statement_line_id) REFERENCES bank_statement_lines(id),
    CONSTRAINT bank_matches_voucher_line_fk FOREIGN KEY (voucher_line_id) REFERENCES voucher_lines(id),
    CONSTRAINT bank_matches_matched_by_fk FOREIGN KEY (matched_by) REFERENCES users(id),
    CONSTRAINT bank_matches_reversed_match_fk FOREIGN KEY (reversed_match_id) REFERENCES bank_matches(id)
) ENGINE=InnoDB;

CREATE TABLE audit_log (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    occurred_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    actor_id BINARY(16) NULL,
    database_actor VARCHAR(255) NOT NULL,
    request_id BINARY(16) NULL,
    event VARCHAR(32) NOT NULL,
    entity_table VARCHAR(191) NOT NULL,
    entity_key VARCHAR(1024) NOT NULL,
    old_values JSON NULL,
    new_values JSON NULL,
    reason TEXT NULL,
    PRIMARY KEY (id),
    KEY audit_entity_time_idx (entity_table, entity_key(191), occurred_at),
    KEY audit_actor_time_idx (actor_id, occurred_at),
    CONSTRAINT audit_log_actor_fk FOREIGN KEY (actor_id) REFERENCES users(id)
) ENGINE=InnoDB;
