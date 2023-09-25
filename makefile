.PHONY: generate install

generate:
	go generate ./...

install: generate
	go install .