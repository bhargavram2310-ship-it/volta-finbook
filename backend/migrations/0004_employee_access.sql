-- Forward-only extension of the managed application schema.
-- Existing public.employees, including salary and all existing rows, is untouched.
-- No sample employees, passwords, users, roles or business transactions are seeded.

ALTER TABLE finbook.employees
 ADD COLUMN official_email text,
 ADD COLUMN mobile_number text,
 ADD COLUMN designation text,
 ADD COLUMN department text,
 ADD COLUMN reporting_to uuid REFERENCES finbook.employees(id),
 ADD COLUMN date_of_joining date,
 ADD COLUMN employment_type text CHECK (employment_type IN ('Permanent','Contract','Temporary','Intern')),
 ADD COLUMN employment_status text NOT NULL DEFAULT 'Active' CHECK (employment_status IN ('Active','Inactive','Resigned')),
 ADD COLUMN date_of_confirmation date,
 ADD COLUMN date_of_leaving date,
 ADD COLUMN date_of_birth date,
 ADD COLUMN gender text,
 ADD COLUMN personal_email text,
 ADD COLUMN personal_mobile_number text,
 ADD COLUMN alternate_contact_number text,
 ADD COLUMN address_line_1 text,
 ADD COLUMN address_line_2 text,
 ADD COLUMN city text,
 ADD COLUMN state text,
 ADD COLUMN country text,
 ADD COLUMN postal_code text,
 ADD COLUMN emergency_contact_name text,
 ADD COLUMN emergency_contact_number text,
 ADD COLUMN emergency_contact_relationship text,
 ADD COLUMN blood_group text,
 ADD COLUMN photo_bytes bytea CHECK (octet_length(photo_bytes)<=2097152),
 ADD COLUMN photo_content_type text CHECK (photo_content_type IN ('image/jpeg','image/png')),
 ADD COLUMN remarks text,
 ADD COLUMN version integer NOT NULL DEFAULT 1 CHECK(version>0),
 ADD COLUMN created_by uuid REFERENCES finbook.users(id),
 ADD COLUMN updated_by uuid REFERENCES finbook.users(id),
 ADD CONSTRAINT employee_no_self_reporting CHECK(reporting_to IS DISTINCT FROM id),
 ADD CONSTRAINT employee_leaving_after_joining CHECK(date_of_leaving IS NULL OR date_of_joining IS NULL OR date_of_leaving>=date_of_joining),
 ADD CONSTRAINT employee_official_email_format CHECK(official_email IS NULL OR official_email ~ '^[^[:space:]@]+@[^[:space:]@]+[.][^[:space:]@]+$'),
 ADD CONSTRAINT employee_personal_email_format CHECK(personal_email IS NULL OR personal_email ~ '^[^[:space:]@]+@[^[:space:]@]+[.][^[:space:]@]+$'),
 ADD CONSTRAINT employee_photo_metadata CHECK((photo_bytes IS NULL)=(photo_content_type IS NULL));
UPDATE finbook.employees SET employment_status='Inactive' WHERE NOT active;
CREATE UNIQUE INDEX employees_official_email_uq ON finbook.employees(lower(official_email)) WHERE official_email IS NOT NULL;
CREATE INDEX employees_department_designation_idx ON finbook.employees(department,designation,employment_status);
CREATE INDEX employees_reporting_to_idx ON finbook.employees(reporting_to);

ALTER TABLE finbook.users
 ADD COLUMN employee_id uuid UNIQUE REFERENCES finbook.employees(id),
 ADD COLUMN login_email text,
 ADD COLUMN account_status text NOT NULL DEFAULT 'Active' CHECK(account_status IN ('Active','Inactive','Locked')),
 ADD COLUMN password_reset_required boolean NOT NULL DEFAULT true,
 ADD COLUMN last_login_at timestamptz,
 ADD COLUMN failed_login_count integer NOT NULL DEFAULT 0 CHECK(failed_login_count>=0),
 ADD COLUMN login_blocked_until timestamptz,
 ADD COLUMN created_by uuid REFERENCES finbook.users(id),
 ADD COLUMN updated_by uuid REFERENCES finbook.users(id),
 ADD COLUMN version integer NOT NULL DEFAULT 1 CHECK(version>0),
 ADD CONSTRAINT users_login_email_format CHECK(login_email IS NULL OR login_email ~ '^[^[:space:]@]+@[^[:space:]@]+[.][^[:space:]@]+$');
