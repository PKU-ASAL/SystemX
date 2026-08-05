CREATE TABLE IF NOT EXISTS schema_meta (
  version INTEGER PRIMARY KEY
);
CREATE TABLE IF NOT EXISTS device_identity (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  device_id TEXT NOT NULL UNIQUE,
  host_id TEXT NOT NULL,
  created_at_ns INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS enrollment (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  state TEXT NOT NULL CHECK (state IN ('standalone', 'managed')),
  tenant_id TEXT,
  agent_id TEXT,
  gateway_address TEXT,
  tls_ca_path TEXT,
  tls_cert_path TEXT,
  tls_key_path TEXT,
  tls_server_name TEXT,
  upload_history INTEGER NOT NULL DEFAULT 0,
  managed_from_seq INTEGER,
  updated_at_ns INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS policy (
  kind TEXT PRIMARY KEY,
  version INTEGER NOT NULL,
  document_json BLOB NOT NULL,
  digest TEXT NOT NULL,
  updated_at_ns INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS signals (
  sequence INTEGER PRIMARY KEY,
  signal_id TEXT NOT NULL UNIQUE,
  observed_at_ns INTEGER NOT NULL,
  rule_id TEXT NOT NULL,
  severity TEXT NOT NULL,
  payload BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS signals_observed_at ON signals(observed_at_ns);
CREATE INDEX IF NOT EXISTS signals_rule_time ON signals(rule_id, observed_at_ns);
CREATE INDEX IF NOT EXISTS signals_severity_time ON signals(severity, observed_at_ns);
CREATE TABLE IF NOT EXISTS segments (
  segment_id INTEGER PRIMARY KEY,
  path TEXT NOT NULL UNIQUE,
  state TEXT NOT NULL CHECK (state IN ('open', 'sealed')),
  first_sequence INTEGER NOT NULL,
  last_sequence INTEGER NOT NULL,
  record_count INTEGER NOT NULL,
  bytes INTEGER NOT NULL,
  created_at_ns INTEGER NOT NULL,
  sealed_at_ns INTEGER
);
CREATE TABLE IF NOT EXISTS upload_checkpoint (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  segment_id INTEGER,
  record_offset INTEGER NOT NULL,
  last_batch_id TEXT,
  updated_at_ns INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS runtime_counters (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  dropped_batches_storage INTEGER NOT NULL DEFAULT 0,
  dropped_events_storage INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS sequence_cursor (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  event_sequence INTEGER NOT NULL DEFAULT 0,
  signal_sequence INTEGER NOT NULL DEFAULT 0
);
INSERT OR IGNORE INTO sequence_cursor(singleton,event_sequence,signal_sequence) VALUES (1,0,0);

INSERT INTO schema_meta(version) VALUES (1);
INSERT INTO device_identity(singleton,device_id,host_id,created_at_ns)
VALUES (1,'rc5-device-fixture','rc5-host-fixture',1);
INSERT INTO enrollment(singleton,state,tenant_id,agent_id,gateway_address,tls_ca_path,
  tls_cert_path,tls_key_path,tls_server_name,upload_history,managed_from_seq,updated_at_ns)
VALUES (1,'managed','default','rc5-agent-fixture','10.66.0.10:9444',
  '/fixture/ca.pem','/fixture/agent.pem','/fixture/agent-key.pem',
  'sysarmor-gateway.local',0,1,1);
INSERT INTO policy(kind,version,document_json,digest,updated_at_ns)
VALUES ('endpoint',1,
  '{"policy_id":"rc5-managed-policy","version":1,"collection":{"behaviors":["process.exec","process.exit","process.fork","file.read","file.write","network.connect"],"observe_only":true},"detection":{},"telemetry":{},"response":{}}',
  'f6a449a61f8e28829a0026aec0da60bda27db4eff18321faadf323b5657601b4',1);
