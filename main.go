package main

import (
	"flag"
	"time"

	"github.com/francistor/igor/core"
	"github.com/francistor/igor/handler"
	"github.com/francistor/igor/router"
)

func main() {

	// defer profile.Start(profile.BlockProfile).Stop()

	// Get the command line arguments
	bootPtr := flag.String("boot", "resources/searchRules.json", "File or http URL with Configuration Search Rules")
	instancePtr := flag.String("instance", "", "Name of instance")

	flag.Parse()

	// Initialize the Config Object
	core.InitPolicyConfigInstance(*bootPtr, *instancePtr, true)

	// Get logger
	logger := core.GetLogger()

	// Start Diameter
	_ = router.NewDiameterRouter(*instancePtr, handler.EmptyDiameterHandler).Start()
	logger.Info("Diameter router started")

	// Start Radius
	_ = router.NewRadiusRouter(*instancePtr, handler.TestRadiusAttributesHandler).Start()
	//_ = router.NewRadiusRouter(*instancePtr, handler.EmptyRadiusHandler).Start()
	logger.Info("Radius router started")

	time.Sleep(1000 * time.Minute)

}