-- Reuse legacy mappings without guessing identities from names or emails.
UPDATE finbook.users u SET employee_id=e.id FROM finbook.employees e WHERE e.user_id=u.id;
UPDATE finbook.users SET account_status='Inactive' WHERE NOT active;
-- Existing deployments with unmapped users require explicit mapping first.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM finbook.users WHERE employee_id IS NULL) THEN
  RAISE EXCEPTION 'Existing users require an explicit employee mapping before migration' USING ERRCODE='23514';
 END IF;
END $$;
ALTER TABLE finbook.users ALTER COLUMN employee_id SET NOT NULL;
CREATE UNIQUE INDEX users_login_email_uq ON finbook.users(lower(login_email)) WHERE login_email IS NOT NULL;
COMMENT ON COLUMN finbook.users.employee_id IS 'Authoritative employee-login mapping. One user per employee. Legacy employees.user_id is compatibility-only and must agree when populated.';

ALTER TABLE finbook.roles ADD COLUMN description text, ADD COLUMN created_by uuid REFERENCES finbook.users(id), ADD COLUMN updated_by uuid REFERENCES finbook.users(id), ADD COLUMN version integer NOT NULL DEFAULT 1 CHECK(version>0);
ALTER TABLE finbook.permissions ADD COLUMN created_at timestamptz NOT NULL DEFAULT now(), ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE finbook.user_roles ADD COLUMN created_at timestamptz NOT NULL DEFAULT now(), ADD COLUMN created_by uuid REFERENCES finbook.users(id);
ALTER TABLE finbook.role_permissions ADD COLUMN created_at timestamptz NOT NULL DEFAULT now(), ADD COLUMN created_by uuid REFERENCES finbook.users(id);

CREATE TABLE finbook.role_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES finbook.users(id),
 employee_id uuid NOT NULL REFERENCES finbook.employees(id),
 previous_role_id uuid REFERENCES finbook.roles(id),
 new_role_id uuid REFERENCES finbook.roles(id),
 previous_role_name text,
 new_role_name text,
 action text NOT NULL CHECK(action IN ('Assigned','Modified','Removed')),
 modified_by uuid NOT NULL REFERENCES finbook.users(id),
 modified_at timestamptz NOT NULL DEFAULT now(),
 remarks text NOT NULL CHECK(length(trim(remarks))>0),
 CHECK((action='Assigned' AND previous_role_id IS NULL AND new_role_id IS NOT NULL)
    OR (action='Removed' AND previous_role_id IS NOT NULL AND new_role_id IS NULL)
    OR (action='Modified' AND previous_role_id IS NOT NULL AND new_role_id IS NOT NULL))
);
CREATE INDEX role_history_user_time_idx ON finbook.role_history(user_id,modified_at DESC);
CREATE INDEX role_history_employee_time_idx ON finbook.role_history(employee_id,modified_at DESC);
CREATE INDEX role_history_old_role_idx ON finbook.role_history(previous_role_id,modified_at DESC);
CREATE INDEX role_history_new_role_idx ON finbook.role_history(new_role_id,modified_at DESC);
CREATE TRIGGER immutable_role_history BEFORE UPDATE OR DELETE OR TRUNCATE ON finbook.role_history FOR EACH STATEMENT EXECUTE FUNCTION finbook.deny_mutation();

