-- No tax rates, roles, commercial amounts, accounts, branches or periods are seeded.
-- All objects are isolated in finbook. Existing public objects are untouched.
-- Unconstrained numeric domains reject excess precision instead of rounding it.
CREATE DOMAIN finbook.money_amount AS numeric CHECK(VALUE IS NULL OR (abs(VALUE)<1000000000000000000 AND scale(VALUE)<=6));
CREATE DOMAIN finbook.exchange_rate_value AS numeric CHECK(VALUE IS NULL OR (VALUE>0 AND scale(VALUE)<=12));
CREATE DOMAIN finbook.rate_percentage AS numeric CHECK(VALUE IS NULL OR (VALUE BETWEEN 0 AND 100 AND scale(VALUE)<=6));
CREATE TYPE finbook.sales_class AS ENUM ('international','domestic','miscellaneous');
CREATE TYPE finbook.document_status AS ENUM ('draft','validation','verification','verified','approval','approved','posted','cancelled','refunded','reversed');
CREATE TYPE finbook.voucher_status AS ENUM ('draft','approved','posted','reversed');
CREATE TYPE finbook.provenance AS ENUM ('user_entered','mir_extracted','system_calculated','user_overridden');
CREATE TYPE finbook.accounting_side AS ENUM ('client','supplier');
CREATE TYPE finbook.period_status AS ENUM ('open','closed');
CREATE TYPE finbook.mir_status AS ENUM ('uploaded','extracted','validation_required','validated','linked','posted','failed','duplicate','reprocessed');

