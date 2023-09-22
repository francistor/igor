package cdrwriter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

var bqdatasetName = "IgorTest2"
var bqtableName = "TestTable2"

var jBigQueryConfig = `{
	"attributeMap":{
		"IgorInt": "Igor-IntegerAttribute",
		"IgorInt64": "Igor-Integer64Attribute",
		"IgorOctets": "Igor-OctetsAttribute",
		"IgorString": "Class:Igor-StringAttribute",
		"SessionTime": "Acct-Session-Time!Acct-Delay-Time",
		"InputBytes": "Acct-Input-Octets<Acct-Input-Gigawords",
		"Status": "Acct-Status-Type",
		"EventTimestamp": "Igor-TimeAttribute",
		"AcctSessionId": "Acct-Session-Id"
	}
}`

// Used for creating the bigquery resources
// Should be executed as a single test (-run TestCreateSchema) and then wait some time for the resources to be available
// Then use t.Skip() or whatever to avoid execution
func TestCreateSchema(t *testing.T) {

	t.Skip()

	googleCredentialsFile := os.Getenv("IGOR_CLOUD_CREDENTIALS")
	if googleCredentialsFile == "" {
		t.Fatal("IGOR_CLOUD_CREDENTIALS not set")
	}

	// Create the bigquery client. It will not report any errors until really used
	ctx := context.Background()
	client, _, err := getBigqueryClient(ctx)
	if err != nil {
		t.Fatalf("could not create bigquery client: %s", err)
		return
	}
	defer client.Close()

	// These are references. No error occurs if the dataset or the table does not
	// exist
	myDataset := client.Dataset(bqdatasetName)
	myTable := myDataset.Table(bqtableName)

	// To capture detailed errors
	var googleError *googleapi.Error

	// Create dataset if it does not exist
	_, err = myDataset.Metadata(ctx)
	if err != nil {
		if ok := errors.As(err, &googleError); ok {
			if googleError.Code == 404 {
				t.Log("creating the dataset ...")
				if err = myDataset.Create(ctx, nil); err != nil {
					fmt.Println("could not create the dataset", err)
					return
				}
				t.Log("done.")
			}
		}
	} else {
		t.Fatal("dataset already exists")
	}

	// Create table if it does not exit
	_, err = myTable.Metadata(ctx)
	if err != nil {
		if ok := errors.As(err, &googleError); ok {
			if googleError.Code == 404 {
				fmt.Println("creating the table ...")
				cdrSchema := bigquery.Schema{
					{Name: "IgorInt", Required: false, Type: bigquery.IntegerFieldType},
					{Name: "IgorInt64", Required: false, Type: bigquery.IntegerFieldType},
					{Name: "IgorOctets", Required: false, Type: bigquery.BytesFieldType},
					{Name: "IgorString", Required: false, Type: bigquery.StringFieldType},
					{Name: "SessionTime", Required: false, Type: bigquery.IntegerFieldType},
					{Name: "InputBytes", Required: false, Type: bigquery.IntegerFieldType},
					{Name: "Status", Required: false, Type: bigquery.IntegerFieldType},
					{Name: "EventTimestamp", Required: true, Type: bigquery.TimestampFieldType},
					{Name: "AcctSessionId", Required: true, Type: bigquery.StringFieldType},
				}
				if err = myTable.Create(ctx, &bigquery.TableMetadata{Schema: cdrSchema}); err != nil {
					t.Fatal("could not create the table", err)
					return
				} else {
					t.Log("wait for some time until doing insertions")
					return
				}
			}
		}
	} else {
		t.Fatal("table already exists")
	}
}

// NOTE: Remove t.Skip() to execute
func TestBigqueryWriter(t *testing.T) {

	t.Skip()

	// Sanity check
	googleCredentialsFile := os.Getenv("IGOR_CLOUD_CREDENTIALS")
	if googleCredentialsFile == "" {
		t.Fatal("IGOR_CLOUD_CREDENTIALS not set")
	}

	// Get the current number of lines in the table
	currentLines := getBQLinesInTable(t)

	var conf map[string]string
	if err := json.Unmarshal([]byte(jBigQueryConfig), &conf); err != nil {
		t.Fatalf("bad BigQuery format: %s", err)
	}
	bqf := NewBigQueryFormat(conf)

	bqw := NewBigQueryCDRWriter(bqdatasetName, bqtableName, bqf /* Timeout seconds */, 10 /* Glitch seconds */, 60, "../cdr/bigquery/bigquery.backup")

	rp := buildSimpleRadiusPacket(t)

	// The same packet will be written twice
	bqw.WriteRadiusCDR(&rp)
	bqw.WriteRadiusCDR(&rp)

	time.Sleep(1 * time.Second)
	bqw.Close()

	// Get the new number of lines in the table
	newLines := getBQLinesInTable(t)
	if currentLines == newLines {
		t.Fatal("no new lines were detected as inserted")
	}
}

