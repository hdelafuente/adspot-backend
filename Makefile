.PHONY: run build test migrate clean

## run: apply migrations and start the server
run: migrate
	CGO_ENABLED=1 go run ./cmd/server

## build: compile the binary to bin/server
build:
	CGO_ENABLED=1 go build -o bin/server ./cmd/server

## migrate: apply SQL migrations against adspot.db
migrate:
	@mkdir -p migrations
	@for f in migrations/*.sql; do \
		echo "Applying $$f..."; \
		sqlite3 adspot.db < "$$f"; \
	done
	@echo "Migrations applied."

## test: run all tests
test:
	CGO_ENABLED=1 go test -v -race ./...

## clean: remove build artifacts and the local database
clean:
	rm -f bin/server adspot.db
