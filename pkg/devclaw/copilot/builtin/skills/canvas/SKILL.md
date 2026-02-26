---
name: canvas
description: "Create and manage persistent interactive workspaces"
trigger: automatic
---

# Canvas

Persistent workspaces for dashboards, progress displays, and interactive content.

## Tools
| Tool | Action |
|------|--------|
| `canvas_create` | Create new canvas |
| `canvas_update` | Update content |
| `canvas_list` | List all canvases |
| `canvas_stop` | Stop canvas |

## When to Use
| Scenario | Canvas |
|----------|--------|
| Long-running task progress | Progress bar |
| Live dashboard | System monitoring |
| Interactive menu | User selection |

## Creating a Canvas
```bash
canvas_create(
  name="build-progress",
  content="<h1>Build Progress</h1><div>Status: Starting...</div>"
)
# Output: Canvas created with ID: canvas-abc123
```

## Updating Content
```bash
canvas_update(
  id="canvas-abc123",
  content="<h1>Build Progress</h1><div>Status: Complete!</div>"
)
```

## Listing Canvases
```bash
canvas_list()
# Output:
# Active canvases (2):
# - canvas-abc123 [build-progress]: running, 5m ago
```

## Stopping a Canvas
```bash
canvas_stop(id="canvas-abc123")
```

## Common Pattern: Progress Dashboard
```bash
# Create
canvas_create(name="deploy-status", content="Starting deploy...")

# Update as progress happens
canvas_update(id="canvas-abc", content="Deploying... 50%")

# Final state
canvas_update(id="canvas-abc", content="Deployed successfully!")

# Clean up
canvas_stop(id="canvas-abc")
```

## Tips
- Keep HTML self-contained (inline CSS/JS)
- Update = full content replacement
- Clean up when done
