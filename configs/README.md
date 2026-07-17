# Product Content

`configs/` contains reference policy and rule metadata. The current runtime
does not discover or load this directory automatically; executable detection
behavior remains in Go code and explicitly applied content packs.

```text
policies/default-edr-policy.json   default policy metadata
rules/endpoint/                    endpoint detection content
rules/cloud/                       platform correlation content
```

Wire contracts belong in `api/proto/`; Agent runtime defaults belong in
`deployments/agent/`; executable test content belongs in `test/data/`. Do not
treat these files as deployed policy without an explicit loader or apply path.
