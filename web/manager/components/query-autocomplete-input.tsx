"use client";

import { useState, type KeyboardEvent } from "react";
import { SearchIcon, XIcon } from "lucide-react";

import { Input } from "@/components/ui/input";
import {
  completeQueryField,
  getQueryFieldSuggestions,
} from "@/lib/query-autocomplete";
import type { SearchField } from "@/lib/opensearch-fields";
import { cn } from "@/lib/utils";

type QueryToken = {
  key: string;
  value: string;
  onRemove: () => void;
};

export function QueryAutocompleteInput({
  query,
  fields,
  tokens = [],
  placeholder,
  ariaLabel,
  onQueryChange,
}: {
  query: string;
  fields: SearchField[];
  tokens?: QueryToken[];
  placeholder: string;
  ariaLabel: string;
  onQueryChange: (value: string) => void;
}) {
  const [activeSuggestionIndex, setActiveSuggestionIndex] = useState(0);
  const suggestions = getQueryFieldSuggestions(query, fields);

  function completeField(index = activeSuggestionIndex) {
    const field = suggestions[index];

    if (!field) return;

    onQueryChange(completeQueryField(query, field.name));
    setActiveSuggestionIndex(0);
  }

  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (!suggestions.length) return;

    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActiveSuggestionIndex((current) => (current + 1) % suggestions.length);
      return;
    }

    if (event.key === "ArrowUp") {
      event.preventDefault();
      setActiveSuggestionIndex((current) => (current - 1 + suggestions.length) % suggestions.length);
      return;
    }

    if (event.key === "Tab" || event.key === "Enter") {
      event.preventDefault();
      completeField();
    }
  }

  return (
    <div className="relative flex min-h-11 min-w-72 flex-1 items-center gap-2 border-x bg-bg px-3 max-lg:min-w-full max-lg:rounded-lg max-lg:border">
      <SearchIcon className="size-4 shrink-0 text-muted-fg" />
      {tokens.map((token) => (
        <span
          key={token.key}
          className="inline-flex h-7 shrink-0 items-center gap-1 rounded-md border bg-muted px-2 font-mono text-xs"
        >
          <span className="font-semibold text-muted-fg">{token.key}:</span>
          <span>{token.value}</span>
          <button
            aria-label={`移除 ${token.key}:${token.value}`}
            className="ml-1 rounded text-muted-fg hover:text-fg"
            type="button"
            onClick={token.onRemove}
          >
            <XIcon className="size-3" />
          </button>
        </span>
      ))}
      <Input
        className="h-10 min-w-40 flex-1 rounded-none border-0 bg-transparent px-0 font-mono focus-visible:ring-0"
        value={query}
        onChange={(event) => {
          setActiveSuggestionIndex(0);
          onQueryChange(event.target.value);
        }}
        onKeyDown={handleKeyDown}
        placeholder={tokens.length ? "Add query..." : placeholder}
        aria-label={ariaLabel}
        aria-autocomplete="list"
        aria-expanded={suggestions.length > 0}
        aria-controls={`${ariaLabel.replace(/\W+/g, "-").toLowerCase()}-suggestions`}
      />
      {suggestions.length > 0 && (
        <div
          className="absolute left-10 right-3 top-[calc(100%+0.375rem)] z-20 overflow-hidden rounded-lg border bg-popover text-popover-foreground shadow-lg"
          id={`${ariaLabel.replace(/\W+/g, "-").toLowerCase()}-suggestions`}
          role="listbox"
        >
          <div className="border-b px-3 py-2 text-xs text-muted-fg">
            Press Tab to insert field, then type a value
          </div>
          {suggestions.map((field, index) => (
            <button
              key={field.name}
              className={cn(
                "flex w-full items-center justify-between gap-3 px-3 py-2 text-left font-mono text-sm",
                index === activeSuggestionIndex ? "bg-accent text-accent-fg" : "hover:bg-muted",
              )}
              role="option"
              type="button"
              aria-selected={index === activeSuggestionIndex}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => completeField(index)}
            >
              <span>{field.name}:</span>
              <span className="truncate text-xs text-muted-fg">{field.example ?? field.type}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