// NOTE: Remove t.Skip() to execute
func TestBigqueryGenBackup(t *testing.T) {

	t.Skip()

	var conf map[string]string
	if err := json.Unmarshal([]byte(jBigQueryConfig), &conf); err != nil {
		t.Fatalf("bad BigQuery format: %s", err)
	}
	bqf := NewBigQueryFormat(conf)

	// Reduced timeout and glitch time
	bqw := NewBigQueryCDRWriter(bqdatasetName, bqtableName, bqf /* Timeout seconds */, 1 /* Glitch seconds */, 1, "../cdr/bigquery/bigquery.backup")
	bqw._forceBigQueryError = true

	rp := buildSimpleRadiusPacket(t)

	// The same packet will be written twice
	bqw.WriteRadiusCDR(&rp)
	bqw.WriteRadiusCDR(&rp)

	time.Sleep(2 * time.Second)
	bqw.Close()

	// Check that file was created
	if _, err := os.Stat("../cdr/bigquery/bigquery.backup"); err != nil {
		t.Fatal("backup file not created")
	}
}

// NOTE: Remove t.Skip() to execute
// Depends on the previous test
func TestBigQueryIngestBackup(t *testing.T) {

	t.Skip()

	// Get the current number of lines in the table
	currentLines := getBQLinesInTable(t)

	var conf map[string]string
	if err := json.Unmarshal([]byte(jBigQueryConfig), &conf); err != nil {
		t.Fatalf("bad BigQuery format: %s", err)
	}
	bqf := NewBigQueryFormat(conf)

	// Reduced timeout and glitch time
	bqw := NewBigQueryCDRWriter(bqdatasetName, bqtableName, bqf /* Timeout seconds */, 1 /* Glitch seconds */, 1, "../cdr/bigquery/bigquery.backup")

	time.Sleep(2 * time.Second)
	bqw.Close()

	// Get the new number of lines in the table
	newLines := getBQLinesInTable(t)
	if currentLines == newLines {
		t.Fatal("no new lines were detected as inserted")
	}
}

// Helper to get the current number of lines in the table, and compare after doing some insertions
func getBQLinesInTable(t *testing.T) int64 {

	// Create the bigquery client. It will not report any errors until really used
	ctx := context.Background()
	client, projectId, err := getBigqueryClient(ctx)
	if err != nil {
		t.Fatal("could not create client for Google cloud")
	}
	q := client.Query("SELECT count(*) FROM " + projectId + "." + bqdatasetName + "." + bqtableName)

	it, err := q.Read(ctx)
	if err != nil {
		t.Fatal("error reading number of lines in table")
	}
	var values []bigquery.Value
	err = it.Next(&values)
	if err != nil {
		t.Fatal("error reading number of lines in table")
	}

	return values[0].(int64)
}

// Helper to create a bigquery client and the project name
// Use defer .Close() on the Client returned
// Returns a bigquery client, a project_id and an error
func getBigqueryClient(ctx context.Context) (*bigquery.Client, string, error) {
	googleCredentialsFile := os.Getenv("IGOR_CLOUD_CREDENTIALS")

	var cred struct {
		Project_id string
	}

	if credBytes, err := os.ReadFile(googleCredentialsFile); err != nil {
		panic("credentials file " + googleCredentialsFile + " read error: " + err.Error())
	} else {
		json.Unmarshal(credBytes, &cred)
	}

	if cred.Project_id == "" {
		panic("credentials file " + googleCredentialsFile + " could not be parsed ")
	}

	options := option.WithCredentialsFile(googleCredentialsFile)

	client, err := bigquery.NewClient(ctx, cred.Project_id, options)
	return client, cred.Project_id, err
}
