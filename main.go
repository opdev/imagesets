package main

import (
	"encoding/json"
	"fmt"
	"github.com/itchyny/gojq"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
    "golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/script/v1"
	"google.golang.org/api/sheets/v4"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
)

func check(msg string, err error) {
	if err != nil {
		log.Fatalln(msg, err)
	}
}

var ctx = context.Background()

func getTags(imagesSource string) [][]interface{} {
	resp, err := http.Get(imagesSource)
	check("Unable to query image source API: ", err)

	body, err := ioutil.ReadAll(resp.Body)
	check("Unable to read response body: ", err)
	defer resp.Body.Close()

	repository := make(map[string]interface{})

	if json.Unmarshal(body, &repository) != nil {
		log.Fatalln(err)
	}

	query, err := gojq.Parse(".tags|with_entries(select(.key|match(\"x86_64\")))")
	check("Unable to compile gojq query string: ", err)

	images := query.Run(repository)
	imageSet, _ := images.Next()

	tags := [][]interface{}{
		{"imageId", "name", "manifestDigest", "size", "lastModified"},
	}

	for _, imageMeta := range imageSet.(map[string]interface{}) {
		meta := imageMeta.(map[string]interface{})
		size := fmt.Sprintf("%.f", meta["size"].(float64))

		tag := []interface{}{
			meta["image_id"].(string), meta["name"].(string), meta["manifest_digest"].(string), size, meta["last_modified"].(string),
		}
		tags = append(tags, tag)
	}

	return tags
}

func updateSheet(serviceaccount string, sheet string, tags [][]interface{}) (status int, err error){
	// Put images in Google Sheet
	sheetsClient, err := sheets.NewService(
		ctx,
		option.WithCredentialsFile(serviceaccount),
		option.WithScopes("https://www.googleapis.com/auth/spreadsheets"),
	)
	check("Unable to retrieve Google Sheets client: ", err)
	// TODO: environment variable
	sheetID := sheet
	sheetRange := "imageSets!A1:E" + strconv.Itoa(len(tags))
	images := sheets.ValueRange{
		MajorDimension: "ROWS",
		Range: sheetRange,
		Values: tags,
	}
	currentImages := []*sheets.ValueRange{
		&images,
	}

	batchUpdate := sheets.BatchUpdateValuesRequest{
		Data: currentImages,
		ValueInputOption: "RAW",
	}
	updateStatus, err := sheetsClient.Spreadsheets.Values.BatchUpdate(sheetID, &batchUpdate).Context(ctx).Do()
	check("Unable to update sheet: ", err)
	if err != nil {
		return updateStatus.HTTPStatusCode, err
	}
	return updateStatus.HTTPStatusCode, nil
}

func updateForm(credentials, token, form string) (status int, err error){
	credentialsFileBytes, err := ioutil.ReadFile(credentials)
	check("Unable to read credentials file: ", err)

	credentialsConfig, err := google.ConfigFromJSON(credentialsFileBytes,
		"https://www.googleapis.com/auth/script.projects",
		"https://www.googleapis.com/auth/forms",
		"https://www.googleapis.com/auth/spreadsheets")
	check("Unable to create config from credentials file: ", err)

	// Push image names to Google Form (OpenShift Version drop down)
	// TODO: EET-1010 Create AppScript
	tokenFile, err := os.Open(token)
	check("Unable to open token file: ", err)
	defer tokenFile.Close()

	tokenJSON := &oauth2.Token{}
	err = json.NewDecoder(tokenFile).Decode(tokenJSON)
	check("Unable to parse token file: ", err)

	credentialsClient := credentialsConfig.Client(ctx, tokenJSON)

	scriptsClient, err := script.NewService(ctx, option.WithHTTPClient(credentialsClient))
	check("Unable to retrieve Google Apps Script client: ", err)

	er := &script.ExecutionRequest{ Function: "main" }

	scriptsRunStatus, err := scriptsClient.Scripts.Run(form, er).Do()
	check("Unable to run script function: ", err)

	return scriptsRunStatus.HTTPStatusCode, err
}

func main() {
	// Get latest images from source
	tags := getTags(os.Getenv("IMAGE_SOURCE"))

	updateSheetStatus, err := updateSheet(
		os.Getenv("GOOGLE_SERVICE_ACCOUNT"),
		os.Getenv("GOOGLE_SHEET_ID"),
		tags)
	if err != nil {
		log.Fatalf("Unable to update Google Sheet: %v", err)
	}

	if updateSheetStatus == 200 {
		updateFormStatus, err := updateForm(
			os.Getenv("GOOGLE_CREDENTIALS"),
			os.Getenv("GOOGLE_TOKEN"),
			os.Getenv("GOOGLE_FORM_ID"))
		if err != nil {
			log.Fatalf("Unable to update Google Form: Status(%v) %v", updateFormStatus, err)
		}
	}
}
