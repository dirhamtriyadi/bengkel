-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION protect_posted_journal_lines() RETURNS trigger AS $$
DECLARE
    target_id uuid;
    journal_status varchar(20);
BEGIN
    target_id := COALESCE(NEW.journal_entry_id, OLD.journal_entry_id);
    SELECT status INTO journal_status FROM journal_entries WHERE id = target_id;
    IF journal_status = 'posted' THEN
        RAISE EXCEPTION 'posted journal lines are immutable; create a reversal instead';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER posted_journal_lines_immutable
BEFORE INSERT OR UPDATE OR DELETE ON journal_lines
FOR EACH ROW EXECUTE FUNCTION protect_posted_journal_lines();

-- +goose Down
DROP TRIGGER IF EXISTS posted_journal_lines_immutable ON journal_lines;
DROP FUNCTION IF EXISTS protect_posted_journal_lines();
