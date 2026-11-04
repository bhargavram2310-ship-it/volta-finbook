CREATE FUNCTION finbook.deny_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Immutable record: append an authorized correction or reversal' USING ERRCODE='55000'; END $$;

CREATE FUNCTION finbook.audit_change() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,finbook AS $$
DECLARE before_row jsonb; after_row jsonb; actor text; request text;
BEGIN
 IF TG_OP<>'INSERT' THEN before_row=to_jsonb(OLD)-ARRAY['password_hash','token_hash','passport_ciphertext','account_number_ciphertext','raw_bytes']; END IF;
 IF TG_OP<>'DELETE' THEN after_row=to_jsonb(NEW)-ARRAY['password_hash','token_hash','passport_ciphertext','account_number_ciphertext','raw_bytes']; END IF;
 actor=nullif(current_setting('finbook.actor_id',true),''); request=nullif(current_setting('finbook.request_id',true),'');
 INSERT INTO finbook.audit_log(actor_id,request_id,event,entity_table,entity_key,old_values,new_values,reason)
 VALUES(actor::uuid,request::uuid,lower(TG_OP),TG_TABLE_NAME,coalesce(after_row->>'id',before_row->>'id',after_row->>'code',before_row->>'code',after_row::text,before_row::text),before_row,after_row,nullif(current_setting('finbook.reason',true),''));
 IF TG_OP='DELETE' THEN RETURN OLD; END IF; RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION finbook.audit_change() FROM PUBLIC;
CREATE TRIGGER immutable_audit BEFORE UPDATE OR DELETE OR TRUNCATE ON finbook.audit_log FOR EACH STATEMENT EXECUTE FUNCTION finbook.deny_mutation();
CREATE TRIGGER immutable_decisions BEFORE UPDATE OR DELETE OR TRUNCATE ON finbook.approval_decisions FOR EACH STATEMENT EXECUTE FUNCTION finbook.deny_mutation();
CREATE TRIGGER immutable_corrections BEFORE UPDATE OR DELETE OR TRUNCATE ON finbook.field_corrections FOR EACH STATEMENT EXECUTE FUNCTION finbook.deny_mutation();
CREATE TRIGGER immutable_migration_history BEFORE UPDATE OR DELETE OR TRUNCATE ON finbook.schema_migrations FOR EACH STATEMENT EXECUTE FUNCTION finbook.deny_mutation();

CREATE FUNCTION finbook.validate_period_bounds() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE fy finbook.financial_years;
BEGIN
 SELECT * INTO fy FROM finbook.financial_years WHERE id=NEW.financial_year_id FOR SHARE;
 IF NEW.starts_on<fy.starts_on OR NEW.ends_on>fy.ends_on THEN RAISE EXCEPTION 'Period must fit within its financial year' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' AND (NEW.starts_on,NEW.ends_on,NEW.financial_year_id) IS DISTINCT FROM (OLD.starts_on,OLD.ends_on,OLD.financial_year_id)
 AND EXISTS(SELECT 1 FROM finbook.vouchers WHERE period_id=OLD.id AND status IN ('posted','reversed')) THEN RAISE EXCEPTION 'A period with posted entries cannot be redefined' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER period_bounds BEFORE INSERT OR UPDATE ON finbook.accounting_periods FOR EACH ROW EXECUTE FUNCTION finbook.validate_period_bounds();
CREATE FUNCTION finbook.validate_year_bounds() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM finbook.accounting_periods WHERE financial_year_id=OLD.id AND (starts_on<NEW.starts_on OR ends_on>NEW.ends_on)) THEN RAISE EXCEPTION 'Financial year cannot exclude existing periods' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER year_bounds BEFORE UPDATE ON finbook.financial_years FOR EACH ROW EXECUTE FUNCTION finbook.validate_year_bounds();

