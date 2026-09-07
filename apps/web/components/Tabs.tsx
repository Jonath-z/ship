"use client";

export interface TabItem<T extends string> {
  id: T;
  label: string;
}

/** Horizontal tab strip; controlled by the parent. */
export function Tabs<T extends string>({
  tabs,
  active,
  onChange,
}: {
  tabs: TabItem<T>[];
  active: T;
  onChange: (id: T) => void;
}) {
  return (
    <div className="flex gap-1 border-b border-zinc-800" role="tablist">
      {tabs.map((tab) => (
        <button
          aria-selected={tab.id === active}
          className={`-mb-px border-b-2 px-4 py-2 text-sm ${
            tab.id === active
              ? "border-emerald-400 font-medium text-zinc-100"
              : "border-transparent text-zinc-400 hover:text-zinc-200"
          }`}
          key={tab.id}
          onClick={() => onChange(tab.id)}
          role="tab"
          type="button"
        >
          {tab.label}
        </button>
      ))}
    </div>
  );
}
