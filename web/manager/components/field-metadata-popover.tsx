"use client";

import { DatabaseIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent } from "@/components/ui/popover";
import type { SearchField } from "@/lib/opensearch-fields";

export function FieldMetadataPopover({ fields }: { fields: SearchField[] }) {
  return (
    <Popover>
      <Button className="h-7 gap-1 px-2 font-normal" intent="outline" size="xs">
        <DatabaseIcon />
        {fields.length} searchable fields
      </Button>
      <PopoverContent className="max-w-none [--trigger-width:28rem]" placement="bottom start">
        <div className="w-[440px] max-w-[calc(100vw-2rem)]">
          <div className="border-b px-3 py-2">
            <div className="text-sm font-semibold">Index fields</div>
            <div className="text-xs text-muted-fg">From OpenSearch field capabilities</div>
          </div>
          <div className="max-h-80 overflow-auto">
            {fields.map((field) => (
              <div
                key={field.name}
                className="grid grid-cols-[minmax(0,1fr)_90px_92px] items-center gap-2 border-b px-3 py-2 last:border-b-0"
              >
                <div className="min-w-0 font-mono text-xs">
                  <div className="truncate">{field.name}</div>
                  {field.example && (
                    <div className="mt-0.5 truncate text-muted-fg">e.g. {field.example}</div>
                  )}
                </div>
                <Badge className="justify-center bg-bg text-muted-fg">{field.type}</Badge>
                <Badge className="justify-center bg-bg text-muted-fg">
                  {field.aggregatable ? "agg" : "search"}
                </Badge>
              </div>
            ))}
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
