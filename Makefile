.PHONY: dev build up down logs restart clean migrate-up migrate-down migrate-version migrate-create migrate-baseline

# ── Local development ─────────────────────────────
dev-frontend:
	cd frontend && npm run dev

dev-backend:
	cd backend && go run ./cmd/server

# ── Docker ────────────────────────────────────────
up:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f --tail 100

logs-backend:
	docker compose logs -f --tail 100 backend

restart:
	docker compose restart backend

# ── Build ─────────────────────────────────────────
build-backend:
	cd backend && CGO_ENABLED=0 go build -o ./bin/server ./cmd/server

build-frontend:
	cd frontend && npm run build

# ── Maintenance ───────────────────────────────────
clean:
	docker compose down -v
	rm -rf backend/bin frontend/dist

# ── Go tools ──────────────────────────────────────
tidy:
	cd backend && go mod tidy

lint:
	cd backend && golangci-lint run ./...

test:
	cd backend && go test ./...

# Integration tests spin up a real Postgres via testcontainers-go to
# exercise upsert / migration semantics that pure unit tests can't
# fake. Tagged `integration` so `make test` stays fast and Docker-free.
#
# Local dev on Colima: macOS does NOT forward published container
# ports by default, so the gorm-side dial fails with "connection
# refused" even though the container booted fine. Fix:
#   colima start --network-address
# Docker Desktop and GitHub Actions runners work out of the box.
test-integration:
	cd backend && go test -tags=integration -timeout=300s ./...

# ── Database ──────────────────────────────────────
db-shell:
	docker compose exec postgres psql -U subguard -d subguard

redis-shell:
	docker compose exec redis redis-cli

# ── Migrations (golang-migrate) ───────────────────
# Install the CLI first:  brew install golang-migrate
# Set DATABASE_URL, e.g.:
#   export DATABASE_URL="postgres://subguard:PASS@localhost:5432/subguard?sslmode=disable"
# See docs/MIGRATIONS.md for the baseline adoption procedure.
MIGRATIONS_DIR := backend/migrations

migrate-up:
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" down 1

migrate-version:
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" version

migrate-create:
	@test -n "$(name)" || (echo "usage: make migrate-create name=add_foo" && exit 1)
	migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $(name)

# One-time: stamp an existing DB as already at the baseline version
# WITHOUT running 000001 (the schema is already there). See docs/MIGRATIONS.md.
migrate-baseline:
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" force 1

# Generate 000001_baseline.up.sql + .down.sql from the production schema.
# This is step 1 of the PR-2 cutover documented in docs/MIGRATIONS.md.
# Runs sanity checks against the dump and refuses to write the down
# migration if the dump is missing expected columns or indexes.
migrate-generate-baseline:
	./scripts/generate-baseline.sh

# Show the current migration version + whether the database is "dirty"
# (a previous up/down failed partway and needs operator intervention).
# Use as a smoke check before any deploy that includes migrations.
migrate-status:
	@migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" version 2>&1 \
		| awk '{ print "version:", $$0 }'