CREATE FUNCTION finbook.guard_voucher_lines() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE parent_id uuid; parent_status finbook.voucher_status;
BEGIN
 IF TG_OP='UPDATE' AND NEW.voucher_id<>OLD.voucher_id THEN RAISE EXCEPTION 'Voucher lines cannot be reparented' USING ERRCODE='55000'; END IF;
 IF TG_OP='DELETE' THEN parent_id=OLD.voucher_id; ELSE parent_id=NEW.voucher_id; END IF;
 SELECT status INTO parent_status FROM finbook.vouchers WHERE id=parent_id FOR UPDATE;
 IF parent_status IN ('posted','reversed') THEN RAISE EXCEPTION 'Posted lines require reversal, not modification' USING ERRCODE='55000'; END IF;
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Use controlled draft adjustment instead of physical deletion' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER protect_voucher_lines BEFORE INSERT OR UPDATE OR DELETE ON finbook.voucher_lines FOR EACH ROW EXECUTE FUNCTION finbook.guard_voucher_lines();
CREATE TRIGGER protect_voucher_lines_truncate BEFORE TRUNCATE ON finbook.voucher_lines EXECUTE FUNCTION finbook.deny_mutation();

CREATE FUNCTION finbook.guard_voucher() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE total_debit numeric; total_credit numeric; entry_count bigint; period finbook.accounting_periods; fy finbook.financial_years; decimals integer; base text; reverse_id uuid;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Vouchers cannot be physically deleted' USING ERRCODE='55000'; END IF;
 IF TG_OP='INSERT' AND NEW.status<>'draft' THEN RAISE EXCEPTION 'New vouchers must begin as drafts' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' AND OLD.status IN ('posted','reversed') THEN
  IF OLD.status='posted' AND NEW.status='reversed'
    AND (to_jsonb(NEW)-ARRAY['status','reversed_by','reversal_reason','updated_at'])=(to_jsonb(OLD)-ARRAY['status','reversed_by','reversal_reason','updated_at']) THEN
   SELECT id INTO reverse_id FROM finbook.vouchers WHERE id=NEW.reversed_by AND reversal_of=OLD.id AND status='posted';
   IF reverse_id IS NULL OR nullif(trim(NEW.reversal_reason),'') IS NULL THEN RAISE EXCEPTION 'A posted matching reversal and reason are required' USING ERRCODE='23514'; END IF;
   RETURN NEW;
  END IF;
  RAISE EXCEPTION 'Posted vouchers are immutable; use a linked reversal' USING ERRCODE='55000';
 END IF;
 IF TG_OP='UPDATE' AND NEW.status='reversed' THEN RAISE EXCEPTION 'Only a posted voucher may be marked reversed' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' AND NEW.status='posted' THEN
  SELECT * INTO period FROM finbook.accounting_periods WHERE id=NEW.period_id FOR SHARE;
  SELECT * INTO fy FROM finbook.financial_years WHERE id=period.financial_year_id FOR SHARE;
  IF period.id IS NULL OR period.status<>'open' OR fy.closed OR NEW.posting_date NOT BETWEEN period.starts_on AND period.ends_on THEN RAISE EXCEPTION 'Posting requires an open matching accounting period' USING ERRCODE='23514'; END IF;
  SELECT c.base_currency INTO base FROM finbook.company c JOIN finbook.branches b ON b.company_id=c.id WHERE b.id=NEW.branch_id AND b.active AND b.deleted_at IS NULL FOR SHARE OF c,b;
  IF NEW.currency_code IS DISTINCT FROM base THEN RAISE EXCEPTION 'Ledger vouchers must use configured company base currency' USING ERRCODE='23514'; END IF;
  SELECT decimal_places INTO decimals FROM finbook.currencies WHERE code=NEW.currency_code AND active AND deleted_at IS NULL;
  IF decimals IS NULL THEN RAISE EXCEPTION 'Posting currency must be active' USING ERRCODE='23514'; END IF;
  PERFORM 1 FROM finbook.accounts WHERE id IN (SELECT account_id FROM finbook.voucher_lines WHERE voucher_id=NEW.id) ORDER BY id FOR SHARE;
  PERFORM 1 FROM finbook.voucher_types WHERE id=NEW.voucher_type_id AND active AND deleted_at IS NULL FOR SHARE;
  IF NOT FOUND THEN RAISE EXCEPTION 'Voucher type must be active' USING ERRCODE='23514'; END IF;
  SELECT count(*),coalesce(sum(debit),0),coalesce(sum(credit),0) INTO entry_count,total_debit,total_credit FROM finbook.voucher_lines WHERE voucher_id=NEW.id;
  IF entry_count<2 OR total_debit<=0 OR total_debit<>total_credit THEN RAISE EXCEPTION 'Posting blocked: debits must equal credits across at least two positive lines' USING ERRCODE='23514'; END IF;
  IF EXISTS(SELECT 1 FROM finbook.voucher_lines l JOIN finbook.accounts a ON a.id=l.account_id WHERE l.voucher_id=NEW.id AND (NOT a.active OR NOT a.postable OR a.deleted_at IS NOT NULL OR l.debit<>trunc(l.debit,decimals) OR l.credit<>trunc(l.credit,decimals))) THEN RAISE EXCEPTION 'Account state or currency precision is invalid for posting' USING ERRCODE='23514'; END IF;
  IF NEW.reversal_of IS NOT NULL THEN
   PERFORM 1 FROM finbook.vouchers WHERE id=NEW.reversal_of AND status='posted' AND currency_code=NEW.currency_code FOR UPDATE;
   IF NOT FOUND THEN RAISE EXCEPTION 'Reversal source must be a posted voucher in the same currency' USING ERRCODE='23514'; END IF;
   IF EXISTS(
    WITH original AS (SELECT account_id,party_id,cost_centre_id,sum(debit) d,sum(credit) c FROM finbook.voucher_lines WHERE voucher_id=NEW.reversal_of GROUP BY account_id,party_id,cost_centre_id),
         reversing AS (SELECT account_id,party_id,cost_centre_id,sum(debit) d,sum(credit) c FROM finbook.voucher_lines WHERE voucher_id=NEW.id GROUP BY account_id,party_id,cost_centre_id)
    SELECT 1 FROM original o FULL JOIN reversing r ON o.account_id=r.account_id AND o.party_id IS NOT DISTINCT FROM r.party_id AND o.cost_centre_id IS NOT DISTINCT FROM r.cost_centre_id
    WHERE coalesce(o.d,0)<>coalesce(r.c,0) OR coalesce(o.c,0)<>coalesce(r.d,0)
   ) THEN RAISE EXCEPTION 'Reversal must exactly offset the original account and allocation dimensions' USING ERRCODE='23514'; END IF;
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER protect_voucher BEFORE INSERT OR UPDATE OR DELETE ON finbook.vouchers FOR EACH ROW EXECUTE FUNCTION finbook.guard_voucher();
CREATE TRIGGER protect_voucher_truncate BEFORE TRUNCATE ON finbook.vouchers EXECUTE FUNCTION finbook.deny_mutation();

