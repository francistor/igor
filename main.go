package main

import (
	"fmt"
	"igor/diamdict"
	"os"
)

func main() {

	/*
		myDictItem := diamdict.GroupedAVPDictItem{
			AVPDictItem: diamdict.AVPDictItem{
				Code:         0,
				VendorId:     0,
				Name:         "myAVP",
				DiameterType: diamdict.None,
			},
			GroupedItems: map[string]diamdict.GroupedProperties{},
		}
	*/

	// Read the full Diameter Dictionary
	jsonDict, _ := os.ReadFile("/home/francisco/igor/resources/diameterDictionary.json")
	diameterDict := diamdict.NewDictionaryFromJSON(jsonDict)

	fmt.Println(diameterDict)
}
