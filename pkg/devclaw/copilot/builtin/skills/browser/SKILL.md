---
name: browser
description: "Automate web browsing, scraping, and form interaction"
trigger: automatic
---

# Browser Automation

Navigate websites, interact with elements, capture screenshots, and extract content.

## Architecture
```
┌─────────────────────────────────────────────────────────────┐
│                      Agent Context                          │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ browser_navigate│
                  │   (load page)   │
                  └────────┬────────┘
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
        ▼                  ▼                  ▼
┌───────────────┐  ┌───────────────┐  ┌───────────────┐
│ browser_click │  │ browser_fill  │  │ browser_wait  │
│ (interact)    │  │ (input text)  │  │ (for content) │
└───────────────┘  └───────────────┘  └───────────────┘
```

## Tools
| Tool | Action |
|------|--------|
| `browser_navigate` | Go to URL |
| `browser_screenshot` | Capture page |
| `browser_content` | Get page text |
| `browser_click` | Click element |
| `browser_fill` | Fill form field |
| `browser_wait` | Wait for content |

## Workflow Pattern
```
1. NAVIGATE → browser_navigate(url="...")
2. WAIT     → browser_wait(text="Expected content")
3. INTERACT → browser_click() or browser_fill()
4. EXTRACT  → browser_content() or browser_screenshot()
```

## Navigation
```bash
browser_navigate(url="https://example.com/products")
```

## Waiting
```bash
browser_wait(text="Welcome")
browser_wait(time=3)  # 3 seconds
```

## Content & Screenshots
```bash
browser_content()  # Get page text
browser_screenshot(filename="page.png")  # Capture
```

## Interaction
```bash
browser_click(ref="submit-btn")
browser_fill(ref="email", value="user@example.com")
```

## Common Patterns

### Login Flow
```bash
browser_navigate(url="https://example.com/login")
browser_wait(text="Email")
browser_fill(ref="email", value="user@example.com")
browser_fill(ref="password", value="secret123")
browser_click(ref="submit-btn")
browser_wait(text="Dashboard")
```

### Search and Extract
```bash
browser_navigate(url="https://example.com/search")
browser_fill(ref="search-input", value="golang tutorial")
browser_click(ref="search-btn")
browser_wait(text="Results")
browser_content()
```

## Troubleshooting

### "Element not found"
- Get current content to see available refs: `browser_content()`
- Wait longer: `browser_wait(time=3)`

### "Page not loading"
- Check with screenshot: `browser_screenshot()`
- Verify URL is correct

## Tips
- Always wait after navigate
- Use screenshots for debugging
- Get refs from content output