CREATE TABLE finbook.currencies (
 code text PRIMARY KEY CHECK (code ~ '^[A-Z]{3}$'), name text NOT NULL,
 decimal_places smallint NOT NULL CHECK (decimal_places BETWEEN 0 AND 6),
 active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz
);
CREATE TABLE finbook.company (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), singleton boolean NOT NULL DEFAULT true UNIQUE CHECK(singleton),
 legal_name text NOT NULL CHECK(length(trim(legal_name))>0), base_currency text NOT NULL REFERENCES finbook.currencies(code),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE finbook.branches (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), company_id uuid NOT NULL REFERENCES finbook.company(id), code text NOT NULL UNIQUE,
 name text NOT NULL, state_code text, gst_registration text, active boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
 UNIQUE(company_id,id)
);
CREATE TABLE finbook.users (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), username text NOT NULL, display_name text NOT NULL,
 password_hash text NOT NULL, active boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz
);
CREATE UNIQUE INDEX users_username_active_uq ON finbook.users(lower(username)) WHERE deleted_at IS NULL;
CREATE TABLE finbook.roles (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), code text NOT NULL UNIQUE, name text NOT NULL, active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz);
CREATE TABLE finbook.permissions (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), module text NOT NULL, screen text NOT NULL, action text NOT NULL, UNIQUE(module,screen,action));
CREATE TABLE finbook.user_roles (user_id uuid NOT NULL REFERENCES finbook.users(id), role_id uuid NOT NULL REFERENCES finbook.roles(id), PRIMARY KEY(user_id,role_id));
CREATE TABLE finbook.role_permissions (role_id uuid NOT NULL REFERENCES finbook.roles(id), permission_id uuid NOT NULL REFERENCES finbook.permissions(id), PRIMARY KEY(role_id,permission_id));
CREATE TABLE finbook.sessions (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES finbook.users(id), token_hash bytea NOT NULL UNIQUE CHECK(octet_length(token_hash)=32), expires_at timestamptz NOT NULL, revoked_at timestamptz, created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX sessions_user_expiry_idx ON finbook.sessions(user_id,expires_at);

CREATE TABLE finbook.parties (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), company_id uuid NOT NULL REFERENCES finbook.company(id), code text NOT NULL UNIQUE,
 legal_name text NOT NULL, gst_registration text, state_code text, country_code text, email text, phone text,
 active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz
);
-- Separate commercial identities even when a legal organization fills several roles.
CREATE TABLE finbook.customers (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), party_id uuid NOT NULL UNIQUE REFERENCES finbook.parties(id), credit_days integer CHECK(credit_days>=0), credit_limit finbook.money_amount CHECK(credit_limit>=0));
CREATE TABLE finbook.travel_agents (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), party_id uuid NOT NULL UNIQUE REFERENCES finbook.parties(id));
CREATE TABLE finbook.suppliers (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), party_id uuid NOT NULL UNIQUE REFERENCES finbook.parties(id), settlement_channel text CHECK(settlement_channel IN ('airline','bsp','consolidator','other')));
CREATE TABLE finbook.airlines (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), code text NOT NULL UNIQUE, name text NOT NULL, ticket_prefix text, active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz);
CREATE TABLE finbook.locations (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), kind text NOT NULL CHECK(kind IN ('country','city','airport')), code text NOT NULL, name text NOT NULL, parent_id uuid REFERENCES finbook.locations(id), active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz, UNIQUE(kind,code), CHECK(parent_id IS DISTINCT FROM id));
CREATE TABLE finbook.cost_centres (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), code text NOT NULL UNIQUE, name text NOT NULL, active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz);
CREATE TABLE finbook.employees (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), branch_id uuid REFERENCES finbook.branches(id), user_id uuid UNIQUE REFERENCES finbook.users(id), employee_code text NOT NULL UNIQUE, name text NOT NULL, active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz);
COMMENT ON TABLE finbook.employees IS 'ERP employee master, independent of existing public.employees. No automatic import or linkage.';
CREATE TABLE finbook.projects (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), code text NOT NULL UNIQUE, name text NOT NULL, customer_id uuid REFERENCES finbook.customers(id), active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz);
CREATE TABLE finbook.services (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), code text NOT NULL UNIQUE, name text NOT NULL, sales_class finbook.sales_class, active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz);
CREATE TABLE finbook.exchange_rates (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), from_currency text NOT NULL REFERENCES finbook.currencies(code), to_currency text NOT NULL REFERENCES finbook.currencies(code), effective_date date NOT NULL, rate finbook.exchange_rate_value NOT NULL CHECK(rate>0), verified_by uuid REFERENCES finbook.users(id), created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(from_currency,to_currency,effective_date), CHECK(from_currency<>to_currency));
CREATE TABLE finbook.accounts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), code text NOT NULL UNIQUE, name text NOT NULL,
 classification text NOT NULL CHECK(classification IN ('asset','liability','equity','income','expense')),
 parent_id uuid REFERENCES finbook.accounts(id), postable boolean NOT NULL DEFAULT true, active boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz, CHECK(parent_id IS DISTINCT FROM id)
);
CREATE TABLE finbook.bank_cash_accounts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), ledger_account_id uuid NOT NULL UNIQUE REFERENCES finbook.accounts(id),
 kind text NOT NULL CHECK(kind IN ('bank','cash')), currency_code text NOT NULL REFERENCES finbook.currencies(code),
 bank_name text, account_number_ciphertext bytea, masked_last_four text CHECK(masked_last_four ~ '^[0-9]{4}$'),
 active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
 CHECK(kind='bank' OR (bank_name IS NULL AND account_number_ciphertext IS NULL AND masked_last_four IS NULL))
);
CREATE TABLE finbook.financial_years (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), label text NOT NULL UNIQUE, starts_on date NOT NULL, ends_on date NOT NULL,
 closed boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(starts_on<=ends_on), EXCLUDE USING gist(daterange(starts_on,ends_on,'[]') WITH &&)
);
CREATE TABLE finbook.accounting_periods (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), financial_year_id uuid NOT NULL REFERENCES finbook.financial_years(id),
 label text NOT NULL, starts_on date NOT NULL, ends_on date NOT NULL, status finbook.period_status NOT NULL DEFAULT 'open',
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(starts_on<=ends_on), UNIQUE(financial_year_id,label), EXCLUDE USING gist(daterange(starts_on,ends_on,'[]') WITH &&)
);
CREATE TABLE finbook.voucher_types (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), code text NOT NULL UNIQUE, name text NOT NULL, active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz);
CREATE TABLE finbook.numbering_rules (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), branch_id uuid NOT NULL REFERENCES finbook.branches(id), document_kind text NOT NULL,
 financial_year_id uuid NOT NULL REFERENCES finbook.financial_years(id), prefix text NOT NULL, next_number bigint NOT NULL CHECK(next_number>0),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(branch_id,document_kind,financial_year_id)
);
CREATE TABLE finbook.tax_rules (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), code text NOT NULL, version integer NOT NULL CHECK(version>0), component text NOT NULL,
 rate_percent finbook.rate_percentage CHECK(rate_percent BETWEEN 0 AND 100), basis jsonb NOT NULL CHECK(jsonb_typeof(basis)='object'),
 effective_from date NOT NULL, effective_to date, verification_status text NOT NULL DEFAULT 'to_be_verified_by_finance' CHECK(verification_status IN ('to_be_verified_by_finance','verified','rejected')),
 verified_by uuid REFERENCES finbook.users(id), verified_at timestamptz, active boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
 UNIQUE(code,version), CHECK(effective_to IS NULL OR effective_to>=effective_from),
 CHECK(NOT active OR (verification_status='verified' AND verified_by IS NOT NULL AND verified_at IS NOT NULL AND rate_percent IS NOT NULL))
);
COMMENT ON TABLE finbook.tax_rules IS 'All rates, applicability, place of supply, CGST/SGST/IGST, airline tax and service charge treatment: to be verified by finance. No statutory defaults seeded.';
CREATE TABLE finbook.commercial_rules (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), kind text NOT NULL CHECK(kind IN ('commission','discount','markup','cancellation','refund','tds')), code text NOT NULL UNIQUE, configuration jsonb NOT NULL CHECK(jsonb_typeof(configuration)='object'), verification_status text NOT NULL DEFAULT 'to_be_verified_by_finance', active boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz, CHECK(NOT active OR verification_status='verified'));
CREATE TABLE finbook.approval_rules (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), code text NOT NULL UNIQUE, transaction_kind text NOT NULL, currency_code text REFERENCES finbook.currencies(code), minimum_amount finbook.money_amount, maximum_amount finbook.money_amount, required_role_id uuid NOT NULL REFERENCES finbook.roles(id), conditions jsonb NOT NULL DEFAULT '{}', active boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz, CHECK(minimum_amount IS NULL OR maximum_amount IS NULL OR minimum_amount<=maximum_amount));
CREATE TABLE finbook.approval_requests (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), rule_id uuid NOT NULL REFERENCES finbook.approval_rules(id), entity_type text NOT NULL, entity_id uuid NOT NULL, entity_version integer NOT NULL CHECK(entity_version>0), status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected','withdrawn')), requested_by uuid NOT NULL REFERENCES finbook.users(id), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(rule_id,entity_type,entity_id,entity_version));
CREATE TABLE finbook.approval_decisions (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, request_id uuid NOT NULL REFERENCES finbook.approval_requests(id), actor_id uuid NOT NULL REFERENCES finbook.users(id), decision text NOT NULL CHECK(decision IN ('approved','rejected')), reason text NOT NULL CHECK(length(trim(reason))>0), created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(request_id,actor_id));

