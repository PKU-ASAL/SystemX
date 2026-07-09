import type { SearchField } from "./opensearch-fields";

export function getQueryFieldSuggestions(value: string, fields: SearchField[]) {
  const token = getCurrentQueryToken(value).trim();

  if (!token || token.includes(":")) {
    return [];
  }

  const normalizedToken = token.toLowerCase();

  return fields.filter((field) => field.name.toLowerCase().startsWith(normalizedToken));
}

export function completeQueryField(value: string, fieldName: string) {
  const token = getCurrentQueryToken(value);

  if (token.includes(":")) {
    return value;
  }

  const prefix = value.slice(0, value.length - token.length);

  return `${prefix}${fieldName}:`;
}

function getCurrentQueryToken(value: string) {
  const match = value.match(/(?:^|\s)(\S*)$/);

  return match?.[1] ?? "";
}
