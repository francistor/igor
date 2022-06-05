package main

import (
	"flag"
	"igor/config"
)

func main() {

	// Get the command line arguments
	bootPtr := flag.String("boot", "resources/searchRules.json", "File or http URL with Configuration Search Rules")
	instancePtr := flag.String("instance", "", "Name of instance")

	flag.Parse()

	// Initialize the Config Object
	config.InitPolicyConfigInstance(*bootPtr, *instancePtr, true)

	// Get logger
	// logger := config.GetConfigInstance(*instancePtr).IgorLogger
}
