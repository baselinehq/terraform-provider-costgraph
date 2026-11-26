TFPLUGINDOCS_VERSION := v0.25.0
.DEFAULT_GOAL := build
GORELEASER_VERSION := 2.20.3
TFPLUGINDOCS := go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@$(TFPLUGINDOCS_VERSION)
GORELEASER := go run github.com/goreleaser/goreleaser/v2@v$(GORELEASER_VERSION)

.PHONY: fmt build lint generate check-generated testacc testacc-core testacc-aws testacc-gcp release-check

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
	TF_ACC=1 go test -v -count=1 -timeout 15m ./internal/provider -run TestAcc

testacc-core:
	TF_ACC=1 go test -v -count=1 -timeout 15m ./internal/provider -run 'TestAcc(FocusPushIntegration|Tenant|AIGatewayIntegration)$$'

testacc-aws:
	TF_ACC=1 go test -v -count=1 -timeout 15m ./internal/provider -run '^TestAccAWSIntegration$$'

testacc-gcp:
	TF_ACC=1 go test -v -count=1 -timeout 15m ./internal/provider -run '^TestAccGoogleCloudIntegration$$'

release-check:
	$(GORELEASER) check
