# Frontend Guidelines

## Tech Stack

React 19 · TypeScript · Vite · Tailwind CSS 4 · react-i18next · lucide-react · Zustand · Vitest

## Commands

```bash
npm run dev         # Dev server
npm run build       # Production build (tsc + vite)
npm run lint        # ESLint
npm run test        # Vitest
```

## Structure

```
components/ui/   # Reusable: Button, Card, Modal, Input, Select, Toggle, Badge, etc.
components/      # App-level: ChatMessage, ChatInput, Sidebar, Navbar
pages/           # Route pages (Setup/ for wizard)
layouts/         # AppLayout, SettingsLayout
hooks/           # useChat, useSSE
stores/          # Zustand (app.ts)
lib/             # api.ts, utils.ts (cn, timeAgo, truncate)
i18n/            # en.json, pt.json, es.json
```

## Conventions

- **Icons**: `lucide-react` — `import { Settings } from 'lucide-react'`
- **Classes**: `cn()` from `@/lib/utils` (clsx + tailwind-merge)
- **Imports**: `@/` alias, never relative `../../`
- **i18n**: `t('namespace.key')` — no hardcoded text. Add keys to all 3 files.
- **API**: `api.*()` from `@/lib/api` — never raw fetch
- **State**: `useState` local, Zustand (`@/stores/app`) global
- **Exports**: Named `export function X`, not default

## Design Tokens

Defined in `index.css` via `@theme`. Always use semantic classes:

- Text: `text-text-primary`, `text-text-secondary`, `text-text-muted`
- Background: `bg-bg-main`, `bg-bg-surface`, `bg-bg-subtle`
- Brand: `bg-brand`, `bg-brand-hover`
- Border: `border-border`
- Status: `text-success`, `text-error`, `text-warning`

Dark mode: handled via `.dark` class — tokens switch automatically.

## Don'ts

- Raw Tailwind colors (`bg-gray-100`) — use tokens (`bg-bg-subtle`)
- Hardcoded text — use `t()`
- Template literals in className — use `cn()`
- New components without checking `components/ui/` first
