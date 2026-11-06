CREATE TABLE reconciliation_statements (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
 file_name VARCHAR(255) NOT NULL,
 file_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL UNIQUE,
 statement_kind VARCHAR(16) NOT NULL,
 source_bytes MEDIUMBLOB NOT NULL,
 row_count INT UNSIGNED NOT NULL,
 uploaded_by VARCHAR(64) NOT NULL,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 CONSTRAINT reconciliation_statement_kind CHECK (statement_kind IN ('bank','supplier'))
) ENGINE=InnoDB;

CREATE TABLE reconciliation_statement_rows (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
 statement_id BIGINT UNSIGNED NOT NULL,
 line_number INT UNSIGNED NOT NULL,
 source_section VARCHAR(191) NOT NULL,
 source_row INT UNSIGNED NOT NULL,
 cells JSON NOT NULL,
 UNIQUE KEY reconciliation_source_row (statement_id, line_number),
 CONSTRAINT reconciliation_row_statement FOREIGN KEY (statement_id) REFERENCES reconciliation_statements(id)
) ENGINE=InnoDB;
