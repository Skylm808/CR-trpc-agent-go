# CR-trpc-agent-go

Prototype of an automated Go code review agent built on `trpc-agent-go`.

## First version

- parse unified diffs
- run deterministic review rules
- generate JSON and Markdown reports
- persist results to SQLite

## Run

```bash
go test ./...
```