CREATE TABLE finbook.mir_files (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), sha256 text NOT NULL UNIQUE CHECK(sha256 ~ '^[a-f0-9]{64}$'), original_name text NOT NULL,
 raw_bytes bytea NOT NULL, source_path text, status finbook.mir_status NOT NULL,
 imported_by uuid REFERENCES finbook.users(id), created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(sha256=encode(sha256(raw_bytes),'hex'))
);
CREATE TABLE finbook.mir_source_lines (file_id uuid NOT NULL REFERENCES finbook.mir_files(id), line_number integer NOT NULL CHECK(line_number>0), raw_bytes bytea NOT NULL, terminator_bytes bytea NOT NULL, PRIMARY KEY(file_id,line_number));
CREATE TABLE finbook.mir_parse_runs (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), file_id uuid NOT NULL REFERENCES finbook.mir_files(id), parser_version text NOT NULL, run_key text NOT NULL UNIQUE, status finbook.mir_status NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz, UNIQUE(id,file_id));
CREATE TABLE finbook.mir_records (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), file_id uuid NOT NULL REFERENCES finbook.mir_files(id), parse_run_id uuid NOT NULL, record_key text NOT NULL, passenger_key text NOT NULL, status finbook.mir_status NOT NULL, issues jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(issues)='array'), created_at timestamptz NOT NULL DEFAULT now(), FOREIGN KEY(parse_run_id,file_id) REFERENCES finbook.mir_parse_runs(id,file_id), UNIQUE(parse_run_id,record_key,passenger_key));
CREATE TABLE finbook.mir_fields (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), record_id uuid NOT NULL REFERENCES finbook.mir_records(id), source_field_id text NOT NULL, label text NOT NULL, raw_value text, extracted_value jsonb, line_number integer NOT NULL CHECK(line_number>0), byte_start bigint NOT NULL CHECK(byte_start>=0), byte_end bigint NOT NULL, extraction_status text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), CHECK(byte_end>=byte_start), UNIQUE(record_id,source_field_id));

