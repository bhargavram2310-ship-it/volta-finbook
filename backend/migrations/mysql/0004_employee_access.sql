-- MySQL 8.4 port of the table, index, constraint and relationship DDL in
-- migrations/0004_employee_access.sql. Apply after 0001_foundation.sql.
-- No data backfill or default user/employee/role records are inserted.

ALTER TABLE employees
    ADD COLUMN official_email VARCHAR(254) NULL,
    ADD COLUMN mobile_number VARCHAR(64) NULL,
    ADD COLUMN designation VARCHAR(100) NULL,
    ADD COLUMN department VARCHAR(100) NULL,
    ADD COLUMN reporting_to BINARY(16) NULL,
    ADD COLUMN date_of_joining DATE NULL,
    ADD COLUMN employment_type VARCHAR(32) NULL,
    ADD COLUMN employment_status VARCHAR(16) NOT NULL DEFAULT 'Active',
    ADD COLUMN date_of_confirmation DATE NULL,
    ADD COLUMN date_of_leaving DATE NULL,
    ADD COLUMN date_of_birth DATE NULL,
    ADD COLUMN gender VARCHAR(64) NULL,
    ADD COLUMN personal_email VARCHAR(254) NULL,
    ADD COLUMN personal_mobile_number VARCHAR(64) NULL,
    ADD COLUMN alternate_contact_number VARCHAR(64) NULL,
    ADD COLUMN address_line_1 VARCHAR(255) NULL,
    ADD COLUMN address_line_2 VARCHAR(255) NULL,
    ADD COLUMN city VARCHAR(100) NULL,
    ADD COLUMN state VARCHAR(100) NULL,
    ADD COLUMN country VARCHAR(100) NULL,
    ADD COLUMN postal_code VARCHAR(32) NULL,
    ADD COLUMN emergency_contact_name VARCHAR(191) NULL,
    ADD COLUMN emergency_contact_number VARCHAR(64) NULL,
    ADD COLUMN emergency_contact_relationship VARCHAR(100) NULL,
    ADD COLUMN blood_group VARCHAR(16) NULL,
    ADD COLUMN photo_bytes MEDIUMBLOB NULL,
    ADD COLUMN photo_content_type VARCHAR(64) NULL,
    ADD COLUMN remarks TEXT NULL,
    ADD COLUMN version INT NOT NULL DEFAULT 1,
    ADD COLUMN created_by BINARY(16) NULL,
    ADD COLUMN updated_by BINARY(16) NULL,
    ADD COLUMN official_email_key VARCHAR(254)
        GENERATED ALWAYS AS (IF(official_email IS NULL, NULL, LOWER(official_email))) STORED,
    ADD UNIQUE KEY employees_official_email_uq (official_email_key),
    ADD KEY employees_department_designation_idx (department, designation, employment_status),
    ADD KEY employees_reporting_to_idx (reporting_to),
    ADD KEY employees_created_by_idx (created_by),
    ADD KEY employees_updated_by_idx (updated_by),
    ADD CONSTRAINT employees_employment_type CHECK (
        employment_type IS NULL OR employment_type IN ('Permanent','Contract','Temporary','Intern')
    ),
    ADD CONSTRAINT employees_employment_status CHECK (
        employment_status IN ('Active','Inactive','Resigned')
    ),
    ADD CONSTRAINT employees_photo_size CHECK (photo_bytes IS NULL OR OCTET_LENGTH(photo_bytes) <= 2097152),
    ADD CONSTRAINT employees_photo_content_type CHECK (
        photo_content_type IS NULL OR photo_content_type IN ('image/jpeg','image/png')
    ),
    ADD CONSTRAINT employee_no_self_reporting CHECK (reporting_to IS NULL OR reporting_to <> id),
    ADD CONSTRAINT employee_leaving_after_joining CHECK (
        date_of_leaving IS NULL OR date_of_joining IS NULL OR date_of_leaving >= date_of_joining
    ),
    ADD CONSTRAINT employee_official_email_format CHECK (
        official_email IS NULL OR REGEXP_LIKE(official_email, '^[^[:space:]@]+@[^[:space:]@]+[.][^[:space:]@]+$', 'c')
    ),
    ADD CONSTRAINT employee_personal_email_format CHECK (
        personal_email IS NULL OR REGEXP_LIKE(personal_email, '^[^[:space:]@]+@[^[:space:]@]+[.][^[:space:]@]+$', 'c')
    ),
    ADD CONSTRAINT employee_photo_metadata CHECK (
        (photo_bytes IS NULL AND photo_content_type IS NULL)
        OR (photo_bytes IS NOT NULL AND photo_content_type IS NOT NULL)
    ),
    ADD CONSTRAINT employees_active_matches_status CHECK (active = (employment_status = 'Active')),
    ADD CONSTRAINT employees_reporting_to_fk FOREIGN KEY (reporting_to) REFERENCES employees(id),
    ADD CONSTRAINT employees_created_by_fk FOREIGN KEY (created_by) REFERENCES users(id),
    ADD CONSTRAINT employees_updated_by_fk FOREIGN KEY (updated_by) REFERENCES users(id);

