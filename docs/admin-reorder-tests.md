# Reorder integration tests

Run `go test ./...` for ordinary tests. Set `ADMIN_REORDER_TEST_DATABASE_URL` to a disposable PostgreSQL instance and run `go test ./internal/admin -run Reorder -v` to include database verification.

The database test creates a uniquely named database, installs a minimal schema with the application's deferred module order constraint, and drops that database afterward. It never writes application tables in the supplied connection's database. The connection needs permission to create and drop databases.

Coverage includes published, unpublished and archived siblings; foreign/missing IDs; PostgreSQL array encoding in Exec mode; atomic rollback after an injected draft write failure; preservation of unrelated draft edits and draft destinations; and publishing after reorder.
