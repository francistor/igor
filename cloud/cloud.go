package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/storage"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

var gsRegex = regexp.MustCompile("gs://(.+)/(.+)")
var gStorageClient *storage.Client
var mutex sync.Mutex

// Do not use GetLogger() here, since it may have not been initialized

// Retrieves an object from Google Storage.
// It must be specified as gs://<bucket-name>/<object-name>
// Returns the contents of the object and an optional error
func GetGoogleStorageObject(objName string) ([]byte, error) {

	// Get the credentials
	clientOptions, _ := GetGoogleAccessData()

	// Create Google Storage client
	var err error

	ctx := context.Background()

	// Depending on whether we are using specific credentials file or ADC
	mutex.Lock()
	if gStorageClient == nil {
		if clientOptions == nil {
			gStorageClient, err = storage.NewClient(ctx)
		} else {
			gStorageClient, err = storage.NewClient(ctx, clientOptions)
		}
		if err != nil {
			panic("error creating Google storage client " + err.Error())
		}
	}
	mutex.Unlock()

	// Get the bucket and file names
	matches := gsRegex.FindStringSubmatch(objName)
	if len(matches) != 3 {
		panic("bad gs URL specification: " + objName)
	}

	bucket := gStorageClient.Bucket(matches[1])
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

func GetBigqueryClient(ctx context.Context) (*bigquery.Client, error) {

	// Get the credentials
	clientOptions, projectId := GetGoogleAccessData()

	if clientOptions == nil {
		return bigquery.NewClient(ctx, projectId)
	} else {
		return bigquery.NewClient(ctx, projectId, clientOptions)
	}
}

// Returns the options to use in client building and the project-id
// using the specified credentials file or Google ADC credentials.
// If options is not nil, they must be used to create the client, because specific
// credentials are needed. Otherwise, use the default client creation.
// Returns the clientOptions for client building and the project_id
// Does not return error. Panics.
func GetGoogleAccessData() (option.ClientOption, string) {

	var projectId string

	// If passing client credentials, use them to build the bigquery client. The projectId is one of the properties
	// of the JSON credentials file
	credentialsFile := os.Getenv("IGOR_CLOUD_CREDENTIALS")
	if credentialsFile != "" {

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

		// Use ADC to get the default credentials including the projectId
		googleCredentials, err := google.FindDefaultCredentials(context.Background(), compute.ComputeScope)
		if err != nil {
			panic("could not get default credentials. Not running in Google cloud or IGOR_CLOUD_CREDENTIALS not set " + err.Error())
		}

		return nil, googleCredentials.ProjectID
	}
}
