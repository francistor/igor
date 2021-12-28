package main

import (
	"flag"
	"fmt"
	"igor/config"
	"net/http"
	"time"
)

func main() {

	// Get the command line arguments
	bootPtr := flag.String("boot", "resources/searchRules.json", "File or http URL with Configuration Search Rules")
	instancePtr := flag.String("instance", "", "Name of instance")
	flag.Parse()

	// Initialize the Config Object
	config.Config.Init(*bootPtr, *instancePtr)

	// Start http server
	go httpServer()

	time.Sleep(10 * time.Second)

}

func httpServer() {
	// Serve configuration
	var fileHandler = http.FileServer(http.Dir("resources"))
	http.Handle("/", fileHandler)
	err := http.ListenAndServe(":8100", nil)
	if err != nil {
		panic("could not start http server")
	}
	fmt.Println("Configuration HTTP server started")
}

// Check that if multiple routines ask for the same object,
// only one fully progresses while the others wait and finally
// get from cache.
func testGetMultipleConfigObject(objectName string) {
	for i := 0; i < 10; i++ {
		go func() {
			_, err := config.Config.GetConfigObject(objectName)
			if err != nil {
				fmt.Println("error")
				panic(err)
			}
		}()
	}
}
