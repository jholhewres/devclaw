# Frontend Guidelines

This document provides guidelines for React/TypeScript frontend development.

## Tech Stack

- React 18+ TypeScript
- Vite (build tool)
- Tailwind CSS
- react-i18next (internationalization)
- Untitled UI (icons)

## Code Quality

```bash
npm run lint        # Check for code issues
npm run lint:fix    # Auto-fix ESLint issues
npm run format      # Format with Prettier
```

Configured with ESLint + Prettier (single quotes, semicolons, 100 char width). Run `lint:fix` to fix and format in one go.

## Tailwind CSS & Styling

- **Semantic colors**: Use configured classes (not `var()`)
- **Conditional classes**: Use `cx` utility from `@/utils/cx` (not template literals)
- **Mobile-first**: Use breakpoint prefixes like `sm:flex-row`, `md:p-6`

**Available semantic classes:**
- Text: `text-primary`, `text-secondary`, `text-muted`, `text-tertiary`, `text-quaternary`
- Background: `bg-main`, `bg-surface`, `bg-subtle`, `bg-elevated`
- Border: `border`
- Brand: `brand`, `brand-hover`
- Status: `success`, `error`, `warning`, `info`
- Radius: `radius-sm/md/lg/xl/2xl`

## Components

- **Check existing**: Look in `components/base/` and `components/application/` before creating new
- **Named exports**: Use `export const ComponentName = ...` (not default exports)
- **Imports**: Use `@/` alias (not relative paths like `../../components/...`)
- **Naming**: Files/Components in PascalCase, hooks in camelCase with `use` prefix

**File structure**:
```
components/base/      # UI components (Button, Input, Toggle)
components/application/  # App components (modals, tabs)
pages/               # Routes
lib/                 # API layer
utils/               # Pure functions
icons/               # Custom icons
```

## Icons

- **Untitled UI**: Import directly from `@untitledui/icons` (don't create wrappers)
- **Custom icons**: Create in `web/src/icons/` for project-specific icons

**Untitled UI MCP setup (optional):**
```bash
claude mcp add untitledui --transport http https://www.untitledui.com/react/api/mcp
```

Use MCP for discovering appropriate icons and validating design system components.

## Code Patterns

- **State**: Use `useState` for local, lift up when needed
- **API**: Always use `api.*()` from `@/lib/api` (not direct fetch)
- **i18n**: Use `useTranslation()` and `t('page.section.key')` (no hardcoded text)
- **Errors**: Show user feedback via toast (import `useToast` from `@/contexts/ToastContext`)
- **Loading/Empty**: Use spinners for loading, `EmptyState` component for empty lists

## TypeScript & Comments

- **Strict typing**: Always type props and interfaces, use `unknown` instead of `any`
- **Comments**: Only for complex logic ("why", not "what")

## Anti-patterns

1. Don't recreate existing components - check `components/` first
2. Don't use native Tailwind colors - use semantic classes
3. Don't use long relative imports - use `@/` alias
4. Don't leave hardcoded text - use i18n
5. Don't use template literals for className - use `cx`
6. Don't create wrappers for Untitled UI icons - import directly

## Untitled UI MCP

When available, use for discovering icons and validating design system components (optional but recommended).