CREATE TABLE finbook.sales_documents (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), company_id uuid NOT NULL REFERENCES finbook.company(id), branch_id uuid NOT NULL,
 sales_class finbook.sales_class NOT NULL, document_number text, document_date date NOT NULL, due_date date,
 customer_id uuid REFERENCES finbook.customers(id), agent_id uuid REFERENCES finbook.travel_agents(id), cost_centre_id uuid REFERENCES finbook.cost_centres(id),
 employee_id uuid REFERENCES finbook.employees(id), client_employee_code text, project_id uuid REFERENCES finbook.projects(id), tour_code text,
 currency_code text NOT NULL REFERENCES finbook.currencies(code), exchange_rate finbook.exchange_rate_value CHECK(exchange_rate>0),
 status finbook.document_status NOT NULL DEFAULT 'draft', version integer NOT NULL DEFAULT 1 CHECK(version>0),
 reference text, narration text, organization jsonb NOT NULL DEFAULT '{}', classification jsonb NOT NULL DEFAULT '{}',
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
 FOREIGN KEY(company_id,branch_id) REFERENCES finbook.branches(company_id,id), UNIQUE(branch_id,document_number)
);
CREATE INDEX sales_date_class_status_idx ON finbook.sales_documents(document_date,sales_class,status);
CREATE INDEX sales_customer_due_idx ON finbook.sales_documents(customer_id,due_date) WHERE deleted_at IS NULL;
CREATE TABLE finbook.tickets (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), ticket_number text NOT NULL UNIQUE CHECK(length(trim(ticket_number))>0 AND ticket_number=upper(trim(ticket_number))),
 ticket_number_key text GENERATED ALWAYS AS (regexp_replace(upper(ticket_number),'[^A-Z0-9]','','g')) STORED UNIQUE,
 airline_id uuid REFERENCES finbook.airlines(id), pnr text, passenger_name text, passport_ciphertext bytea, nationality text,
 ticket_date date, ticket_date_source text, travel_date date, travel_date_source text, sector text, crs text, fare_basis text,
 status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','issued','reissued','void','cancelled','refunded','no_show','unutilized')),
 original_ticket_id uuid REFERENCES finbook.tickets(id), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
 CHECK(original_ticket_id IS DISTINCT FROM id)
);
CREATE INDEX tickets_pnr_idx ON finbook.tickets(pnr);
CREATE INDEX tickets_passenger_idx ON finbook.tickets(lower(passenger_name));
CREATE TABLE finbook.sales_lines (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), document_id uuid NOT NULL REFERENCES finbook.sales_documents(id), line_number integer NOT NULL CHECK(line_number>0),
 ticket_id uuid REFERENCES finbook.tickets(id), mir_record_id uuid REFERENCES finbook.mir_records(id), supplier_id uuid REFERENCES finbook.suppliers(id),
 service_id uuid REFERENCES finbook.services(id), service_category text NOT NULL, description text, service_from date, service_to date, quantity numeric(18,6) CHECK(quantity>0),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
 UNIQUE(document_id,line_number), UNIQUE(document_id,ticket_id), UNIQUE(mir_record_id)
);
CREATE UNIQUE INDEX ticket_original_sale_uq ON finbook.sales_lines(ticket_id) WHERE ticket_id IS NOT NULL;
CREATE TABLE finbook.ticket_segments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), ticket_id uuid NOT NULL REFERENCES finbook.tickets(id), sequence integer NOT NULL CHECK(sequence>0),
 origin_id uuid REFERENCES finbook.locations(id), destination_id uuid REFERENCES finbook.locations(id), flight_number text,
 departure_at timestamptz, arrival_at timestamptz, departure_source text, arrival_source text,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(ticket_id,sequence)
);
CREATE TABLE finbook.accounting_components (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), code text NOT NULL UNIQUE, label text NOT NULL, effect text NOT NULL CHECK(effect IN ('add','subtract')), category text NOT NULL, active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz);
CREATE TABLE finbook.line_accounting (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), sales_line_id uuid NOT NULL REFERENCES finbook.sales_lines(id), side finbook.accounting_side NOT NULL,
 currency_code text NOT NULL REFERENCES finbook.currencies(code), total finbook.money_amount, provenance finbook.provenance NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(sales_line_id,side)
);
CREATE TABLE finbook.line_accounting_components (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), accounting_id uuid NOT NULL REFERENCES finbook.line_accounting(id), component_id uuid NOT NULL REFERENCES finbook.accounting_components(id),
 amount finbook.money_amount NOT NULL CHECK(amount>=0), tax_rule_id uuid REFERENCES finbook.tax_rules(id), source_field_id uuid REFERENCES finbook.mir_fields(id), provenance finbook.provenance NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(accounting_id,component_id),
 CHECK(provenance<>'mir_extracted' OR source_field_id IS NOT NULL)
);
CREATE TABLE finbook.field_corrections (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, sales_line_id uuid REFERENCES finbook.sales_lines(id), mir_field_id uuid REFERENCES finbook.mir_fields(id),
 field_name text NOT NULL, old_value jsonb, new_value jsonb, reason text NOT NULL CHECK(length(trim(reason))>0),
 actor_id uuid NOT NULL REFERENCES finbook.users(id), created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(sales_line_id IS NOT NULL OR mir_field_id IS NOT NULL)
);
CREATE TABLE finbook.value_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, entity_type text NOT NULL, entity_id uuid NOT NULL, field_name text NOT NULL,
 value jsonb, provenance finbook.provenance NOT NULL, source_field_id uuid REFERENCES finbook.mir_fields(id),
 calculation_version text, previous_event_id bigint UNIQUE REFERENCES finbook.value_events(id),
 actor_id uuid REFERENCES finbook.users(id), reason text, created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(provenance<>'mir_extracted' OR source_field_id IS NOT NULL),
 CHECK(provenance<>'system_calculated' OR nullif(trim(calculation_version),'') IS NOT NULL),
 CHECK(provenance<>'user_overridden' OR (actor_id IS NOT NULL AND previous_event_id IS NOT NULL AND nullif(trim(reason),'') IS NOT NULL))
);
CREATE INDEX value_events_field_history_idx ON finbook.value_events(entity_type,entity_id,field_name,id);
CREATE TABLE finbook.attachments (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), entity_type text NOT NULL, entity_id uuid NOT NULL, storage_key text NOT NULL, original_name text NOT NULL, sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'), content_type text NOT NULL, byte_length bigint NOT NULL CHECK(byte_length>=0), created_by uuid REFERENCES finbook.users(id), created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX attachments_entity_idx ON finbook.attachments(entity_type,entity_id);
CREATE TABLE finbook.after_sales (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), original_line_id uuid NOT NULL REFERENCES finbook.sales_lines(id),
 kind text NOT NULL CHECK(kind IN ('cancellation','refund','reissue','exchange','void','adm','acm','no_show','adjustment')),
 status text NOT NULL DEFAULT 'requested' CHECK(status IN ('requested','confirmed','approved','received','paid','posted','rejected','reversed')),
 replacement_ticket_id uuid REFERENCES finbook.tickets(id), reason text NOT NULL, currency_code text NOT NULL REFERENCES finbook.currencies(code),
 supplier_penalty finbook.money_amount, agency_fee finbook.money_amount, refund_amount finbook.money_amount, rule_id uuid REFERENCES finbook.commercial_rules(id),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz
);

