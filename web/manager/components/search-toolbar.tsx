import type { ReactNode } from "react";

export const searchToolbarInlineSelectClass =
  "flex w-[220px] min-w-0 items-center gap-2 bg-bg px-3 max-lg:w-full max-lg:rounded-lg max-lg:border";

export const searchToolbarPlainButtonClass =
  "h-11 rounded-none border-0 px-3 max-lg:w-full max-lg:rounded-lg max-lg:border";

export const searchToolbarRunButtonClass =
  "h-11 rounded-none border-y-0 border-r-0 border-l px-4 max-lg:w-full max-lg:rounded-lg max-lg:border";

export function SearchToolbar({
  controls,
  meta,
}: {
  controls: ReactNode;
  meta: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex min-h-11 items-stretch overflow-visible rounded-lg border bg-bg max-lg:flex-wrap max-lg:border-0 max-lg:bg-transparent">
        {controls}
      </div>
      <div className="flex flex-wrap items-center gap-2">{meta}</div>
    </div>
  );
}
