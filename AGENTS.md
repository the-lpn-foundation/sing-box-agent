# AGENTS.md - sing-box-agent Governance

## Scope

**What this project is:**

A lightweight Go service that embeds sing-box as a library and provides a REST API for remote management. Designed to replace 3x-ui panels and SSH-based config management while maintaining multi-server architecture.

**Key characteristics:**
- Go 1.24.x target version
- Module path: `github.com/lenya/sing-box-agent`
- Single binary deployment (zero-dependency)
- REST API for user management, inbound configuration, and traffic stats
- Prometheus metrics export for Grafana dashboards
- State consistency model: API is source of truth, agent applies desired state

**Non-goals:**
- Web UI (handled by existing bot + web)
- Database storage (delegated to an optional central control-plane API)
- User authentication (delegated to central API)
- Multi-tenancy (one agent = one server)

---

## Safety

### Destructive Command Policy

**BLOCK BY DEFAULT. Allow-once with explicit acknowledgment.**

| Action | Policy |
|--------|--------|
| `rm -rf`, `dd`, `mkfs`, `chmod -R 777` | BLOCKED - never execute |
| `git push --force`, `git reset --hard` | BLOCKED - requires explicit user request |
| Database schema changes | BLOCKED - requires explicit user request |
| Overwriting existing files | ALLOW with confirmation |
| Creating new files | ALLOW |
| Running `go build`, `go test` | ALLOW |
| Running `make lint`, `make test`, `make build` | ALLOW |

**Do Not List:**
- Do NOT run destructive commands without explicit user request
- Do NOT overwrite files without confirmation
- Do NOT modify `.sisyphus/plans/*.md` (READ-ONLY)
- Do NOT commit changes without user request
- Do NOT push to remote without explicit user request

**Allow List:**
- Reading files, directories, and documentation
- Creating new files and directories
- Running `go list ./...`, `go build ./...`, `go test ./...`
- Running `make help`, `make lint`, `make test`, `make build`
- Running `grep`, `find`, `ls`, `cat` for exploration
- Running `git status`, `git diff`, `git log` for inspection

---

## Validation

**Required commands before commit:**

```bash
make lint      # Run golangci-lint
make test      # Run go test ./...
go list ./...  # Verify all packages build
```

**LSP diagnostics must be clean on all changed files.**

---

## Evidence

**What must be captured in `.sisyphus/evidence/`:**

| Artifact | Purpose |
|----------|---------|
| `command.log` | Full command output for audit |
| `output.diff` | Diff of file changes |
| `diagnostics.txt` | LSP diagnostics output |
| `build.log` | Build output for verification |
| `test.log` | Test output for verification |

**Evidence contract:** Every task must produce evidence that validation was performed.

---

## Instruction Precedence

**Order of precedence (highest to lowest):**

1. **Repository instructions** (`AGENTS.md`, `docs/*.md`, `.sisyphus/plans/*.md`)
2. **User instructions** (direct task requests)
3. **Global instructions** (system-wide policies)

**Example:** If `AGENTS.md` requires `make lint` before commit, and user requests a commit without linting, the repository instruction takes precedence.

---

## Instruction Budget

**Guidelines for prompt length:**

- **Concise is key** - Keep context window shared with system prompt, conversation history, and other skills
- **Default assumption:** Claude is already very smart - only add context Claude doesn't already have
- **Challenge each piece of information:** "Does Claude really need this explanation?" and "Does this paragraph justify its token cost?"
- **Prefer concise examples over verbose explanations**
- **Keep SKILL.md body to essentials** - under 500 lines to minimize context bloat

**When splitting content:**
- Keep core workflow in SKILL.md
- Move variant-specific details to reference files
- Use one-level deep reference structure
- Reference files from SKILL.md with clear "when to read" guidance
