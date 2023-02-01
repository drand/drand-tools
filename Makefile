.PHONY: test test-unit
.PHONY: test-unit-cover
.PHONY: coverage
.PHONY: test-integration
.PHONY: linter install build client

VER_PACKAGE=github.com/drand/drand/common
CLI_PACKAGE=github.com/drand/drand-tools/cmd/db-migration

GIT_REVISION := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date -u +%d/%m/%Y@%H:%M:%S)

drand: build

####################  Lint and fmt process ##################

install_lint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.50

lint:
	golangci-lint --version
	golangci-lint run --timeout 5m

lint-todo:
	golangci-lint run -E stylecheck -E gosec -E goconst -E godox -E gocritic

fmt:
	@echo "Checking (& upgrading) formatting of files. (if this fail, re-run until success)"
	@{ \
		files=$$( go fmt ./... ); \
		if [ -n "$$files" ]; then \
		echo "Files not properly formatted: $$files"; \
		exit 1; \
		fi; \
	}

check-modtidy:
	go mod tidy
	git diff --exit-code -- go.mod go.sum

clean:
	go clean

############################################ Test ############################################

test: test-unit test-integration

test-unit:
	go test -failfast -race -short -v ./...

test-unit-cover:
	go test -failfast -short -v -coverprofile=coverage.txt -covermode=count -coverpkg=all $(go list ./... | grep -v /demo/)

test-integration:
	go test -failfast -race -short -tags integration -v ./...

coverage:
	go get -v -t -d ./...
	go test -failfast -v -covermode=atomic -coverpkg ./... -coverprofile=coverage.txt ./...

# create the "db-migration" binary and install it in $GOBIN
install:
	go install -ldflags "-X $(VER_PACKAGE).COMMIT=$(GIT_REVISION) -X $(VER_PACKAGE).BUILDDATE=$(BUILD_DATE) -X $(CLI_PACKAGE).buildDate=$(BUILD_DATE) -X $(CLI_PACKAGE).gitCommit=$(GIT_REVISION)" ./cmd/db-migration

# create the "db-migration" binary in the current folder
db-migration:
	go build -o db-migration -mod=readonly -ldflags "-X $(VER_PACKAGE).COMMIT=$(GIT_REVISION) -X $(VER_PACKAGE).BUILDDATE=$(BUILD_DATE) -X $(CLI_PACKAGE).buildDate=$(BUILD_DATE) -X $(CLI_PACKAGE).gitCommit=$(GIT_REVISION)" ./cmd/db-migration

build_all: db-migration

#build_docker_all: build_docker build_docker_dev
#
#build_docker:
#	docker build --build-arg gitCommit=$(GIT_REVISION) --build-arg buildDate=$(BUILD_DATE) -t drandorg/go-drand:latest .
#
#build_docker_dev:
#	docker build -f test/docker/Dockerfile --build-arg gitCommit=$(GIT_REVISION) --build-arg buildDate=$(BUILD_DATE) -t drandorg/go-drand-dev:latest .
