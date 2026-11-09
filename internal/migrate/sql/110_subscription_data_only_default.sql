-- +goose Up
-- A subscription created without dataOnly is delivered the envelope.
--
-- The documented default is the envelope (dataOnly false), and it is what an
-- SDK sync that leaves dataOnly out has always produced; a create through the
-- API or the console defaulted to data-only. Both now default to the envelope
-- (owner ruling 2026-11-09). The platform always writes the column, so this
-- changes only what a raw INSERT gets. Existing rows are not touched: their
-- deliveries keep the shape they have.
--
-- msg_subscriptions is a small configuration table: a plain statement.
ALTER TABLE msg_subscriptions ALTER COLUMN data_only SET DEFAULT FALSE;

-- +goose Down
ALTER TABLE msg_subscriptions ALTER COLUMN data_only SET DEFAULT TRUE;
