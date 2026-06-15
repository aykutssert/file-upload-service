.PHONY: check db-down db-migrate db-up fmt fmt-check schema-check services-up test test-race vet

check: fmt-check vet test test-race

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || \
		(echo "Run 'make fmt' to format Go files"; gofmt -l .; exit 1)

vet:
	@if go list ./... 2>/dev/null | grep -q .; then go vet ./...; fi

test:
	@if go list ./... 2>/dev/null | grep -q .; then go test ./...; fi

test-race:
	@if go list ./... 2>/dev/null | grep -q .; then go test -race ./...; fi

db-up:
	docker compose up -d --wait postgres

db-down:
	docker compose down

db-migrate:
	go run github.com/pressly/goose/v3/cmd/goose@v3.27.1 \
		-dir migrations postgres \
		"$${UPLOAD_API_DATABASE_URL:-postgres://file_upload:local-development-only@127.0.0.1:5432/file_upload?sslmode=disable}" \
		up

services-up:
	docker compose up -d --build --wait
	$(MAKE) db-migrate

schema-check:
	sh scripts/schema-check.sh
