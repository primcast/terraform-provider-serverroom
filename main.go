// terraform-provider-serverroom: the Terraform provider for your dedicated servers, one for every brand.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/primcast/terraform-provider-serverroom/provider"
)

// version is set by the release build: go build -ldflags "-X main.version=1.2.3".
var (
	version = "dev"
	commit  = ""
)

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/primcast/serverroom",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
