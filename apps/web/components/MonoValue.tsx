"use client";

import { useState } from "react";

/** Values longer than this (or containing newlines) start collapsed. */
const clampLength = 160;

/**
 * Monospace value that wraps inside its column instead of widening it:
 * tokens and URLs have no spaces, so breaking anywhere is the only option.
 * Very long values are clamped to two lines with an expand toggle.
 */
export function MonoValue({
  value,
  className = "",
}: {
  value: string;
  className?: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const long = value.length > clampLength || value.includes("\n");

  return (
    <div className={`min-w-0 ${className}`}>
      <span
        className={`block whitespace-pre-wrap break-all font-mono text-sm ${
          long && !expanded ? "line-clamp-2" : ""
        }`}
      >
        {value}
      </span>
      {long ? (
        <button
          className="mt-1 text-xs text-zinc-500 transition-colors duration-150 hover:text-zinc-200"
          onClick={() => setExpanded((current) => !current)}
          type="button"
        >
          {expanded ? "Show less" : `Show all (${value.length} characters)`}
        </button>
      ) : null}
    </div>
  );
}