ALTER TABLE users
    ADD COLUMN employee_id BINARY(16) NOT NULL,
    ADD COLUMN login_email VARCHAR(254) NULL,
    ADD COLUMN account_status VARCHAR(16) NOT NULL DEFAULT 'Active',
    ADD COLUMN password_reset_required BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN last_login_at DATETIME(6) NULL,
    ADD COLUMN failed_login_count INT NOT NULL DEFAULT 0,
    ADD COLUMN login_blocked_until DATETIME(6) NULL,
    ADD COLUMN created_by BINARY(16) NULL,
    ADD COLUMN updated_by BINARY(16) NULL,
    ADD COLUMN version INT NOT NULL DEFAULT 1,
    ADD COLUMN login_email_key VARCHAR(254)
        GENERATED ALWAYS AS (IF(login_email IS NULL, NULL, LOWER(login_email))) STORED,
    ADD UNIQUE KEY users_employee_id_uq (employee_id),
    ADD UNIQUE KEY users_login_email_uq (login_email_key),
    ADD KEY users_created_by_idx (created_by),
    ADD KEY users_updated_by_idx (updated_by),
    ADD CONSTRAINT users_account_status CHECK (account_status IN ('Active','Inactive','Locked')),
    ADD CONSTRAINT users_failed_login_count_nonnegative CHECK (failed_login_count >= 0),
    ADD CONSTRAINT users_version_positive CHECK (version > 0),
    ADD CONSTRAINT users_login_email_format CHECK (
        login_email IS NULL OR REGEXP_LIKE(login_email, '^[^[:space:]@]+@[^[:space:]@]+[.][^[:space:]@]+$', 'c')
    ),
    ADD CONSTRAINT users_active_matches_status CHECK (active = (account_status = 'Active')),
    ADD CONSTRAINT users_employee_fk FOREIGN KEY (employee_id) REFERENCES employees(id),
    ADD CONSTRAINT users_created_by_fk FOREIGN KEY (created_by) REFERENCES users(id),
    ADD CONSTRAINT users_updated_by_fk FOREIGN KEY (updated_by) REFERENCES users(id);

ALTER TABLE roles
    ADD COLUMN description TEXT NULL,
    ADD COLUMN created_by BINARY(16) NULL,
    ADD COLUMN updated_by BINARY(16) NULL,
    ADD COLUMN version INT NOT NULL DEFAULT 1,
    ADD KEY roles_created_by_idx (created_by),
    ADD KEY roles_updated_by_idx (updated_by),
    ADD CONSTRAINT roles_version_positive CHECK (version > 0),
    ADD CONSTRAINT roles_created_by_fk FOREIGN KEY (created_by) REFERENCES users(id),
    ADD CONSTRAINT roles_updated_by_fk FOREIGN KEY (updated_by) REFERENCES users(id);

ALTER TABLE permissions
    ADD COLUMN created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    ADD COLUMN updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6);

ALTER TABLE user_roles
    ADD COLUMN created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    ADD COLUMN created_by BINARY(16) NULL,
    ADD KEY user_roles_created_by_idx (created_by),
    ADD CONSTRAINT user_roles_created_by_fk FOREIGN KEY (created_by) REFERENCES users(id);

ALTER TABLE role_permissions
    ADD COLUMN created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    ADD COLUMN created_by BINARY(16) NULL,
    ADD KEY role_permissions_created_by_idx (created_by),
    ADD CONSTRAINT role_permissions_created_by_fk FOREIGN KEY (created_by) REFERENCES users(id);

