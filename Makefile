TFPLUGINDOCS_VERSION := v0.25.0
.DEFAULT_GOAL := build
GORELEASER_VERSION := 2.20.3
TFPLUGINDOCS := go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@$(TFPLUGINDOCS_VERSION)
GORELEASER := go run github.com/goreleaser/goreleaser/v2@v$(GORELEASER_VERSION)

.PHONY: fmt build lint generate check-generated testacc release-check

fmt:
	gofmt -w .
	terraform fmt -recursive examples

build:
	go build -trimpath -o bin/terraform-provider-costgraph .

lint:
	@test -z "$$(gofmt -l .)"
	go vet ./...
	terraform fmt -check -recursive examples

generate:
	terraform fmt -recursive examples
	$(TFPLUGINDOCS) generate --provider-name costgraph

check-generated: generate
	git diff --exit-code -- docs examples
	@test -z "$$(git ls-files --others --exclude-standard docs examples)"

testacc:
	@test -n "$${COSTGRAPH_TEST_API_URL}" || (echo 'Set COSTGRAPH_TEST_API_URL to an isolated CostGraph API.'; exit 1)
	@test -n "$${COSTGRAPH_TEST_API_KEY}" || (echo 'Set COSTGRAPH_TEST_API_KEY for the isolated test deployment.'; exit 1)
	@test -n "$${COSTGRAPH_TEST_TENANT_ID}" || (echo 'Set COSTGRAPH_TEST_TENANT_ID for the isolated test tenant.'; exit 1)
	TF_ACC=1 go test -v -count=1 -timeout 15m ./internal/provider -run TestAcc

release-check:
	$(GORELEASER) check
