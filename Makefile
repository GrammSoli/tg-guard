.PHONY: dev build up down logs restart clean

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

# ── Database ──────────────────────────────────────
db-shell:
	docker compose exec postgres psql -U subguard -d subguard

redis-shell:
	docker compose exec redis redis-cli
