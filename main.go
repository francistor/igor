package main

import (
	"fmt"
	"igor/config"
)

func main() {

	_, err := config.ReadResource("resources/diameterDictionary.txt")
	if err != nil {
		fmt.Println("error")
	} else {
	}
}
