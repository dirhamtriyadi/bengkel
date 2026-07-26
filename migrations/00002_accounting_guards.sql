-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION ensure_balanced_journal() RETURNS trigger AS $$
DECLARE
    target_id uuid;
    total_debit bigint;
    total_credit bigint;
BEGIN
    target_id := COALESCE(NEW.journal_entry_id, OLD.journal_entry_id);
    SELECT COALESCE(SUM(debit),0), COALESCE(SUM(credit),0)
      INTO total_debit, total_credit
      FROM journal_lines WHERE journal_entry_id = target_id AND deleted_at IS NULL;
    IF total_debit <> total_credit THEN
        RAISE EXCEPTION 'journal entry % is not balanced: debit %, credit %', target_id, total_debit, total_credit;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER journal_lines_must_balance
AFTER INSERT OR UPDATE OR DELETE ON journal_lines
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION ensure_balanced_journal();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION protect_audit_log() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_logs are immutable';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER audit_logs_immutable
BEFORE UPDATE OR DELETE ON audit_logs
FOR EACH ROW EXECUTE FUNCTION protect_audit_log();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION protect_posted_journal() RETURNS trigger AS $$
BEGIN
    IF OLD.status = 'posted' THEN
        RAISE EXCEPTION 'posted journal entries are immutable; create a reversal instead';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER posted_journal_immutable
BEFORE UPDATE OR DELETE ON journal_entries
FOR EACH ROW EXECUTE FUNCTION protect_posted_journal();

-- +goose Down
DROP TRIGGER IF EXISTS posted_journal_immutable ON journal_entries;
DROP FUNCTION IF EXISTS protect_posted_journal();
DROP TRIGGER IF EXISTS audit_logs_immutable ON audit_logs;
DROP FUNCTION IF EXISTS protect_audit_log();
DROP TRIGGER IF EXISTS journal_lines_must_balance ON journal_lines;
DROP FUNCTION IF EXISTS ensure_balanced_journal();
