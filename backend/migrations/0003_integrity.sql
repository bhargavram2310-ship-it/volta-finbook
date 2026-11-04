CREATE FUNCTION finbook.guard_mir_original() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE allowed text[];
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Original MIR evidence cannot be deleted' USING ERRCODE='55000'; END IF;
 allowed=ARRAY['status']; IF TG_TABLE_NAME='mir_parse_runs' THEN allowed=ARRAY['status','completed_at']; END IF;
 IF (to_jsonb(NEW)-allowed) IS DISTINCT FROM (to_jsonb(OLD)-allowed) THEN RAISE EXCEPTION 'Original MIR evidence is immutable; append a new parse run or correction' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER protect_mir_file BEFORE UPDATE OR DELETE ON finbook.mir_files FOR EACH ROW EXECUTE FUNCTION finbook.guard_mir_original();
CREATE TRIGGER protect_mir_record BEFORE UPDATE OR DELETE ON finbook.mir_records FOR EACH ROW EXECUTE FUNCTION finbook.guard_mir_original();
CREATE TRIGGER protect_mir_run BEFORE UPDATE OR DELETE ON finbook.mir_parse_runs FOR EACH ROW EXECUTE FUNCTION finbook.guard_mir_original();
CREATE TRIGGER protect_mir_file_truncate BEFORE TRUNCATE ON finbook.mir_files EXECUTE FUNCTION finbook.deny_mutation();
CREATE TRIGGER protect_mir_run_truncate BEFORE TRUNCATE ON finbook.mir_parse_runs EXECUTE FUNCTION finbook.deny_mutation();
CREATE FUNCTION finbook.validate_mir_field() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM finbook.mir_records r JOIN finbook.mir_source_lines l ON l.file_id=r.file_id AND l.line_number=NEW.line_number JOIN finbook.mir_files f ON f.id=r.file_id WHERE r.id=NEW.record_id AND NEW.byte_end<=octet_length(f.raw_bytes)) THEN RAISE EXCEPTION 'MIR field must reference a retained source line and valid byte range' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER mir_field_source BEFORE INSERT ON finbook.mir_fields FOR EACH ROW EXECUTE FUNCTION finbook.validate_mir_field();

CREATE FUNCTION finbook.validate_value_event() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE previous finbook.value_events;
BEGIN
 IF NEW.previous_event_id IS NOT NULL THEN
  SELECT * INTO previous FROM finbook.value_events WHERE id=NEW.previous_event_id;
  IF (previous.entity_type,previous.entity_id,previous.field_name) IS DISTINCT FROM (NEW.entity_type,NEW.entity_id,NEW.field_name) THEN RAISE EXCEPTION 'Field history must stay on the same entity and field' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER value_event_history BEFORE INSERT ON finbook.value_events FOR EACH ROW EXECUTE FUNCTION finbook.validate_value_event();

