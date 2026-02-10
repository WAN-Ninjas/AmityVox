-- Migration 004: Add cached member_count column to guilds for performance.

ALTER TABLE guilds ADD COLUMN member_count INT NOT NULL DEFAULT 0;

-- Backfill existing counts.
UPDATE guilds SET member_count = (SELECT COUNT(*) FROM guild_members WHERE guild_members.guild_id = guilds.id);

-- Trigger to auto-increment on member join.
CREATE OR REPLACE FUNCTION guild_member_count_inc() RETURNS TRIGGER AS $$
BEGIN
    UPDATE guilds SET member_count = member_count + 1 WHERE id = NEW.guild_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_guild_member_count_inc
    AFTER INSERT ON guild_members
    FOR EACH ROW EXECUTE FUNCTION guild_member_count_inc();

-- Trigger to auto-decrement on member leave.
CREATE OR REPLACE FUNCTION guild_member_count_dec() RETURNS TRIGGER AS $$
BEGIN
    UPDATE guilds SET member_count = GREATEST(member_count - 1, 0) WHERE id = OLD.guild_id;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_guild_member_count_dec
    AFTER DELETE ON guild_members
    FOR EACH ROW EXECUTE FUNCTION guild_member_count_dec();
