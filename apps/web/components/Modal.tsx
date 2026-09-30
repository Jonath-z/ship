"use client";

import { useEffect, type ReactNode } from "react";

/** Centered overlay dialog matching the dark zinc house style. */
export function Modal({
  title,
  onClose,
  children,
  wide = false,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
}) {
  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div
      aria-modal
      className="fixed inset-0 z-50 grid place-items-center overflow-y-auto bg-black/70 p-4 backdrop-blur-sm"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
      role="dialog"
    >
      <section
        className={`w-full ${wide ? "max-w-3xl" : "max-w-lg"} rounded-xl border border-zinc-800 bg-zinc-950 p-6 shadow-2xl shadow-black/60`}
      >
        <div className="flex items-start justify-between gap-4">
          <h2 className="text-lg font-semibold text-zinc-50">{title}</h2>
          <button
            aria-label="Close"
            className="rounded p-1 text-zinc-500 hover:text-zinc-200"
            onClick={onClose}
            type="button"
          >
            ✕
          </button>
        </div>
        <div className="mt-4">{children}</div>
      </section>
    </div>
  );
}
