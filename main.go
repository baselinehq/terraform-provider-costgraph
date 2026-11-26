package main

import (
	"context"
	"flag"
	"log"

	"github.com/baselinehq/terraform-provider-costgraph/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var version = "dev"

func main() {
	debug := flag.Bool("debug", false, "Run with debugger support")
	flag.Parse()
	if err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/baselinehq/costgraph",
		Debug:   *debug,
	}); err != nil {
		log.Fatal(err)
	}
}
