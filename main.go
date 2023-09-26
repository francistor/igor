package main

import (
	"flag"
	"os"
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
	logLocationPtr := flag.String("log", "", "location of log files")

	flag.Parse()

	// Set environment variable with
	if *logLocationPtr != "" {
		os.Setenv("IGOR_LOG_OUTPUTS", *logLocationPtr)
	}

	// Initialize the Config Object
	core.InitPolicyConfigInstance(*bootPtr, *instancePtr, nil, true)

	// Get logger
	logger := core.GetLogger()

	// Start Diameter
	_ = router.NewDiameterRouter(*instancePtr, handler.EmptyDiameterHandler).Start()
	logger.Info("diameter router started")

	// Start Radius
	_ = router.NewRadiusRouter(*instancePtr, handler.TestRadiusAttributesHandler).Start()
	//_ = router.NewRadiusRouter(*instancePtr, handler.EmptyRadiusHandler).Start()
	logger.Info("radius router started")

	time.Sleep(1000 * time.Minute)

}
