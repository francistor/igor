package main

import (
	"flag"
	"time"

	"github.com/francistor/igor/config"
	"github.com/francistor/igor/handlerfunctions"
	"github.com/francistor/igor/router"
)

func main() {

	// defer profile.Start(profile.BlockProfile).Stop()

	// Get the command line arguments
	bootPtr := flag.String("boot", "resources/searchRules.json", "File or http URL with Configuration Search Rules")
	instancePtr := flag.String("instance", "", "Name of instance")

	flag.Parse()

	// Initialize the Config Object
	config.InitPolicyConfigInstance(*bootPtr, *instancePtr, true)

	// Get logger
	logger := config.GetLogger()

	// Start Diameter
	_ = router.NewDiameterRouter(*instancePtr, handlerfunctions.EmptyDiameterHandler)
	logger.Info("Diameter router started")

	// Start Radius
	// _ = router.NewRadiusRouter(*instancePtr, handlerfunctions.TestRadiusAttributesHandler)
	_ = router.NewRadiusRouter(*instancePtr, handlerfunctions.EmptyRadiusHandler)
	logger.Info("Radius router started")

	time.Sleep(1000 * time.Minute)

}
