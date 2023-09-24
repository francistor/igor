package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"

	"cloud.google.com/go/storage"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

var gsRegex = regexp.MustCompile("gs://(.+)/(.+)")

// Retrieves an object from Google Storage.
// It must be specified as gs://<bucket-name>/<object-name>
// Returns the contents of the object and an optional error
func getGoogleStorageObject(objName string) ([]byte, error) {

	// Get the credentials
	clientOptions, _ := getGoogleAccessData()

	// Create Google Storage client
	var gs *storage.Client
	var err error

	ctx := context.Background()

	// Depending on whether we are using specific credentials file or ADC
	if clientOptions == nil {
		gs, err = storage.NewClient(ctx)
	} else {
		gs, err = storage.NewClient(ctx, clientOptions)
	}
	if err != nil {
		panic("error creating Google storage client " + err.Error())
	}
	defer gs.Close()

	// Get the bucket and file names
	matches := gsRegex.FindStringSubmatch(objName)
	if len(matches) != 3 {
		panic("bad gs URL specification: " + objName)
	}

	bucket := gs.Bucket(matches[1])
	obj := bucket.Object(matches[2])
	objReader, err := obj.NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not create reader for %s due to %w", objName, err)
	}
	var contents bytes.Buffer
	if _, err := io.Copy(&contents, objReader); err != nil {
		return nil, fmt.Errorf("could not read %s due to %w", objName, err)
	}

	return contents.Bytes(), nil
}

// Returns the options to use in client building and the project-id
// using the specified credentials file or Google ADC credentials.
// If options is not nil, they must be used to create the client, because specific
// credentials are needed. Otherwise, use the default client creation.
func getGoogleAccessData() (option.ClientOption, string) {

	var projectId string

	// If passing client credentials, use them to build the bigquery client. The projectId is one of the properties
	// of the JSON credentials file
	credentialsFile := os.Getenv("IGOR_CLOUD_CREDENTIALS")
	if credentialsFile != "" {

		GetLogger().Debug("using Google credentials file")

		// To store the json account key file contents
		var cred struct {
			Project_id string
		}

		if credBytes, err := os.ReadFile(credentialsFile); err != nil {
			panic("could not read credentials file " + credentialsFile)
		} else {
			json.Unmarshal(credBytes, &cred)
		}

		if cred.Project_id == "" {
			panic("credentials file " + credentialsFile + " could not be parsed ")
		}
		projectId = cred.Project_id

		options := option.WithCredentialsFile(credentialsFile)

		return options, projectId
	} else {

		GetLogger().Debug("using Google ADC")

		// Use ADC to get the default credentials including the projectId
		googleCredentials, err := google.FindDefaultCredentials(context.Background(), compute.ComputeScope)
		if err != nil {
			panic("could not get default credentials. Are we running in a Google Cloud? " + err.Error())
		}

		return nil, googleCredentials.ProjectID
	}
}
