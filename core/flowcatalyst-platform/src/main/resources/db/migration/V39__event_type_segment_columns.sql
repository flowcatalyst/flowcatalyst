-- Add denormalized segment columns to event_types for type-safe ORM queries.
-- The composite 'code' column remains the source of truth and unique constraint.

ALTER TABLE event_types ADD COLUMN application VARCHAR(100);
ALTER TABLE event_types ADD COLUMN subdomain VARCHAR(100);
ALTER TABLE event_types ADD COLUMN aggregate VARCHAR(100);

UPDATE event_types SET
  application = split_part(code, ':', 1),
  subdomain = split_part(code, ':', 2),
  aggregate = split_part(code, ':', 3)
WHERE code LIKE '%:%';

ALTER TABLE event_types ALTER COLUMN application SET NOT NULL;
ALTER TABLE event_types ALTER COLUMN subdomain SET NOT NULL;
ALTER TABLE event_types ALTER COLUMN aggregate SET NOT NULL;

CREATE INDEX idx_event_types_application ON event_types(application);
CREATE INDEX idx_event_types_subdomain ON event_types(subdomain);
CREATE INDEX idx_event_types_aggregate ON event_types(aggregate);
