EDITOR_ID ?= editor
EDITOR_SECRET ?= editor-secret

test:
	go test ./...

build:
	docker compose --profile client build

init:
	docker compose run --rm server init --db /data/edition.db --editor-id $(EDITOR_ID) --editor-secret $(EDITOR_SECRET)

up:
	docker compose up server

down:
	docker compose down

