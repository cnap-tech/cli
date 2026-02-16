.PHONY: build generate clean

build:
	go build -o cnap ./cmd/cnap/

generate:
	go tool oapi-codegen -config oapi-codegen.yaml openapi.json

clean:
	rm -f cnap