CREATE FUNCTION finbook.protect_verified_tax() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.verification_status='verified' AND (to_jsonb(NEW)-ARRAY['active','deleted_at','updated_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['active','deleted_at','updated_at']) THEN RAISE EXCEPTION 'Verified tax definitions are versioned; create a new rule version' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER verified_tax_immutable BEFORE UPDATE ON finbook.tax_rules FOR EACH ROW EXECUTE FUNCTION finbook.protect_verified_tax();

CREATE FUNCTION finbook.protect_after_sales() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Use a linked after-sales reversal' USING ERRCODE='55000'; END IF;
 IF TG_OP='UPDATE' AND OLD.status IN ('posted','reversed') THEN RAISE EXCEPTION 'Posted after-sales events are immutable' USING ERRCODE='55000'; END IF;
 IF NEW.status='posted' AND NOT EXISTS(SELECT 1 FROM finbook.vouchers WHERE source_after_sales_id=NEW.id AND status='posted') THEN RAISE EXCEPTION 'After-sales posting requires a posted voucher' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER after_sales_posted_guard BEFORE INSERT OR UPDATE OR DELETE ON finbook.after_sales FOR EACH ROW EXECUTE FUNCTION finbook.protect_after_sales();

CREATE FUNCTION finbook.protect_account_classification() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.classification,NEW.parent_id) IS DISTINCT FROM (OLD.classification,OLD.parent_id) AND EXISTS(SELECT 1 FROM finbook.posted_entries WHERE account_id=OLD.id) THEN RAISE EXCEPTION 'Posted ledger classification requires a controlled new account' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER account_classification_guard BEFORE UPDATE ON finbook.accounts FOR EACH ROW EXECUTE FUNCTION finbook.protect_account_classification();

CREATE FUNCTION finbook.protect_base_currency() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.base_currency<>OLD.base_currency AND EXISTS(SELECT 1 FROM finbook.vouchers WHERE status IN ('posted','reversed')) THEN RAISE EXCEPTION 'Company base currency cannot change after posting begins' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER company_base_currency_guard BEFORE UPDATE ON finbook.company FOR EACH ROW EXECUTE FUNCTION finbook.protect_base_currency();

CREATE FUNCTION finbook.protect_currency_precision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.decimal_places<>OLD.decimal_places AND EXISTS(SELECT 1 FROM finbook.vouchers WHERE currency_code=OLD.code AND status IN ('posted','reversed')) THEN RAISE EXCEPTION 'Currency precision cannot change after posting begins' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER currency_precision_guard BEFORE UPDATE ON finbook.currencies FOR EACH ROW EXECUTE FUNCTION finbook.protect_currency_precision();

CREATE FUNCTION finbook.validate_allocation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE payment finbook.payments; line_amount numeric; allocated numeric; target_amount numeric; target_currency text; target_used numeric; original finbook.allocations;
BEGIN
 SELECT p.* INTO payment FROM finbook.payments p JOIN finbook.payment_lines l ON l.payment_id=p.id WHERE l.id=NEW.payment_line_id FOR UPDATE OF p;
 SELECT amount INTO line_amount FROM finbook.payment_lines WHERE id=NEW.payment_line_id FOR UPDATE;
 IF payment.status<>'posted' THEN RAISE EXCEPTION 'Allocations require a posted receipt/payment' USING ERRCODE='23514'; END IF;
 IF NEW.reversal_of IS NOT NULL THEN
  SELECT * INTO original FROM finbook.allocations WHERE id=NEW.reversal_of FOR UPDATE;
  IF original.id IS NULL OR original.reversal_of IS NOT NULL OR (NEW.payment_line_id,NEW.invoice_id,NEW.supplier_line_id,NEW.side,NEW.amount) IS DISTINCT FROM (original.payment_line_id,original.invoice_id,original.supplier_line_id,original.side,original.amount) THEN RAISE EXCEPTION 'Allocation reversal must match the original exactly' USING ERRCODE='23514'; END IF;
  RETURN NEW;
 END IF;
 SELECT coalesce(sum(CASE WHEN reversal_of IS NULL THEN amount ELSE -amount END),0) INTO allocated FROM finbook.allocations WHERE payment_line_id=NEW.payment_line_id;
 IF allocated+NEW.amount>line_amount THEN RAISE EXCEPTION 'Allocation exceeds payment line amount' USING ERRCODE='23514'; END IF;
 IF NEW.side='client' THEN
  IF NEW.invoice_id IS NULL OR NEW.supplier_line_id IS NOT NULL OR payment.direction<>'receipt' THEN RAISE EXCEPTION 'Client collections must allocate receipts to invoices' USING ERRCODE='23514'; END IF;
  PERFORM 1 FROM finbook.invoices WHERE id=NEW.invoice_id FOR UPDATE;
  SELECT sum(a.total),min(a.currency_code) INTO target_amount,target_currency FROM finbook.invoices i JOIN finbook.sales_lines l ON l.document_id=i.sales_document_id JOIN finbook.line_accounting a ON a.sales_line_id=l.id AND a.side='client' WHERE i.id=NEW.invoice_id;
  SELECT coalesce(sum(CASE WHEN reversal_of IS NULL THEN amount ELSE -amount END),0) INTO target_used FROM finbook.allocations WHERE invoice_id=NEW.invoice_id;
 ELSE
  IF NEW.supplier_line_id IS NULL OR NEW.invoice_id IS NOT NULL OR payment.direction<>'payment' THEN RAISE EXCEPTION 'Supplier settlement must allocate payments to supplier lines' USING ERRCODE='23514'; END IF;
  PERFORM 1 FROM finbook.sales_lines WHERE id=NEW.supplier_line_id FOR UPDATE;
  SELECT a.total,a.currency_code INTO target_amount,target_currency FROM finbook.line_accounting a JOIN finbook.sales_lines l ON l.id=a.sales_line_id JOIN finbook.sales_documents d ON d.id=l.document_id WHERE l.id=NEW.supplier_line_id AND a.side='supplier' AND d.status='posted';
  SELECT coalesce(sum(CASE WHEN reversal_of IS NULL THEN amount ELSE -amount END),0) INTO target_used FROM finbook.allocations WHERE supplier_line_id=NEW.supplier_line_id;
 END IF;
 IF target_amount IS NULL OR target_currency IS DISTINCT FROM payment.currency_code OR target_used+NEW.amount>target_amount THEN RAISE EXCEPTION 'Allocation currency or outstanding limit is invalid' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER allocation_limits BEFORE INSERT ON finbook.allocations FOR EACH ROW EXECUTE FUNCTION finbook.validate_allocation();

CREATE VIEW finbook.customer_outstanding AS
 WITH billed AS (SELECT i.id AS invoice_id,d.customer_id,d.due_date,a.currency_code,sum(a.total) AS billed FROM finbook.invoices i JOIN finbook.sales_documents d ON d.id=i.sales_document_id JOIN finbook.sales_lines l ON l.document_id=d.id JOIN finbook.line_accounting a ON a.sales_line_id=l.id AND a.side='client' WHERE d.status='posted' GROUP BY i.id,d.customer_id,d.due_date,a.currency_code),
 paid AS (SELECT invoice_id,sum(CASE WHEN reversal_of IS NULL THEN amount ELSE -amount END) AS allocated FROM finbook.allocations WHERE invoice_id IS NOT NULL GROUP BY invoice_id)
 SELECT b.*,coalesce(p.allocated,0) AS allocated,b.billed-coalesce(p.allocated,0) AS outstanding FROM billed b LEFT JOIN paid p ON p.invoice_id=b.invoice_id;
COMMENT ON VIEW finbook.customer_outstanding IS 'Posted sales and posted-payment allocations only; controlled credit/refund integration will extend this view in Phase 6/7.';

DO $$ DECLARE target text; BEGIN
 FOR target IN SELECT table_name FROM information_schema.columns WHERE table_schema='finbook' AND column_name='deleted_at' LOOP
  EXECUTE format('CREATE TRIGGER require_soft_delete BEFORE DELETE ON finbook.%I FOR EACH STATEMENT EXECUTE FUNCTION finbook.deny_mutation()',target);
 END LOOP;
END $$;
