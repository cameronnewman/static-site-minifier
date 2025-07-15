
BASE_IMAGE          := scratch

ENVIRONMENT         ?= CI
GOLANG_BUILD_IMAGE  ?= golang:1.24.4-bullseye
GOLANG_LINT_IMAGE   := golangci/golangci-lint:v2.0.2
GOLANG_GOSEC_IMAGE  := securego/gosec:2.22.3

APP					:= builder

#
# golang
#
# goals fmt, lint, test, build & publish (prefixed with 'go-')
#

.PHONY: go-generate
go-generate: ## Runs `go generate` within a docker container
	@echo "+++ $$(date) - Running 'go generate'"

ifeq ($(filter $(ENVIRONMENT),local docker),$(ENVIRONMENT))
	go generate ./...
else
	DOCKER_BUILDKIT=1 \
	docker run --rm \
	-v $(PWD):/usr/src/app \
	-w /usr/src/app \
	--entrypoint "/bin/bash" \
	$(GOLANG_BUILD_IMAGE) \
	-c "cd /usr/src/app && go generate ./..."
endif

	@echo "$$(date) - Completed 'go generate'"

.PHONY: go-fmt
go-fmt: ## Runs `go fmt` within a docker container
	@echo "+++ $$(date) - Running 'go fmt'"

ifeq ($(filter $(ENVIRONMENT),local docker),$(ENVIRONMENT))
	go fmt ./...
else
	DOCKER_BUILDKIT=1 \
	docker run --rm \
	-v $(PWD):/usr/src/app \
	-w /usr/src/app \
	$(GOLANG_BUILD_IMAGE) \
	--entrypoint "/bin/bash" \
	-c "cd /usr/src/app && go fmt ./..."

endif

	@echo "$$(date) - Completed 'go fmt'"

.PHONY: go-lint
go-lint: ## Runs `golangci-lint run` with more than 60 different linters using golangci-lint within a docker container.
	@echo "+++ $$(date) - Running 'golangci-lint run -v'"

ifeq ($(filter $(ENVIRONMENT),local docker),$(ENVIRONMENT))
	golangci-lint run -v
else
	DOCKER_BUILDKIT=1 \
	docker run --rm \
	-e GOPACKAGESPRINTGOLISTERRORS=1 \
	-e GO111MODULE=on \
	-e GOGC=100 \
	-v $(PWD):/usr/src/app \
	-w /usr/src/app \
	--entrypoint "/bin/bash" \
	$(GOLANG_LINT_IMAGE) \
	-c "cd /usr/src/app && golangci-lint run -v"

endif

	@echo "$$(date) - Completed 'golangci-lint run'"

.PHONY: go-test
go-test: ## Runs `go test` within a docker container
	@echo "+++ $$(date) - Running 'go test'"

ifeq ($(filter $(ENVIRONMENT),local docker),$(ENVIRONMENT))
	go test -failfast -cover -coverprofile=coverage.txt -v -p 8 -count=1 ./...
else

	DOCKER_BUILDKIT=1 \
	docker run --rm \
	-v $(PWD):/usr/src/app \
	-w /usr/src/app \
	--entrypoint=bash \
	$(GOLANG_BUILD_IMAGE) \
	-c "mkdir -p internal/frontend/dist/assets && touch internal/frontend/dist/.gitkeep && touch internal/frontend/dist/assets/.gitkeep && go test -failfast -cover -coverprofile=coverage.txt -v -p 8 -count=1 ./..."

endif

	@echo "+++ $$(date) - Completed 'go test'"

.PHONY: go-integration-test
go-integration-test: ## Runs `go test -run integration` within a docker container
	@echo "+++ $$(date) - Running 'go test -integration'"

ifeq ($(filter $(ENVIRONMENT),local docker),$(ENVIRONMENT))
	RUN_TEST="INTEGRATION" \
	go test -failfast -cover -coverprofile=coverage_integration.txt -v -count=1 ./...
else

	DOCKER_BUILDKIT=1 \
	docker run --rm \
	--add-host host.docker.internal:host-gateway \
	-e RUN_TEST="INTEGRATION" \
	-e DB_CON_INTEGRATION="host=host.docker.internal port=54321 user=postgres password=postgres dbname=postgres sslmode=disable pool_max_conns=2" \
	-v $(PWD):/usr/src/app \
	-w /usr/src/app \
	--entrypoint=bash \
	$(GOLANG_BUILD_IMAGE) \
	-c "go install gotest.tools/gotestsum@latest && cd /usr/src/app && gotestsum --junitfile junit.xml --format pkgname-and-test-fails -- -failfast -cover -coverprofile=coverage_integration.txt -v -p 1 -count=1 ./..."

endif

	@echo "+++ $$(date) - Completed 'go test -integration'"

.PHONY: go-build
go-build: check-APP ## Runs `go build` within a docker container
	@echo "+++ $$(date) - Running 'go build' for all go apps"

ifeq ($(filter $(ENVIRONMENT),local docker),$(ENVIRONMENT))
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -o $(APP) -ldflags '-s -w -X main.version=${VERSION_HASH}' cmd/$(APP)/main.go
else

	DOCKER_BUILDKIT=1 \
	docker run --rm \
	-v $(PWD):/usr/src/app \
	-w /usr/src/app \
	--entrypoint=bash \
	$(GOLANG_BUILD_IMAGE) \
	-c "CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -o $(APP) -ldflags '-s -w -X main.version=$(VERSION_HASH)' cmd/$(APP)/main.go"

endif

	@echo "$$(date) - Completed 'go build'"

.PHONY: go-app-build
go-app-build: check-APP ## Runs the build in a multi-stage docker img, requires APP var to be set
	@echo "+++ $$(date) - Running 'go build' for $(APP)"

	DOCKER_BUILDKIT=1 \
	docker build \
	--tag=$(DOCKER_REPO):$(SHA1) \
	--tag=$(DOCKER_REPO):latest \
	--build-arg BUILD_IMAGE=$(GOLANG_BUILD_IMAGE) \
	--build-arg BASE_IMAGE=$(BASE_IMAGE) \
	--build-arg VERSION=$(VERSION_HASH) \
	--build-arg APP=$(APP) \
	--file cmd/$(APP)/Dockerfile .

	@echo "$$(date) - Completed 'go build' for $(APP)"

#
# :kludge: Need to clean-up this section
#


.PHONY: go-run-dev
go-run-dev: check-APP ## Runs the app local within a docker container, requires APP var to be set (excludes go-generate go-sql-generate)
ifeq ($(filter $(ENVIRONMENT),local docker),$(ENVIRONMENT))
	go run -x -ldflags "-X main.version=$(VERSION_HASH)" cmd/$(APP)/main.go
else
	@echo "Starting backend app"
	DOCKER_BUILDKIT=1 \
	docker run -it --rm \
	-v $(PWD):/usr/src/app \
	-w /usr/src/app \
	$(GOLANG_BUILD_IMAGE) \
	go run -v -ldflags "-s -w -X main.version=$(VERSION_HASH)" cmd/$(APP)/main.go
endif


.PHONY: go-app-bash
go-app-bash:  ## Returns an interactive shell in the golang docker image - useful for debugging
	DOCKER_BUILDKIT=1 \
	docker run -it --rm \
	--memory=4g \
	-v $(PWD):/usr/src/app \
	-w /usr/src/app \
	--entrypoint "/bin/bash" \
	$(GOLANG_BUILD_IMAGE)


#
#  /end golang
#
