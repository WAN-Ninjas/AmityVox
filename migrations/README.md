# Database Migrations

SQL migration files are located in `internal/database/migrations/` and are
embedded into the Go binary at compile time. The `amityvox migrate` CLI command
runs them automatically.

To run migrations manually with `golang-migrate`:

```sh
migrate -source "file://internal/database/migrations" \
        -database "postgres://user:pass@localhost:5432/amityvox?sslmode=disable" \
        up
```