CREATE TABLE finbook.employee_change_requests (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 employee_id uuid NOT NULL REFERENCES finbook.employees(id),
 requested_by uuid NOT NULL REFERENCES finbook.users(id),
 employee_version integer NOT NULL CHECK(employee_version>0),
 original_values jsonb NOT NULL CHECK(jsonb_typeof(original_values)='object'),
 requested_values jsonb NOT NULL CHECK(jsonb_typeof(requested_values)='object' AND requested_values<>'{}'::jsonb),
 status text NOT NULL DEFAULT 'Pending Approval' CHECK(status IN ('Pending Approval','Approved','Rejected')),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 decided_by uuid REFERENCES finbook.users(id),
 decided_at timestamptz,
 decision_remarks text,
 CHECK((status='Pending Approval' AND decided_by IS NULL AND decided_at IS NULL)
    OR (status IN ('Approved','Rejected') AND decided_by IS NOT NULL AND decided_at IS NOT NULL AND length(trim(decision_remarks))>0))
);
CREATE INDEX employee_change_requests_employee_idx ON finbook.employee_change_requests(employee_id,created_at DESC);
CREATE INDEX employee_change_requests_status_idx ON finbook.employee_change_requests(status,created_at DESC);
CREATE TRIGGER change_requests_no_delete BEFORE DELETE OR TRUNCATE ON finbook.employee_change_requests FOR EACH STATEMENT EXECUTE FUNCTION finbook.deny_mutation();

CREATE TABLE finbook.password_reset_tokens (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 user_id uuid NOT NULL REFERENCES finbook.users(id),
 token_hash bytea NOT NULL UNIQUE CHECK(octet_length(token_hash)=32),
 expires_at timestamptz NOT NULL,
 used_at timestamptz,
 created_by uuid NOT NULL REFERENCES finbook.users(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(expires_at>created_at)
);
CREATE INDEX password_reset_user_idx ON finbook.password_reset_tokens(user_id,expires_at);

-- Reuse the append-only audit table; never create a competing audit store.
CREATE OR REPLACE FUNCTION finbook.audit_change() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,finbook AS $$
DECLARE before_row jsonb; after_row jsonb; actor text; request text;
BEGIN
 IF TG_OP<>'INSERT' THEN before_row=to_jsonb(OLD)-ARRAY['password_hash','token_hash','passport_ciphertext','account_number_ciphertext','raw_bytes','photo_bytes']; END IF;
 IF TG_OP<>'DELETE' THEN after_row=to_jsonb(NEW)-ARRAY['password_hash','token_hash','passport_ciphertext','account_number_ciphertext','raw_bytes','photo_bytes']; END IF;
 actor=nullif(current_setting('finbook.actor_id',true),''); request=nullif(current_setting('finbook.request_id',true),'');
 INSERT INTO finbook.audit_log(actor_id,request_id,event,entity_table,entity_key,old_values,new_values,reason)
 VALUES(actor::uuid,request::uuid,lower(TG_OP),TG_TABLE_NAME,coalesce(after_row->>'id',before_row->>'id',after_row->>'code',before_row->>'code',after_row::text,before_row::text),before_row,after_row,nullif(current_setting('finbook.reason',true),''));
 IF TG_OP='DELETE' THEN RETURN OLD; END IF; RETURN NEW;
END $$;
CREATE TRIGGER record_audit AFTER INSERT OR UPDATE OR DELETE ON finbook.role_history FOR EACH ROW EXECUTE FUNCTION finbook.audit_change();
CREATE TRIGGER record_audit AFTER INSERT OR UPDATE OR DELETE ON finbook.employee_change_requests FOR EACH ROW EXECUTE FUNCTION finbook.audit_change();
CREATE TRIGGER record_audit AFTER INSERT OR UPDATE OR DELETE ON finbook.password_reset_tokens FOR EACH ROW EXECUTE FUNCTION finbook.audit_change();

CREATE FUNCTION finbook.validate_employee_access() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE manager finbook.employees;
BEGIN
 IF TG_OP='INSERT' AND (NEW.official_email IS NULL OR length(trim(NEW.employee_code))=0 OR length(trim(NEW.name))=0) THEN
  RAISE EXCEPTION 'Employee code, name and official email are required' USING ERRCODE='23514';
 END IF;
 IF TG_OP='UPDATE' THEN
  IF NEW.employment_status IS NOT DISTINCT FROM OLD.employment_status AND NEW.active IS DISTINCT FROM OLD.active THEN
   NEW.employment_status=CASE WHEN NEW.active THEN 'Active' ELSE 'Inactive' END;
  END IF;
  NEW.version=OLD.version+1;
 END IF;
 NEW.active=(NEW.employment_status='Active');
 IF NEW.user_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM finbook.users WHERE id=NEW.user_id AND employee_id=NEW.id) THEN
  RAISE EXCEPTION 'Legacy user mapping must match the authoritative user employee mapping' USING ERRCODE='23514';
 END IF;
 IF NEW.reporting_to IS NOT NULL THEN
  SELECT * INTO manager FROM finbook.employees WHERE id=NEW.reporting_to FOR SHARE;
  IF manager.id IS NULL OR manager.deleted_at IS NOT NULL OR manager.employment_status<>'Active' THEN
   RAISE EXCEPTION 'Reporting manager must be an existing active employee' USING ERRCODE='23514';
  END IF;
 END IF;
 NEW.updated_at=now();
 RETURN NEW;