CREATE TABLE finbook.payments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), branch_id uuid NOT NULL REFERENCES finbook.branches(id), payment_number text,
 direction text NOT NULL CHECK(direction IN ('receipt','payment','transfer')), payment_date date NOT NULL, posting_date date,
 bank_cash_account_id uuid NOT NULL REFERENCES finbook.bank_cash_accounts(id), currency_code text NOT NULL REFERENCES finbook.currencies(code),
 method text NOT NULL CHECK(method IN ('cash','transfer','cheque')), reference text, cheque_number text, cheque_date date,
 status finbook.document_status NOT NULL DEFAULT 'draft', narration text, version integer NOT NULL DEFAULT 1 CHECK(version>0),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
 UNIQUE(branch_id,payment_number), CHECK(method='cheque' OR (cheque_number IS NULL AND cheque_date IS NULL))
);
CREATE INDEX payment_reference_review_idx ON finbook.payments(bank_cash_account_id,reference) WHERE reference IS NOT NULL AND deleted_at IS NULL;
CREATE TABLE finbook.payment_lines (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), payment_id uuid NOT NULL REFERENCES finbook.payments(id), line_number integer NOT NULL CHECK(line_number>0),
 party_id uuid REFERENCES finbook.parties(id), ledger_account_id uuid NOT NULL REFERENCES finbook.accounts(id), amount finbook.money_amount NOT NULL CHECK(amount>0),
 invoice_reference text, narration text, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(payment_id,line_number)
);
CREATE TABLE finbook.vouchers (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), branch_id uuid NOT NULL REFERENCES finbook.branches(id), voucher_type_id uuid NOT NULL REFERENCES finbook.voucher_types(id),
 voucher_number text NOT NULL, posting_date date NOT NULL, period_id uuid NOT NULL REFERENCES finbook.accounting_periods(id), currency_code text NOT NULL REFERENCES finbook.currencies(code),
 status finbook.voucher_status NOT NULL DEFAULT 'draft', idempotency_key text NOT NULL UNIQUE CHECK(length(trim(idempotency_key))>0),
 payload_sha256 bytea NOT NULL CHECK(octet_length(payload_sha256)=32), source_sales_id uuid REFERENCES finbook.sales_documents(id), source_payment_id uuid REFERENCES finbook.payments(id), source_after_sales_id uuid REFERENCES finbook.after_sales(id),
 reversal_of uuid UNIQUE REFERENCES finbook.vouchers(id), reversed_by uuid UNIQUE REFERENCES finbook.vouchers(id), reversal_reason text,
 narration text, prepared_by uuid REFERENCES finbook.users(id), posted_by uuid REFERENCES finbook.users(id), posted_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(branch_id,voucher_number),
 CHECK(num_nonnulls(source_sales_id,source_payment_id,source_after_sales_id)<=1),
 CHECK(reversal_of IS DISTINCT FROM id AND reversed_by IS DISTINCT FROM id),
 CHECK(status NOT IN ('posted','reversed') OR (posted_by IS NOT NULL AND posted_at IS NOT NULL)),
 CHECK(status<>'reversed' OR (reversed_by IS NOT NULL AND length(trim(reversal_reason))>0))
);
CREATE INDEX vouchers_period_status_idx ON finbook.vouchers(period_id,status,posting_date);
CREATE TABLE finbook.voucher_lines (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), voucher_id uuid NOT NULL REFERENCES finbook.vouchers(id), line_number integer NOT NULL CHECK(line_number>0),
 account_id uuid NOT NULL REFERENCES finbook.accounts(id), party_id uuid REFERENCES finbook.parties(id), cost_centre_id uuid REFERENCES finbook.cost_centres(id),
 debit finbook.money_amount NOT NULL DEFAULT 0 CHECK(debit>=0), credit finbook.money_amount NOT NULL DEFAULT 0 CHECK(credit>=0),
 foreign_currency text REFERENCES finbook.currencies(code), foreign_amount finbook.money_amount, exchange_rate finbook.exchange_rate_value CHECK(exchange_rate>0),
 narration text, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(voucher_id,line_number), CHECK((debit>0 AND credit=0) OR (credit>0 AND debit=0)),
 CHECK((foreign_currency IS NULL AND foreign_amount IS NULL AND exchange_rate IS NULL) OR (foreign_currency IS NOT NULL AND foreign_amount IS NOT NULL AND exchange_rate IS NOT NULL))
);
CREATE INDEX voucher_lines_account_idx ON finbook.voucher_lines(account_id,voucher_id);
CREATE TABLE finbook.invoices (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), sales_document_id uuid NOT NULL UNIQUE REFERENCES finbook.sales_documents(id), invoice_number text NOT NULL UNIQUE, invoice_date date NOT NULL, idempotency_key text NOT NULL UNIQUE, created_by uuid NOT NULL REFERENCES finbook.users(id), created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE finbook.allocations (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), payment_line_id uuid NOT NULL REFERENCES finbook.payment_lines(id), invoice_id uuid REFERENCES finbook.invoices(id), supplier_line_id uuid REFERENCES finbook.sales_lines(id), side finbook.accounting_side NOT NULL, amount finbook.money_amount NOT NULL CHECK(amount>0), reversal_of uuid UNIQUE REFERENCES finbook.allocations(id), created_at timestamptz NOT NULL DEFAULT now(), CHECK(num_nonnulls(invoice_id,supplier_line_id)=1));
CREATE TABLE finbook.bank_statement_imports (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), bank_cash_account_id uuid NOT NULL REFERENCES finbook.bank_cash_accounts(id), sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'), original_name text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(bank_cash_account_id,sha256));
CREATE TABLE finbook.bank_statement_lines (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), import_id uuid NOT NULL REFERENCES finbook.bank_statement_imports(id), sequence integer NOT NULL CHECK(sequence>0), transaction_date date NOT NULL, signed_amount finbook.money_amount NOT NULL, reference text, raw_row jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(import_id,sequence));
CREATE TABLE finbook.bank_matches (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), statement_line_id uuid NOT NULL REFERENCES finbook.bank_statement_lines(id), voucher_line_id uuid NOT NULL REFERENCES finbook.voucher_lines(id), matched_by uuid NOT NULL REFERENCES finbook.users(id), idempotency_key text NOT NULL UNIQUE, reversed_match_id uuid UNIQUE REFERENCES finbook.bank_matches(id), reason text, created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX bank_match_pair_idx ON finbook.bank_matches(statement_line_id,voucher_line_id);

