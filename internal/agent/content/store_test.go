package content

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"slices"
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
			"requires":{"events":[{"behavior":"file.write","fields":["file.path"]},{"behavior":"process.exec","fields":["process.binary"]}]},
			"output":{"terminal":false},
			"suppress":{"within":"5m","by":["process.stable_id","file.path"]}
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
	if rules[0].Terminal == nil || *rules[0].Terminal {
		t.Fatalf("terminal = %v, want explicit false", rules[0].Terminal)
	}
	if rules[0].Suppression.Within != "5m" || !slices.Equal(rules[0].Suppression.By, []string{"process.stable_id", "file.path"}) {
		t.Fatalf("suppression = %+v", rules[0].Suppression)
	}
}

func TestStoreParsesConditionTree(t *testing.T) {
	store := NewStore()
	raw := `{
		"api_version":"sysarmor.content/v1",
		"kind":"rulepack",
		"metadata":{"id":"rulepack:condition-tree","version":"v1"},
		"spec":{"rulesets":[{"id":"ruleset:condition-tree","version":"v1","rules":[{
			"rule_id":"neutral_boolean_rule","version":1,"severity":"medium",
			"runtime":{"type":"expr","expr":{"condition_group":{"all":[
				{"any":[
					{"condition":{"field":"process.binary_name","op":"in","ref":"ctx:test-tools"}},
					{"condition":{"field":"process.argv","op":"contains","ref":"ctx:test-markers"}}
				]},
				{"not":{"condition":{"field":"socket.port","op":"in","values":["80"]}}}
			]}}},
			"requires":{"events":[{"behavior":"network.connect","fields":["process.binary","process.argv","socket.port"]}]}
		}]}]}
	}`
	if _, err := store.Apply(raw, true, false); err != nil {
		t.Fatal(err)
	}
	rules := store.Snapshot().Rules
	if len(rules) != 1 {
		t.Fatalf("rules = %+v", rules)
	}
	group := rules[0].Expr.ConditionGroup
	if group == nil || len(group.All) != 2 || len(group.All[0].Any) != 2 || group.All[1].Not == nil {
		t.Fatalf("condition group = %+v", group)
	}
	if got := group.All[0].Any[1].Condition; got == nil || got.Field != "process.argv" || got.Ref != "ctx:test-markers" {
		t.Fatalf("condition leaf = %+v", got)
	}
}

func TestStoreParsesCorrelateRule(t *testing.T) {
	store := NewStore()
	raw := `{
		"api_version":"sysarmor.content/v1","kind":"rulepack",
		"metadata":{"id":"rulepack:correlate","version":"v1"},
		"spec":{"rulesets":[{"id":"ruleset:correlate","version":"v1","rules":[{
			"rule_id":"neutral_correlate_rule","version":1,"severity":"high",
			"runtime":{"type":"correlate","correlate":{
				"within":"2m","by":["lineage_id"],"facts":[
					{"id":"change","events":["file.write","file.chmod"],"conditions":[{"field":"file.path","op":"prefix","value":"/tmp/test/"}]},
					{"id":"run","event":"process.exec","condition_group":{"any":[
						{"condition":{"field":"process.binary","op":"prefix","value":"/tmp/test/"}},
						{"condition":{"field":"process.argv","op":"contains","value":"/tmp/test/"}}
					]}}
				]
			}},
			"requires":{"events":[{"behavior":"file.write","fields":["file.path"]},{"behavior":"process.exec","fields":["process.binary"]}]}
		}]}]}
	}`
	if _, err := store.Apply(raw, true, false); err != nil {
		t.Fatal(err)
	}
	rules := store.Snapshot().Rules
	if len(rules) != 1 {
		t.Fatalf("rules = %+v", rules)
	}
	correlate := rules[0].Correlate
	if correlate.Within != "2m" || !slices.Equal(correlate.By, []string{"lineage_id"}) || len(correlate.Facts) != 2 {
		t.Fatalf("correlate = %+v", correlate)
	}
	if !slices.Equal(correlate.Facts[0].Events, []string{"file.write", "file.chmod"}) || correlate.Facts[1].Event != "process.exec" || correlate.Facts[1].ConditionGroup == nil {
		t.Fatalf("facts = %+v", correlate.Facts)
	}
}