END $$;
CREATE TRIGGER employee_access_validation BEFORE INSERT OR UPDATE ON finbook.employees FOR EACH ROW EXECUTE FUNCTION finbook.validate_employee_access();

CREATE FUNCTION finbook.validate_user_access() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE employee finbook.employees;
BEGIN
 SELECT * INTO employee FROM finbook.employees WHERE id=NEW.employee_id FOR UPDATE;
 IF employee.id IS NULL THEN RAISE EXCEPTION 'Employee mapping required' USING ERRCODE='23514'; END IF;
 IF employee.user_id IS NOT NULL AND employee.user_id<>NEW.id THEN RAISE EXCEPTION 'Employee already has a different legacy user mapping' USING ERRCODE='23514'; END IF;
 IF TG_OP='INSERT' AND (NEW.login_email IS NULL OR length(trim(NEW.username))=0) THEN
  RAISE EXCEPTION 'Username and login email required' USING ERRCODE='23514';
 END IF;
 IF TG_OP='UPDATE' THEN
  IF NEW.employee_id IS DISTINCT FROM OLD.employee_id THEN RAISE EXCEPTION 'User employee mapping is immutable' USING ERRCODE='23514'; END IF;
  IF NEW.account_status IS NOT DISTINCT FROM OLD.account_status AND NEW.active IS DISTINCT FROM OLD.active THEN NEW.account_status=CASE WHEN NEW.active THEN 'Active' ELSE 'Inactive' END; END IF;
  NEW.version=OLD.version+1;
 END IF;
 IF (TG_OP='INSERT' OR NEW.account_status='Active') AND (employee.employment_status<>'Active' OR employee.deleted_at IS NOT NULL) THEN
  RAISE EXCEPTION 'Login access requires an active employee' USING ERRCODE='23514';
 END IF;
 NEW.active=(NEW.account_status='Active'); NEW.updated_at=now();
 RETURN NEW;
END $$;
CREATE TRIGGER user_access_validation BEFORE INSERT OR UPDATE ON finbook.users FOR EACH ROW EXECUTE FUNCTION finbook.validate_user_access();

CREATE FUNCTION finbook.suspend_employee_login() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.employment_status<>'Active' OR NEW.deleted_at IS NOT NULL THEN
  UPDATE finbook.users SET account_status='Inactive',active=false WHERE employee_id=NEW.id AND account_status<>'Inactive';
 END IF;
 -- Reactivation intentionally never restores an account automatically.
 RETURN NEW;
END $$;
CREATE TRIGGER suspend_employee_login AFTER UPDATE OF employment_status,active,deleted_at ON finbook.employees FOR EACH ROW EXECUTE FUNCTION finbook.suspend_employee_login();

CREATE FUNCTION finbook.revoke_user_sessions() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.account_status<>'Active' OR NEW.deleted_at IS NOT NULL OR NEW.password_hash IS DISTINCT FROM OLD.password_hash OR NEW.password_reset_required THEN
  UPDATE finbook.sessions SET revoked_at=now() WHERE user_id=NEW.id AND revoked_at IS NULL;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER revoke_user_sessions AFTER UPDATE ON finbook.users FOR EACH ROW EXECUTE FUNCTION finbook.revoke_user_sessions();

