-- Additive MIR staging only. No sales, voucher or accounting table is changed.
CREATE TABLE mir_import_batch (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
 status VARCHAR(32) NOT NULL DEFAULT 'processing',
 imported_count INT NOT NULL DEFAULT 0, duplicate_count INT NOT NULL DEFAULT 0, failed_count INT NOT NULL DEFAULT 0,
 errors JSON NOT NULL, created_by BINARY(16) NULL,
 created_at DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)), completed_at DATETIME(6) NULL,
 FOREIGN KEY(created_by) REFERENCES users(id), INDEX mir_batch_created(created_at)
) ENGINE=InnoDB;
CREATE TABLE mir_source_file (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, batch_id BIGINT NOT NULL,
 filename VARCHAR(255) NOT NULL, sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 original_bytes LONGBLOB NOT NULL, byte_size INT NOT NULL,
 created_at DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
 UNIQUE KEY mir_source_checksum(sha256), INDEX mir_source_filename(filename),
 FOREIGN KEY(batch_id) REFERENCES mir_import_batch(id)
) ENGINE=InnoDB;
CREATE TABLE mir_record (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, source_file_id BIGINT NOT NULL, record_number INT NOT NULL,
 mir_date DATE NULL, first_travel_date DATE NULL, locator VARCHAR(64) NOT NULL DEFAULT '', mir_type VARCHAR(32) NOT NULL DEFAULT '',
 passenger VARCHAR(255) NOT NULL DEFAULT '', ticket_number VARCHAR(64) NOT NULL DEFAULT '', currency VARCHAR(8) NOT NULL DEFAULT '', fare_amount VARCHAR(64) NOT NULL DEFAULT '',
 status VARCHAR(32) NOT NULL, parser_version VARCHAR(64) NOT NULL, raw_record LONGBLOB NOT NULL, decoded_header JSON NOT NULL, validation_issues JSON NOT NULL,
 imported_at DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)), reprocessed_at DATETIME(6) NULL,
 UNIQUE KEY mir_record_source_number(source_file_id,record_number), INDEX mir_record_date(mir_date,id), INDEX mir_record_locator(locator), INDEX mir_record_status(status,mir_date,id), INDEX mir_record_ticket(ticket_number), INDEX mir_record_passenger(passenger),
 FOREIGN KEY(source_file_id) REFERENCES mir_source_file(id)
) ENGINE=InnoDB;
CREATE TABLE mir_batch_record_error (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, batch_id BIGINT NOT NULL, source_file_id BIGINT NOT NULL, record_number INT NOT NULL,
 error_code VARCHAR(64) NOT NULL, created_at DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
 FOREIGN KEY(batch_id) REFERENCES mir_import_batch(id), FOREIGN KEY(source_file_id) REFERENCES mir_source_file(id)
) ENGINE=InnoDB;
CREATE TABLE mir_section (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, identifier VARCHAR(16) NOT NULL, occurrence INT NOT NULL,
 decoder_status VARCHAR(32) NOT NULL, decoder_reason TEXT NOT NULL, raw_data LONGBLOB NOT NULL,
 UNIQUE KEY mir_section_identity(record_id,identifier,occurrence), UNIQUE KEY mir_section_owner(id,record_id),
 FOREIGN KEY(record_id) REFERENCES mir_record(id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_section_item (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, section_id BIGINT NOT NULL, record_id BIGINT NOT NULL, item_number INT NOT NULL, raw_data LONGBLOB NOT NULL,
 UNIQUE KEY mir_item_identity(section_id,item_number), UNIQUE KEY mir_item_owner(id,section_id,record_id),
 FOREIGN KEY(section_id,record_id) REFERENCES mir_section(id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_decoded_field (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, item_id BIGINT NOT NULL, label VARCHAR(64) NOT NULL, field_name VARCHAR(255) NOT NULL,
 data_type VARCHAR(16) NOT NULL, raw_value LONGBLOB NOT NULL, decoded_value JSON NOT NULL,
 byte_start INT NOT NULL, byte_end INT NOT NULL, status VARCHAR(32) NOT NULL, guide_reference VARCHAR(255) NOT NULL, note TEXT NOT NULL,
 FOREIGN KEY(item_id) REFERENCES mir_section_item(id) ON DELETE CASCADE, INDEX mir_field_label(item_id,label)
) ENGINE=InnoDB;
CREATE TABLE mir_item_relationship (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL,
 from_item_id BIGINT NOT NULL, from_section_id BIGINT NOT NULL, to_item_id BIGINT NOT NULL, to_section_id BIGINT NOT NULL,
 relationship_type VARCHAR(64) NOT NULL, source_label VARCHAR(64) NOT NULL,
 UNIQUE KEY mir_link_identity(from_item_id,to_item_id,relationship_type),
 FOREIGN KEY(from_item_id,from_section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE,
 FOREIGN KEY(to_item_id,to_section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_fare_tax (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 source_label VARCHAR(64) NOT NULL, tax_code VARCHAR(16) NOT NULL, amount VARCHAR(64) NOT NULL, currency VARCHAR(8) NOT NULL,
 UNIQUE KEY mir_tax_source(source_item_id,source_label),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_passenger (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 name VARCHAR(255) NOT NULL DEFAULT '',
 ticket_number VARCHAR(255) NOT NULL DEFAULT '',
 passenger_ref VARCHAR(255) NOT NULL DEFAULT '',
 fare_ref VARCHAR(255) NOT NULL DEFAULT '',
 decoded_data JSON NOT NULL, mapping_status VARCHAR(32) NOT NULL,
 UNIQUE KEY mir_passenger_source(source_item_id), INDEX mir_passenger_record(record_id), INDEX mir_passenger_name(name,record_id), INDEX mir_passenger_ticket(ticket_number,record_id),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_frequent_flyer (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 name VARCHAR(255) NOT NULL DEFAULT '',
 carrier VARCHAR(255) NOT NULL DEFAULT '',
 membership_number VARCHAR(255) NOT NULL DEFAULT '',
 decoded_data JSON NOT NULL, mapping_status VARCHAR(32) NOT NULL,
 UNIQUE KEY mir_frequent_flyer_source(source_item_id), INDEX mir_frequent_flyer_record(record_id),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_segment (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 segment_ref VARCHAR(255) NOT NULL DEFAULT '',
 carrier VARCHAR(255) NOT NULL DEFAULT '',
 origin VARCHAR(255) NOT NULL DEFAULT '',
 destination VARCHAR(255) NOT NULL DEFAULT '',
 departure_date VARCHAR(255) NOT NULL DEFAULT '',
 decoded_data JSON NOT NULL, mapping_status VARCHAR(32) NOT NULL,
 UNIQUE KEY mir_segment_source(source_item_id), INDEX mir_segment_record(record_id),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_fare (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 fare_ref VARCHAR(255) NOT NULL DEFAULT '',
 currency VARCHAR(255) NOT NULL DEFAULT '',
 base_amount VARCHAR(255) NOT NULL DEFAULT '',
 total_amount VARCHAR(255) NOT NULL DEFAULT '',
 decoded_data JSON NOT NULL, mapping_status VARCHAR(32) NOT NULL,
 UNIQUE KEY mir_fare_source(source_item_id), INDEX mir_fare_record(record_id),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_fare_basis (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 fare_ref VARCHAR(255) NOT NULL DEFAULT '',
 segment_ref VARCHAR(255) NOT NULL DEFAULT '',
 basis_code VARCHAR(255) NOT NULL DEFAULT '',
 decoded_data JSON NOT NULL, mapping_status VARCHAR(32) NOT NULL,
 UNIQUE KEY mir_fare_basis_source(source_item_id), INDEX mir_fare_basis_record(record_id),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_fare_construction (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 fare_ref VARCHAR(255) NOT NULL DEFAULT '',
 construction_text VARCHAR(255) NOT NULL DEFAULT '',
 decoded_data JSON NOT NULL, mapping_status VARCHAR(32) NOT NULL,
 UNIQUE KEY mir_fare_construction_source(source_item_id), INDEX mir_fare_construction_record(record_id),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_payment (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 payment_type VARCHAR(255) NOT NULL DEFAULT '',
 amount VARCHAR(255) NOT NULL DEFAULT '',
 passenger_ref VARCHAR(255) NOT NULL DEFAULT '',
 decoded_data JSON NOT NULL, mapping_status VARCHAR(32) NOT NULL,
 UNIQUE KEY mir_payment_source(source_item_id), INDEX mir_payment_record(record_id),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_ticket_exchange (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 exchange_ref VARCHAR(255) NOT NULL DEFAULT '',
 decoded_data JSON NOT NULL, mapping_status VARCHAR(32) NOT NULL,
 UNIQUE KEY mir_ticket_exchange_source(source_item_id), INDEX mir_ticket_exchange_record(record_id),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_refund (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 ticket_number VARCHAR(255) NOT NULL DEFAULT '', name VARCHAR(255) NOT NULL DEFAULT '', issue_date VARCHAR(255) NOT NULL DEFAULT '',
 decoded_data JSON NOT NULL, mapping_status VARCHAR(32) NOT NULL,
 UNIQUE KEY mir_refund_source(source_item_id), INDEX mir_refund_record(record_id),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_emd (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 document_number VARCHAR(255) NOT NULL DEFAULT '',
 passenger_ref VARCHAR(255) NOT NULL DEFAULT '',
 segment_ref VARCHAR(255) NOT NULL DEFAULT '',
 currency VARCHAR(255) NOT NULL DEFAULT '',
 total_amount VARCHAR(255) NOT NULL DEFAULT '',
 decoded_data JSON NOT NULL, mapping_status VARCHAR(32) NOT NULL,
 UNIQUE KEY mir_emd_source(source_item_id), INDEX mir_emd_record(record_id),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_emd_coupon (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 document_number VARCHAR(255) NOT NULL DEFAULT '',
 coupon_number VARCHAR(255) NOT NULL DEFAULT '',
 passenger_ref VARCHAR(255) NOT NULL DEFAULT '',
 segment_ref VARCHAR(255) NOT NULL DEFAULT '',
 ticket_number VARCHAR(255) NOT NULL DEFAULT '',
 decoded_data JSON NOT NULL, mapping_status VARCHAR(32) NOT NULL,
 UNIQUE KEY mir_emd_coupon_source(source_item_id), INDEX mir_emd_coupon_record(record_id),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_auxiliary_segment (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 decoded_data JSON NOT NULL, mapping_status VARCHAR(32) NOT NULL,
 UNIQUE KEY mir_auxiliary_segment_source(source_item_id), INDEX mir_auxiliary_segment_record(record_id),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_phone (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 city VARCHAR(255) NOT NULL DEFAULT '',
 phone VARCHAR(255) NOT NULL DEFAULT '',
 decoded_data JSON NOT NULL, mapping_status VARCHAR(32) NOT NULL,
 UNIQUE KEY mir_phone_source(source_item_id), INDEX mir_phone_record(record_id),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_address (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 address_text VARCHAR(255) NOT NULL DEFAULT '',
 decoded_data JSON NOT NULL, mapping_status VARCHAR(32) NOT NULL,
 UNIQUE KEY mir_address_source(source_item_id), INDEX mir_address_record(record_id),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_remark (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 remark_text VARCHAR(255) NOT NULL DEFAULT '',
 segment_ref VARCHAR(255) NOT NULL DEFAULT '',
 decoded_data JSON NOT NULL, mapping_status VARCHAR(32) NOT NULL,
 UNIQUE KEY mir_remark_source(source_item_id), INDEX mir_remark_record(record_id),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_ssr_osi (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 segment_ref VARCHAR(255) NOT NULL DEFAULT '', passenger_ref VARCHAR(255) NOT NULL DEFAULT '', service_type VARCHAR(255) NOT NULL DEFAULT '', service_status VARCHAR(255) NOT NULL DEFAULT '', carrier VARCHAR(255) NOT NULL DEFAULT '', service_text VARCHAR(255) NOT NULL DEFAULT '',
 decoded_data JSON NOT NULL, mapping_status VARCHAR(32) NOT NULL,
 UNIQUE KEY mir_ssr_osi_source(source_item_id), INDEX mir_ssr_osi_record(record_id),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_carrier_fee (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 decoded_data JSON NOT NULL, mapping_status VARCHAR(32) NOT NULL,
 UNIQUE KEY mir_carrier_fee_source(source_item_id), INDEX mir_carrier_fee_record(record_id),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE mir_misc_document (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, record_id BIGINT NOT NULL, section_id BIGINT NOT NULL, source_item_id BIGINT NOT NULL,
 decoded_data JSON NOT NULL, mapping_status VARCHAR(32) NOT NULL,
 UNIQUE KEY mir_misc_document_source(source_item_id), INDEX mir_misc_document_record(record_id),
 FOREIGN KEY(source_item_id,section_id,record_id) REFERENCES mir_section_item(id,section_id,record_id) ON DELETE CASCADE
) ENGINE=InnoDB;
