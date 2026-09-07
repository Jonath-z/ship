"use client";

/**
 * Remembers the most recently visited project/environment so globally scoped
 * screens (e.g. /servers) can link back into the environment shell.
 */

const storageKey = "ship.lastEnvironment";

export interface LastEnvironment {
  projectId: string;
  environmentId: string;
}

export function saveLastEnvironment(selection: LastEnvironment) {
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(selection));
  } catch {
    // Storage may be unavailable (private mode); selection stays URL-only.
  }
}

export function loadLastEnvironment(): LastEnvironment | undefined {
  try {
    const raw = window.localStorage.getItem(storageKey);
    if (!raw) return undefined;
    const parsed = JSON.parse(raw) as Partial<LastEnvironment>;
    if (!parsed.projectId || !parsed.environmentId) return undefined;
    return { projectId: parsed.projectId, environmentId: parsed.environmentId };
  } catch {
    return undefined;
  }
}
