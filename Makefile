.PHONY: all
all: gen build run

.PHONY: clean
clean:
	@go clean

.PHONY: gen
gen:
	@echo "Generating dependency files..."
	@go generate ./...

.PHONY: lint
lint:
	@golangci-lint run

.PHONY: build
build:
	@go build -ldflags "-s -w -X go.szostok.io/version.version=${GIT_TAG} -X 'go.szostok.io/version.buildDate=`date`' -X go.szostok.io/version.commit=${GIT_COMMIT} -X go.szostok.io/version.commitDate=${GIT_COMMIT_DATE}" .

.PHONY: run
run:
	@./durkalive server
