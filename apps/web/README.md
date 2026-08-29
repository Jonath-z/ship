# apps/web

The Ship dashboard: Next.js App Router + React + TypeScript + Tailwind.

## Directory rules

| Folder | Holds | Does not hold |
|---|---|---|
| `app/` | Routes and layouts only. Thin — a page composes a feature. | Business logic, data fetching logic |
| `features/` | Domain UI: `projects/`, `infrastructure/`, `deployments/`, `servers/`, `logs/`. Components, hooks, and state for one slice of the product. | Generic primitives |
| `components/` | Generic primitives that know nothing about Ship's domain. | Anything named after a domain entity |
| `hooks/` | Cross-feature hooks (auth, SSE subscription, permissions). | Feature-specific hooks — those live in `features/` |
| `lib/` | Client construction, formatters, constants. | Rendering |
| `styles/` | Tailwind entry and design tokens. | — |

The single rule worth defending: **domain UI goes in `features/`, not in one giant `components/` directory** (spec §41).

## Hard constraints

- All data access goes through `@ship/api-client`. No ad-hoc `fetch` to the API.
- The frontend never generates or edits Kamal YAML. It mutates the desired-state model; the backend renders (spec §18).
- Dragging a node on the canvas changes *configuration*, not infrastructure. Nothing reaches a server until Deploy is pressed (spec §36).