CREATE FUNCTION finbook.require_posted_sale_for_invoice() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE state finbook.document_status;
BEGIN
 SELECT status INTO state FROM finbook.sales_documents WHERE id=NEW.sales_document_id FOR SHARE;
 IF state<>'posted' THEN RAISE EXCEPTION 'Official invoices require a posted sale' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER invoice_posting_guard BEFORE INSERT ON finbook.invoices FOR EACH ROW EXECUTE FUNCTION finbook.require_posted_sale_for_invoice();
CREATE TRIGGER immutable_invoices BEFORE UPDATE OR DELETE OR TRUNCATE ON finbook.invoices FOR EACH STATEMENT EXECUTE FUNCTION finbook.deny_mutation();

CREATE FUNCTION finbook.guard_sales_document() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Use a controlled reversal or soft delete' USING ERRCODE='55000'; END IF;
 IF TG_OP='INSERT' AND NEW.status<>'draft' THEN RAISE EXCEPTION 'Sales documents must begin as drafts' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' AND OLD.status IN ('posted','cancelled','refunded','reversed') THEN RAISE EXCEPTION 'Finalized sales documents are immutable; record linked after-sales events' USING ERRCODE='55000'; END IF;
 IF NEW.status='posted' THEN
  IF TG_OP='INSERT' OR NEW.customer_id IS NULL OR NEW.document_number IS NULL OR NOT EXISTS(SELECT 1 FROM finbook.sales_lines WHERE document_id=NEW.id) THEN RAISE EXCEPTION 'Sale cannot post without document, customer and lines' USING ERRCODE='23514'; END IF;
  IF EXISTS(SELECT 1 FROM finbook.sales_lines l WHERE l.document_id=NEW.id AND l.deleted_at IS NOT NULL) OR EXISTS(SELECT 1 FROM finbook.sales_lines l WHERE l.document_id=NEW.id AND (l.supplier_id IS NULL OR (SELECT count(*) FROM finbook.line_accounting a WHERE a.sales_line_id=l.id AND a.total IS NOT NULL)<>2)) THEN RAISE EXCEPTION 'Both client and supplier accounting must be complete' USING ERRCODE='23514'; END IF;
  IF NOT EXISTS(SELECT 1 FROM finbook.vouchers WHERE source_sales_id=NEW.id AND status='posted') THEN RAISE EXCEPTION 'Sale must have a posted source voucher' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER protect_sale BEFORE INSERT OR UPDATE OR DELETE ON finbook.sales_documents FOR EACH ROW EXECUTE FUNCTION finbook.guard_sales_document();

