APP_NAME := $(shell grep -m 1 "^module" go.mod | awk '{print $$2}' | awk -F'/' '{print $$NF}')

all: build

serve:
	APP_ENV=production go run main.go serve

serve-dev:
	APP_ENV=development go run main.go serve

build:
	APP_ENV=production go build -o $(APP_NAME) main.go
	@echo "Build Production Done: $(APP_NAME)"

build-dev:
	APP_ENV=development go build -o $(APP_NAME) main.go
	@echo "Build Development Done: $(APP_NAME)"

migrate:
	APP_ENV=production go run main.go migrate
	@echo "Migration Production Done"

migrate-dev:
	APP_ENV=development go run main.go migrate
	@echo "Migration Development Done"

migrate-reset:
	APP_ENV=production go run main.go migrate-reset
	@echo "Migration Production Done"

migrate-reset-dev:
	APP_ENV=development go run main.go migrate-reset
	@echo "Migration Development Done"

swagger:
	$(shell go env GOPATH)/bin/swag init -g main.go --parseInternal --parseDependency --ot json -o .

test:
	APP_ENV=development go test -v ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy

clean:
	rm -f $(APP_NAME)