CREATE TABLE role_history (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id BINARY(16) NOT NULL,
    employee_id BINARY(16) NOT NULL,
    previous_role_id BINARY(16) NULL,
    new_role_id BINARY(16) NULL,
    previous_role_name VARCHAR(191) NULL,
    new_role_name VARCHAR(191) NULL,
    action VARCHAR(16) NOT NULL,
    modified_by BINARY(16) NOT NULL,
    modified_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    remarks VARCHAR(1000) NOT NULL,
    PRIMARY KEY (id),
    KEY role_history_user_time_idx (user_id, modified_at DESC),
    KEY role_history_employee_time_idx (employee_id, modified_at DESC),
    KEY role_history_old_role_idx (previous_role_id, modified_at DESC),
    KEY role_history_new_role_idx (new_role_id, modified_at DESC),
    KEY role_history_modified_by_idx (modified_by),
    CONSTRAINT role_history_action CHECK (action IN ('Assigned','Modified','Removed')),
    CONSTRAINT role_history_remarks_nonempty CHECK (CHAR_LENGTH(TRIM(remarks)) > 0),
    CONSTRAINT role_history_action_roles CHECK (
        (action = 'Assigned' AND previous_role_id IS NULL AND new_role_id IS NOT NULL)
        OR (action = 'Removed' AND previous_role_id IS NOT NULL AND new_role_id IS NULL)
        OR (action = 'Modified' AND previous_role_id IS NOT NULL AND new_role_id IS NOT NULL)
    ),
    CONSTRAINT role_history_user_fk FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT role_history_employee_fk FOREIGN KEY (employee_id) REFERENCES employees(id),
    CONSTRAINT role_history_previous_role_fk FOREIGN KEY (previous_role_id) REFERENCES roles(id),
    CONSTRAINT role_history_new_role_fk FOREIGN KEY (new_role_id) REFERENCES roles(id),
    CONSTRAINT role_history_modified_by_fk FOREIGN KEY (modified_by) REFERENCES users(id)
) ENGINE=InnoDB;

CREATE TABLE employee_change_requests (
    id BINARY(16) NOT NULL,
    employee_id BINARY(16) NOT NULL,
    requested_by BINARY(16) NOT NULL,
    employee_version INT NOT NULL,
    original_values JSON NOT NULL,
    requested_values JSON NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'Pending Approval',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    decided_by BINARY(16) NULL,
    decided_at DATETIME(6) NULL,
    decision_remarks VARCHAR(1000) NULL,
    PRIMARY KEY (id),
    KEY employee_change_requests_employee_idx (employee_id, created_at DESC),
    KEY employee_change_requests_status_idx (status, created_at DESC),
    KEY employee_change_requests_requested_by_idx (requested_by),
    KEY employee_change_requests_decided_by_idx (decided_by),
    CONSTRAINT employee_change_requests_version_positive CHECK (employee_version > 0),
    CONSTRAINT employee_change_requests_original_object CHECK (JSON_TYPE(original_values) = 'OBJECT'),
    CONSTRAINT employee_change_requests_requested_object CHECK (
        JSON_TYPE(requested_values) = 'OBJECT' AND JSON_LENGTH(requested_values) > 0
    ),
    CONSTRAINT employee_change_requests_status CHECK (status IN ('Pending Approval','Approved','Rejected')),
    CONSTRAINT employee_change_requests_decision_state CHECK (
        (status = 'Pending Approval' AND decided_by IS NULL AND decided_at IS NULL)
        OR (
            status IN ('Approved','Rejected') AND decided_by IS NOT NULL AND decided_at IS NOT NULL
            AND decision_remarks IS NOT NULL AND CHAR_LENGTH(TRIM(decision_remarks)) > 0
        )
    ),
    CONSTRAINT employee_change_requests_employee_fk FOREIGN KEY (employee_id) REFERENCES employees(id),
    CONSTRAINT employee_change_requests_requested_by_fk FOREIGN KEY (requested_by) REFERENCES users(id),
    CONSTRAINT employee_change_requests_decided_by_fk FOREIGN KEY (decided_by) REFERENCES users(id)
) ENGINE=InnoDB;

CREATE TABLE password_reset_tokens (
    id BINARY(16) NOT NULL,
    user_id BINARY(16) NOT NULL,
    token_hash BINARY(32) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    used_at DATETIME(6) NULL,
    created_by BINARY(16) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY password_reset_tokens_hash_uq (token_hash),
    KEY password_reset_user_idx (user_id, expires_at),
    KEY password_reset_created_by_idx (created_by),
    CONSTRAINT password_reset_tokens_hash_length CHECK (OCTET_LENGTH(token_hash) = 32),
    CONSTRAINT password_reset_tokens_expiry CHECK (expires_at > created_at),
    CONSTRAINT password_reset_tokens_user_fk FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT password_reset_tokens_created_by_fk FOREIGN KEY (created_by) REFERENCES users(id)
) ENGINE=InnoDB;
