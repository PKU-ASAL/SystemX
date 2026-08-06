export type OpenSearchFieldCapability = {
  type: string;
  searchable: boolean;
  aggregatable: boolean;
};

export type OpenSearchFieldCapabilitiesResponse = {
  fields: Record<string, Record<string, OpenSearchFieldCapability>>;
};

export type SearchField = {
  name: string;
  type: string;
  searchable: boolean;
  aggregatable: boolean;
  conflict: boolean;
  example?: string;
};

const fieldExamples: Record<string, string> = {
  "agent.id": "node-a",
  "agent.registered_at": "2026-07-07",
  "agent.version": "0.8.0",
  "event.action": "process_exec",
  "event.kind": "signal",
  "event.severity": "critical",
  "event.tactic": "credential-access",
  "host.name": "oa-web",
  "incident.chain_id": "threat-chain-001",
  "process.name": "bash",
  "policy.name": "default-edr-policy",
  root_cause: "credential",
  severity: "critical",
  status: "active",
  "user.name": "www-data",
};

const mockFieldCapsByPattern: Record<string, OpenSearchFieldCapabilitiesResponse> = {
  "agents-*": {
    fields: {
      "agent.id": fieldCaps("keyword"),
      "agent.registered_at": fieldCaps("date"),
      "agent.version": fieldCaps("keyword"),
      "host.name": fieldCaps("keyword"),
      "policy.name": fieldCaps("keyword"),
      status: fieldCaps("keyword"),
    },
  },
  "events-*,signals-*": {
    fields: {
      "@timestamp": fieldCaps("date"),
      "agent.id": fieldCaps("keyword"),
      "event.action": fieldCaps("keyword"),
      "event.kind": fieldCaps("keyword"),
      "event.severity": fieldCaps("keyword"),
      "event.summary": fieldCaps("text", true, false),
      "event.tactic": fieldCaps("keyword"),
      "host.name": fieldCaps("keyword"),
      "process.name": fieldCaps("keyword"),
      "user.name": fieldCaps("keyword"),
    },
  },
  "events-*": {
    fields: {
      "@timestamp": fieldCaps("date"),
      "agent.id": fieldCaps("keyword"),
      "event.action": fieldCaps("keyword"),
      "event.kind": fieldCaps("keyword"),
      "event.summary": fieldCaps("text", true, false),
      "host.name": fieldCaps("keyword"),
      "process.name": fieldCaps("keyword"),
    },
  },
  "signals-*": {
    fields: {
      "@timestamp": fieldCaps("date"),
      "agent.id": fieldCaps("keyword"),
      "event.kind": fieldCaps("keyword"),
      "event.severity": fieldCaps("keyword"),
      "event.tactic": fieldCaps("keyword"),
      "host.name": fieldCaps("keyword"),
      "user.name": fieldCaps("keyword"),
    },
  },
  "incidents-*": {
    fields: {
      "@timestamp": fieldCaps("date"),
      "host.name": fieldCaps("keyword"),
      "incident.chain_id": fieldCaps("keyword"),
      root_cause: fieldCaps("keyword"),
      severity: fieldCaps("keyword"),
      status: fieldCaps("keyword"),
    },
  },
  "incident-*": {
    fields: {
      "@timestamp": fieldCaps("date"),
      "host.name": fieldCaps("keyword"),
      "incident.chain_id": fieldCaps("keyword"),
      root_cause: fieldCaps("keyword"),
      severity: fieldCaps("keyword"),
      status: fieldCaps("keyword"),
    },
  },
};

export function normalizeFieldCapabilities(response: OpenSearchFieldCapabilitiesResponse): SearchField[] {
  return Object.entries(response.fields)
    .map(([name, capabilitiesByType]) => {
      const capabilities = Object.values(capabilitiesByType);
      const types = new Set(capabilities.map((capability) => capability.type));
      const conflict = types.size > 1;

      const field = {
        name,
        type: conflict ? "conflict" : capabilities[0]?.type ?? "unknown",
        searchable: capabilities.some((capability) => capability.searchable),
        aggregatable: capabilities.some((capability) => capability.aggregatable),
        conflict,
      };

      if (!fieldExamples[name]) {
        return field;
      }

      return { ...field, example: fieldExamples[name] };
    })
    .sort((left, right) => left.name.localeCompare(right.name));
}

export function getSearchFieldsForIndexPattern(indexPattern: string) {
  const response = mockFieldCapsByPattern[indexPattern] ?? { fields: {} };

  return normalizeFieldCapabilities(response).filter((field) => field.searchable && !field.conflict);
}

function fieldCaps(type: string, searchable = true, aggregatable = true) {
  return {
    [type]: {
      type,
      searchable,
      aggregatable,
    },
  };
}
