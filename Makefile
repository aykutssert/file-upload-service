.PHONY: check fmt fmt-check test test-race vet

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
