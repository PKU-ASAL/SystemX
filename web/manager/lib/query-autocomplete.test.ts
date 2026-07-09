import { describe, expect, it } from "vitest";

import { completeQueryField, getQueryFieldSuggestions } from "./query-autocomplete";
import { getSearchFieldsForIndexPattern } from "./opensearch-fields";

const incidentFields = getSearchFieldsForIndexPattern("incidents-*");

describe("query field autocomplete", () => {
  it("suggests fields from the current token prefix", () => {
    expect(getQueryFieldSuggestions("inc", incidentFields).map((field) => field.name)).toEqual([
      "incident.chain_id",
    ]);
    expect(getQueryFieldSuggestions("host.name:oa sev", incidentFields).map((field) => field.name)).toEqual([
      "severity",
    ]);
  });

  it("completes the selected field and keeps earlier query text", () => {
    expect(completeQueryField("host.name:oa sev", "severity")).toBe("host.name:oa severity:");
  });

  it("does not suggest fields when the current token already has a value separator", () => {
    expect(getQueryFieldSuggestions("severity:cri", incidentFields)).toEqual([]);
  });
});
