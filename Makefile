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

APP_ENV ?= development
CONFIG = .config.$(APP_ENV).yaml
SECRETS_DIR = .docker/$(APP_ENV)
COMPOSE = docker compose --env-file $(SECRETS_DIR)/compose.env

docker-env:
	@case "$(APP_ENV)" in development|staging|production) ;; *) echo "APP_ENV harus development, staging atau production"; exit 1;; esac
	@test -f $(CONFIG) || { echo "$(CONFIG) tidak ditemukan"; exit 1; }
	@mkdir -p $(SECRETS_DIR) && chmod 700 .docker $(SECRETS_DIR)
	@rm -f $(SECRETS_DIR)/*
	@{ echo "APP_ENV=$(APP_ENV)"; awk -v d=$(SECRETS_DIR) ' \
		BEGIN { y = d "/app.yaml"; printf "" > y; printf "" > (d "/redis.conf"); \
			n = split("database_pass redis_password jwt_secret midtrans_server_key", req, " "); \
			for (i = 1; i <= n; i++) printf "" > (d "/" req[i]) } \
		/^[A-Za-z_]+:[ ]*$$/ { sec = toupper(substr($$1, 1, length($$1) - 1)); print > y; next } \
		/^[^ ]/ { sec = "" } \
		sec != "" && /^  [A-Za-z_]+:/ { \
			key = $$1; sub(/:.*/, "", key); name = sec "_" toupper(key); \
			val = $$0; sub(/^[^:]*:[ ]*/, "", val); \
			if (val ~ /^"/) { val = substr(val, 2); sub(/".*/, "", val) } else sub(/[ ]+#.*/, "", val); \
			if (name ~ /(PASS|PASSWORD|SECRET|KEY)$$/) { \
				printf "%s", val > (d "/" tolower(name)); \
				if (name == "REDIS_PASSWORD" && val != "") printf "requirepass \"%s\"\n", val > (d "/redis.conf"); \
				print "  " key ": \"\"" > y; next \
			} \
			print > y; \
			if (val == "" || val ~ /^\[/) next; \
			gsub(/\047/, "\\\047", val); \
			printf "%s=\047%s\047\n", name, val; next \
		} \
		{ print > y }' $(CONFIG); } > $(SECRETS_DIR)/compose.env
	@chmod 644 $(SECRETS_DIR)/*
	@echo "APP_ENV=$(APP_ENV): $(CONFIG) -> $(SECRETS_DIR)/"

# ---- docker: APP_ENV choosing .config.<env>.yaml (default development) ----
docker-up: docker-env swagger
	$(COMPOSE) up -d --build

docker-rebuild: docker-env swagger
	$(COMPOSE) up -d --build migrate app

docker-down: docker-env
	$(COMPOSE) down

docker-config: docker-env
	@$(COMPOSE) config

docker-create-admin: docker-env
	@printf "username: "; read u; printf "password: "; stty -echo; read p; stty echo; echo; \
	$(COMPOSE) run --rm migrate && \
	$(COMPOSE) run --rm migrate create-admin -u "$$u" -p "$$p"
