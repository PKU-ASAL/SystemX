package content

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestStorePersistsLoadsAndSnapshotsValueSets(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStoreWithOptions(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	raw := `{
		"api_version":"sysarmor.content/v1",
		"kind":"iocpack",
		"metadata":{"id":"ioc:c2-control-port-feed","version":"v1"},
		"spec":{"value_type":"port","values":["9443","443","9443"]}
	}`
	if _, err := store.Apply(raw, true, false); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewStoreWithOptions(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := loaded.Snapshot()
	set := snapshot.IOCPacks["ioc:c2-control-port-feed"]
	if set.Version != "v1" || len(set.Values) != 2 || set.Values[0] != "443" || set.Values[1] != "9443" {
		t.Fatalf("set = %+v", set)
	}
}

func TestStoreVerifiesEd25519Signature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	env := Envelope{
		APIVersion: "sysarmor.content/v1",
		Kind:       "contextset",
		Metadata:   Metadata{ID: "ctx:payload-path-prefixes", Version: "signed-v1"},
		Spec:       json.RawMessage(`{"value_type":"path_prefix","values":["/opt/drop/"]}`),
	}
	env.Integrity = Integrity{
		DigestAlg:    "sha256",
		Digest:       signedDigest(env),
		SignatureAlg: "ed25519",
		KeyID:        "local",
		Signature:    base64.StdEncoding.EncodeToString(ed25519.Sign(priv, signedBytes(env))),
	}
	rawData, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	store.trustedKeys = map[string]ed25519.PublicKey{"local": pub}
	if _, err := store.Apply(string(rawData), false, false); err != nil {
		t.Fatal(err)
	}
	store.trustedKeys = map[string]ed25519.PublicKey{"other": pub}
	if _, err := store.Apply(string(rawData), false, false); err == nil {
		t.Fatal("Apply() with untrusted key error = nil")
	}
}

func TestStoreAppliesPatch(t *testing.T) {
	store := NewStore()
	base := `{
		"api_version":"sysarmor.content/v1",
		"kind":"iocpack",
		"metadata":{"id":"ioc:c2-control-port-feed","version":"v1"},
		"spec":{"value_type":"port","values":["443"]}
	}`
	if _, err := store.Apply(base, true, false); err != nil {
		t.Fatal(err)
	}
	patch := `{
		"api_version":"sysarmor.content/v1",
		"kind":"iocpack",
		"metadata":{"id":"ioc:c2-control-port-feed","version":"v2"},
		"spec":{"base_version":"v1","value_type":"port","merge_strategy":"patch","ops":[{"op":"add","value":"9443"},{"op":"remove","value":"443"}]}
	}`
	if _, err := store.Apply(patch, true, false); err != nil {
		t.Fatal(err)
	}
	set := store.Snapshot().IOCPacks["ioc:c2-control-port-feed"]
	if set.Version != "v2" || len(set.Values) != 1 || set.Values[0] != "9443" {
		t.Fatalf("patched set = %+v", set)
	}
}

func TestStoreParsesRulePack(t *testing.T) {
	store := NewStore()
	raw := `{
		"api_version":"sysarmor.content/v1",
		"kind":"rulepack",
		"metadata":{"id":"rulepack:test","version":"v1"},
		"spec":{"rulesets":[{"id":"ruleset:test","version":"v1","rules":[{"rule_id":"reverse_shell_pattern","version":7,"severity":"critical","runtime":{"type":"builtin","entrypoint":"builtin.reverse_shell_pattern"},"requires":{"events":[{"behavior":"network.connect","fields":["socket.port"]}],"ioc":{"optional":["ioc:c2-control-port-feed"]}},"output":{"response_intent":{"action":"collect_evidence","confidence":91}}}]}]}
	}`
	if _, err := store.Apply(raw, true, false); err != nil {
		t.Fatal(err)
	}
	rules := store.Snapshot().Rules
	if len(rules) != 1 || rules[0].RuleSetRef != "ruleset:test" || rules[0].Version != 7 || rules[0].ResponseIntent.Confidence != 91 {
		t.Fatalf("rules = %+v", rules)
	}
}

func TestStoreParsesCEPRulePack(t *testing.T) {
	store := NewStore()
	raw := `{
		"api_version":"sysarmor.content/v1",
		"kind":"rulepack",
		"metadata":{"id":"rulepack:cep","version":"v1"},
		"spec":{"rulesets":[{"id":"ruleset:cep","version":"v1","rules":[{
			"rule_id":"cep_payload_lifecycle",
			"version":2,
			"severity":"critical",
			"runtime":{
				"type":"sequence",
				"sequence":{
					"within":"60s",
					"by":["lineage_id"],
					"steps":[
						{"id":"drop","event":"file.write","conditions":[{"field":"file.path","op":"prefix","value":"/dev/shm/"}]},
						{"id":"chmod","event":"file.chmod","conditions":[{"field":"file.path","op":"same_as","step":"drop"}]},
						{"id":"exec","event":"process.exec","conditions":[{"field":"process.binary","op":"same_as","step":"drop","step_field":"file.path"}]}
					]
				}
			},
			"requires":{"events":[{"behavior":"file.write","fields":["file.path"]},{"behavior":"process.exec","fields":["process.binary"]}]}
		}]}]}
	}`
	if _, err := store.Apply(raw, true, false); err != nil {
		t.Fatal(err)
	}
	rules := store.Snapshot().Rules
	if len(rules) != 1 || rules[0].RuntimeType != "sequence" || rules[0].Sequence.Within != "60s" || len(rules[0].Sequence.Steps) != 3 {
		t.Fatalf("rules = %+v", rules)
	}
	if rules[0].Sequence.Steps[1].Conditions[0].Step != "drop" {
		t.Fatalf("sequence condition = %+v", rules[0].Sequence.Steps[1].Conditions[0])
	}
}
