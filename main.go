// terraform-provider-servcast: the Terraform provider for Server Room and Primcast.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/serverroom/servcast-terraform/provider"
)

// version is set by the release build: go build -ldflags "-X main.version=1.2.3".
var version = "0.1.0"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/serverroom/servcast",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
