# TODOS

## Phase 1

### JSON error envelope for agent consumers
Define consistent `{"error": "msg", "code": "transport_timeout", "transport": "ssh"}` format.
All commands return this on failure. OpenClaw needs machine-parseable error codes.
Depends on: Phase 1 scaffold.

### Spike cloud auth
Register device, get token, list docs from cloud. ~1 hour validation.
Confirms protocol still works and AGPL risk is manageable before investing in Phase 2.
Depends on: nothing.

## Phase 2

### Define document model type hierarchy
Before the v6 parser: define Go types for Document, Notebook, Folder, Page, Layer,
Group, Stroke, TextBlock, GlyphRange, Point in pkg/model/.
Shared data model that parser writes to and renderer reads from.
Depends on: Phase 1 scaffold. Must complete before parser work.
