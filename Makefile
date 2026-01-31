.PHONY: build run dev migrate clean

build:
	go build -o bin/moltnet ./cmd/server

run: build
	./bin/moltnet

dev:
	go run ./cmd/server

migrate:
	psql -U moltnet -d moltnet -f migrations/001_init.sql

clean:
	rm -rf bin/
	rm -rf repos/

setup-db:
	sudo -u postgres psql -c "CREATE USER moltnet WITH PASSWORD 'moltnet';"
	sudo -u postgres psql -c "CREATE DATABASE moltnet OWNER moltnet;"
	make migrate