CREATE TABLE finbook.audit_log (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 actor_id uuid REFERENCES finbook.users(id), database_actor text NOT NULL DEFAULT session_user, request_id uuid,
 event text NOT NULL, entity_table text NOT NULL, entity_key text NOT NULL, old_values jsonb, new_values jsonb, reason text
);
CREATE INDEX audit_entity_time_idx ON finbook.audit_log(entity_table,entity_key,occurred_at);
CREATE INDEX audit_actor_time_idx ON finbook.audit_log(actor_id,occurred_at);
REVOKE ALL ON finbook.audit_log FROM PUBLIC;

CREATE VIEW finbook.posted_entries AS
 SELECT l.id,l.voucher_id,v.voucher_number,v.posting_date,v.period_id,v.currency_code,l.account_id,l.party_id,l.cost_centre_id,l.debit,l.credit
 FROM finbook.voucher_lines l JOIN finbook.vouchers v ON v.id=l.voucher_id WHERE v.status IN ('posted','reversed');
COMMENT ON VIEW finbook.posted_entries IS 'Reversed originals remain in the ledger together with their posted reversing entries. Drafts are excluded. Financial reports must use this view.';
CREATE VIEW finbook.trial_balance AS
 SELECT account_id,currency_code,sum(debit) AS debit,sum(credit) AS credit,sum(debit-credit) AS signed_balance FROM finbook.posted_entries GROUP BY account_id,currency_code;
CREATE VIEW finbook.statement_balances AS
 SELECT a.id AS account_id,a.code,a.name,a.classification,t.currency_code,t.debit,t.credit,t.signed_balance
 FROM finbook.trial_balance t JOIN finbook.accounts a ON a.id=t.account_id;
CREATE VIEW finbook.sales_line_margin AS
 SELECT l.id AS sales_line_id,c.currency_code,c.total AS client_total,s.total AS supplier_total,
 CASE WHEN c.currency_code=s.currency_code THEN c.total-s.total ELSE NULL END AS margin
 FROM finbook.sales_lines l LEFT JOIN finbook.line_accounting c ON c.sales_line_id=l.id AND c.side='client'
 LEFT JOIN finbook.line_accounting s ON s.sales_line_id=l.id AND s.side='supplier';
