import { describe, expect, it } from "vitest";

import {
  getSearchFieldsForIndexPattern,
  normalizeFieldCapabilities,
} from "./opensearch-fields";

describe("OpenSearch field metadata", () => {
  it("normalizes field capabilities into searchable UI fields", () => {
    const fields = normalizeFieldCapabilities({
      fields: {
        "host.name": {
          keyword: {
            type: "keyword",
            searchable: true,
            aggregatable: true,
          },
        },
        "event.duration": {
          long: {
            type: "long",
            searchable: true,
            aggregatable: false,
          },
        },
        "event.conflict": {
          keyword: {
            type: "keyword",
            searchable: true,
            aggregatable: true,
          },
          long: {
            type: "long",
            searchable: false,
            aggregatable: true,
          },
        },
      },
    });

    expect(fields).toEqual([
      {
        name: "event.conflict",
        type: "conflict",
        searchable: true,
        aggregatable: true,
        conflict: true,
      },
      {
        name: "event.duration",
        type: "long",
        searchable: true,
        aggregatable: false,
        conflict: false,
      },
      {
        name: "host.name",
        type: "keyword",
        searchable: true,
        aggregatable: true,
        conflict: false,
        example: "oa-web",
      },
    ]);
  });

  it("returns index pattern fields for events and incidents", () => {
    expect(getSearchFieldsForIndexPattern("events-*,signals-*").map((field) => field.name)).toContain(
      "event.kind",
    );
    expect(getSearchFieldsForIndexPattern("incidents-*").map((field) => field.name)).toContain(
      "incident.chain_id",
    );
  });
});