CREATE FUNCTION finbook.guard_sale_child() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE line_id uuid; doc_id uuid; state finbook.document_status; ticket uuid;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Financial source rows cannot be physically deleted' USING ERRCODE='55000'; END IF;
 IF TG_TABLE_NAME='sales_lines' THEN
  IF TG_OP='UPDATE' AND NEW.document_id<>OLD.document_id THEN RAISE EXCEPTION 'Sales lines cannot be reparented' USING ERRCODE='55000'; END IF;
  doc_id=NEW.document_id;
 ELSIF TG_TABLE_NAME='line_accounting' THEN
  IF TG_OP='UPDATE' AND NEW.sales_line_id<>OLD.sales_line_id THEN RAISE EXCEPTION 'Accounting rows cannot be reparented' USING ERRCODE='55000'; END IF;
  line_id=NEW.sales_line_id;
 ELSE
  IF TG_OP='UPDATE' AND NEW.accounting_id<>OLD.accounting_id THEN RAISE EXCEPTION 'Accounting components cannot be reparented' USING ERRCODE='55000'; END IF;
  SELECT sales_line_id INTO line_id FROM finbook.line_accounting WHERE id=NEW.accounting_id;
 END IF;
 IF doc_id IS NULL THEN SELECT document_id INTO doc_id FROM finbook.sales_lines WHERE id=line_id; END IF;
 SELECT status INTO state FROM finbook.sales_documents WHERE id=doc_id FOR UPDATE;
 IF state IN ('posted','cancelled','refunded','reversed') THEN RAISE EXCEPTION 'Finalized sales data requires linked adjustments' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER protect_sales_line BEFORE INSERT OR UPDATE OR DELETE ON finbook.sales_lines FOR EACH ROW EXECUTE FUNCTION finbook.guard_sale_child();
CREATE TRIGGER protect_line_accounting BEFORE INSERT OR UPDATE OR DELETE ON finbook.line_accounting FOR EACH ROW EXECUTE FUNCTION finbook.guard_sale_child();
CREATE TRIGGER protect_accounting_components BEFORE INSERT OR UPDATE OR DELETE ON finbook.line_accounting_components FOR EACH ROW EXECUTE FUNCTION finbook.guard_sale_child();

CREATE FUNCTION finbook.guard_ticket() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE state finbook.document_status;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Tickets cannot be physically deleted' USING ERRCODE='55000'; END IF;
 FOR state IN SELECT d.status FROM finbook.sales_documents d JOIN finbook.sales_lines l ON l.document_id=d.id WHERE l.ticket_id=OLD.id ORDER BY d.id FOR UPDATE OF d LOOP
  IF state IN ('posted','cancelled','refunded','reversed') THEN RAISE EXCEPTION 'Posted ticket identity is immutable; use after-sales events' USING ERRCODE='55000'; END IF;
 END LOOP;
 RETURN NEW;
END $$;
CREATE TRIGGER protect_ticket BEFORE UPDATE OR DELETE ON finbook.tickets FOR EACH ROW EXECUTE FUNCTION finbook.guard_ticket();
CREATE FUNCTION finbook.guard_segment() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE ticket uuid;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Segments cannot be physically deleted' USING ERRCODE='55000'; END IF;
 IF TG_OP='UPDATE' AND NEW.ticket_id<>OLD.ticket_id THEN RAISE EXCEPTION 'Segments cannot be reparented' USING ERRCODE='55000'; END IF;
 ticket=NEW.ticket_id;
 PERFORM 1 FROM finbook.tickets WHERE id=ticket FOR UPDATE;
 IF EXISTS(SELECT 1 FROM finbook.sales_lines l JOIN finbook.sales_documents d ON d.id=l.document_id WHERE l.ticket_id=ticket AND d.status IN ('posted','cancelled','refunded','reversed')) THEN RAISE EXCEPTION 'Posted itinerary is immutable' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER protect_segment BEFORE INSERT OR UPDATE OR DELETE ON finbook.ticket_segments FOR EACH ROW EXECUTE FUNCTION finbook.guard_segment();

