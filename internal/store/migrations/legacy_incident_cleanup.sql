-- Operator-run cleanup after OpenSearch Incident Report migration is verified.
-- Runtime migrations intentionally never execute this file.
BEGIN;
DROP TABLE IF EXISTS evidence;
DROP TABLE IF EXISTS incident_events;
DROP TABLE IF EXISTS incidents;
COMMIT;