CREATE FUNCTION finbook.record_role_history() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE actor uuid; affected_user uuid; previous_role uuid; next_role uuid; note text;
BEGIN
 actor=nullif(current_setting('finbook.actor_id',true),'')::uuid;
 note=nullif(trim(current_setting('finbook.reason',true)),'');
 IF actor IS NULL OR note IS NULL THEN RAISE EXCEPTION 'Role changes require an authenticated actor and reason' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' AND NEW.user_id IS DISTINCT FROM OLD.user_id THEN RAISE EXCEPTION 'Remove and assign roles when changing users' USING ERRCODE='23514'; END IF;
 IF TG_OP<>'INSERT' THEN affected_user=OLD.user_id; previous_role=OLD.role_id; END IF;
 IF TG_OP<>'DELETE' THEN affected_user=NEW.user_id; next_role=NEW.role_id; END IF;
 IF TG_OP='UPDATE' AND next_role=previous_role THEN RETURN NEW; END IF;
 INSERT INTO finbook.role_history(user_id,employee_id,previous_role_id,new_role_id,previous_role_name,new_role_name,action,modified_by,remarks)
 SELECT affected_user,u.employee_id,previous_role,next_role,(SELECT name FROM finbook.roles WHERE id=previous_role),(SELECT name FROM finbook.roles WHERE id=next_role),CASE TG_OP WHEN 'INSERT' THEN 'Assigned' WHEN 'DELETE' THEN 'Removed' ELSE 'Modified' END,actor,note FROM finbook.users u WHERE u.id=affected_user;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF; RETURN NEW;
END $$;
CREATE TRIGGER user_role_history AFTER INSERT OR UPDATE OR DELETE ON finbook.user_roles FOR EACH ROW EXECUTE FUNCTION finbook.record_role_history();

CREATE FUNCTION finbook.validate_employee_change_request() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE field_name text;
BEGIN
 IF TG_OP='INSERT' AND NEW.status<>'Pending Approval' THEN
  RAISE EXCEPTION 'New requests must await approval' USING ERRCODE='23514';
 END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.status<>'Pending Approval' OR NEW.status='Pending Approval' OR
    (NEW.employee_id,NEW.requested_by,NEW.employee_version,NEW.original_values,NEW.requested_values,NEW.created_at) IS DISTINCT FROM
    (OLD.employee_id,OLD.requested_by,OLD.employee_version,OLD.original_values,OLD.requested_values,OLD.created_at) THEN
   RAISE EXCEPTION 'Only a pending request decision may be recorded' USING ERRCODE='23514';
  END IF;
  IF NEW.decided_by=NEW.requested_by THEN RAISE EXCEPTION 'Requests require another administrator to review' USING ERRCODE='23514'; END IF;
  NEW.decided_at=now();
 END IF;
 IF NOT EXISTS(SELECT 1 FROM finbook.users WHERE id=NEW.requested_by AND employee_id=NEW.employee_id) THEN
  RAISE EXCEPTION 'Request must belong to the requesting employee' USING ERRCODE='23514';
 END IF;
 FOR field_name IN SELECT jsonb_object_keys(NEW.requested_values) LOOP
  IF field_name NOT IN ('personal_email','personal_mobile_number','alternate_contact_number','address_line_1','address_line_2','city','state','country','postal_code','emergency_contact_name','emergency_contact_number') THEN
   RAISE EXCEPTION 'Field cannot be requested through My Info' USING ERRCODE='23514';
  END IF;
 END LOOP;
 NEW.updated_at=now(); RETURN NEW;
END $$;
CREATE TRIGGER employee_change_request_validation BEFORE INSERT OR UPDATE ON finbook.employee_change_requests FOR EACH ROW EXECUTE FUNCTION finbook.validate_employee_change_request();

-- The new tables are accessed through authenticated backend operations only.
REVOKE ALL ON finbook.role_history,finbook.employee_change_requests,finbook.password_reset_tokens FROM PUBLIC;
