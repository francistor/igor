package main

import (
	"flag"
	"igor/config"
	"igor/handler"
)

func main() {

	// Get the command line arguments
	bootPtr := flag.String("boot", "resources/searchRules.json", "File or http URL with Configuration Search Rules")
	instancePtr := flag.String("instance", "", "Name of instance")

	flag.Parse()

	// Initialize the Config Object
	config.InitConfigurationInstance(*bootPtr, *instancePtr)

	// Get logger
	// logger := config.GetConfigInstance(*instancePtr).IgorLogger

	handler := handler.DiameterHandler{}
	handler.Run()
}