CREATE FUNCTION finbook.guard_payment() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE kind text;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Payments cannot be physically deleted' USING ERRCODE='55000'; END IF;
 IF TG_OP='INSERT' AND NEW.status<>'draft' THEN RAISE EXCEPTION 'Payments must begin as drafts' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' AND OLD.status IN ('posted','cancelled','refunded','reversed') THEN RAISE EXCEPTION 'Posted payments require reversal' USING ERRCODE='55000'; END IF;
 SELECT a.kind INTO kind FROM finbook.bank_cash_accounts a WHERE a.id=NEW.bank_cash_account_id;
 IF (kind='cash' AND NEW.method<>'cash') OR (kind='bank' AND NEW.method='cash') THEN RAISE EXCEPTION 'Payment method must match bank/cash account type' USING ERRCODE='23514'; END IF;
 IF NEW.status='posted' AND (NEW.posting_date IS NULL OR NEW.payment_number IS NULL OR (NEW.method='cheque' AND (NEW.cheque_number IS NULL OR NEW.cheque_date IS NULL)) OR NOT EXISTS(SELECT 1 FROM finbook.vouchers WHERE source_payment_id=NEW.id AND status='posted')) THEN RAISE EXCEPTION 'Payment posting requires document details and a posted voucher' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER protect_payment BEFORE INSERT OR UPDATE OR DELETE ON finbook.payments FOR EACH ROW EXECUTE FUNCTION finbook.guard_payment();
CREATE FUNCTION finbook.guard_payment_line() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE state finbook.document_status;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Payment lines cannot be physically deleted' USING ERRCODE='55000'; END IF;
 IF TG_OP='UPDATE' AND NEW.payment_id<>OLD.payment_id THEN RAISE EXCEPTION 'Payment lines cannot be reparented' USING ERRCODE='55000'; END IF;
 SELECT status INTO state FROM finbook.payments WHERE id=NEW.payment_id FOR UPDATE;
 IF state IN ('posted','cancelled','refunded','reversed') THEN RAISE EXCEPTION 'Posted payment lines require reversal' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER protect_payment_line BEFORE INSERT OR UPDATE OR DELETE ON finbook.payment_lines FOR EACH ROW EXECUTE FUNCTION finbook.guard_payment_line();

-- Original MIR bytes, extracted fields and financial allocations are append-only.
DO $$ DECLARE target text; BEGIN
 FOREACH target IN ARRAY ARRAY['mir_source_lines','mir_fields','allocations','bank_statement_imports','bank_statement_lines','bank_matches','attachments','value_events'] LOOP
  EXECUTE format('CREATE TRIGGER immutable_original BEFORE UPDATE OR DELETE OR TRUNCATE ON finbook.%I FOR EACH STATEMENT EXECUTE FUNCTION finbook.deny_mutation()',target);
 END LOOP;
END $$;

-- Audit every durable business/configuration mutation, including role assignments.
DO $$ DECLARE target text; BEGIN
 FOR target IN SELECT tablename FROM pg_tables WHERE schemaname='finbook' AND tablename NOT IN ('audit_log','schema_migrations') LOOP
  EXECUTE format('CREATE TRIGGER record_audit AFTER INSERT OR UPDATE OR DELETE ON finbook.%I FOR EACH ROW EXECUTE FUNCTION finbook.audit_change()',target);
  EXECUTE format('CREATE TRIGGER protect_all_truncate BEFORE TRUNCATE ON finbook.%I EXECUTE FUNCTION finbook.deny_mutation()',target);
 END LOOP;
 FOREACH target IN ARRAY ARRAY['sales_documents','sales_lines','line_accounting','line_accounting_components','tickets','ticket_segments','payments','payment_lines','mir_records','after_sales'] LOOP
  EXECUTE format('CREATE TRIGGER deny_truncate BEFORE TRUNCATE ON finbook.%I EXECUTE FUNCTION finbook.deny_mutation()',target);
 END LOOP;
END $$;
