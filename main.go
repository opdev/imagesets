package main

import (
	"encoding/json"
	"fmt"
	"github.com/itchyny/gojq"
	. "github.com/opdev/imagesets/libs"
	hive "github.com/openshift/hive/pkg/apis/hive/v1"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/script/v1"
	"google.golang.org/api/sheets/v4"
	"io/ioutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"log"
	"net/http"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"strings"
)

func check(msg string, err error) {
	if err != nil {
		log.Fatalln(msg, err)
	}
}

var ctx = context.Background()

func getImages(imagesSource string) [][]interface{} {
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
			meta["image_id"].(string),
			strings.ReplaceAll(meta["name"].(string), "-x86_64", ""),
			meta["manifest_digest"].(string),
			size,
			meta["last_modified"].(string),
		}
		tags = append(tags, tag)
	}
	return tags
}

func updateSheet(serviceaccount string, sheet string, images [][]interface{}) (status int, err error) {
	// Put images in Google Sheet
	sheetsClient, err := sheets.NewService(
		ctx,
		option.WithCredentialsFile(serviceaccount),
		option.WithScopes("https://www.googleapis.com/auth/spreadsheets"),
	)
	check("Unable to retrieve Google Sheets client: ", err)
	sheetID := sheet
	sheetRange := "imageSets!A1:E" + strconv.Itoa(len(images))
	tags := sheets.ValueRange{
		MajorDimension: "ROWS",
		Range: sheetRange,
		Values: images,
	}
	currentImages := []*sheets.ValueRange{
		&tags,
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

func sortSheet(serviceaccount string, sheet string) (status int, err error) {
	sheetsClient, err := sheets.NewService(
		ctx,
		option.WithCredentialsFile(serviceaccount),
		option.WithScopes("https://www.googleapis.com/auth/spreadsheets"),
	)
	check("Unable to retrieve Google Sheets client: ", err)
	sheetID := sheet
	sortRangeRequest := sheets.SortRangeRequest{
		Range: &sheets.GridRange{
			EndColumnIndex:   4,
			SheetId:          0,
			StartColumnIndex: 0,
			StartRowIndex:    1,
		},
		SortSpecs: []*sheets.SortSpec{
			{DimensionIndex: 1, SortOrder: "DESCENDING"},
		},
	}
	requests := sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{SortRange: &sortRangeRequest},
		},
	}
	sortSheetStatus, err := sheetsClient.Spreadsheets.BatchUpdate(sheetID, &requests).Do()
	check("Unable to sort sheet: ", err)
	if err != nil {
		return sortSheetStatus.HTTPStatusCode, err
	}
	return sortSheetStatus.HTTPStatusCode, nil
}

func updateForm(credentials, token, form string) (status int, err error) {
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

func updateClusterImageSets(images [][]interface{}) {
	cfg, err := DefaultK8sAuthenticate()
	check("Unable to create dynamic client: %v\n", err)

	scheme := runtime.NewScheme()
	err = hive.SchemeBuilder.AddToScheme(scheme)
	check("Unable to add hive to scheme: %v\n", err)

	dc, err := client.New(cfg, client.Options{Scheme: scheme})
	check("Unable to create K8s client: %v\n", err)

	cisl := &hive.ClusterImageSetList{}
	err = dc.List(context.Background(), cisl)
	check("Unable to list ClusterImageSets: %v\n", err)

	currentcis := make([]string, 1)
	for _, v := range cisl.Items {
		currentcis = append(currentcis, v.Spec.ReleaseImage)
	}

	for _, i := range images {
		if i[1] == "name" {
			continue
		}

		for _, v := range currentcis {
			if i[1] == v {
				continue
			}
		}

		imageset := hive.ClusterImageSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ocp-" + strings.ReplaceAll(i[1].(string), "-x86_64", ""),
			},
			Spec:       hive.ClusterImageSetSpec{
				ReleaseImage: "quay.io/openshift-release-dev/ocp-release:" + i[1].(string),
			},
		}

		err = dc.Create(context.Background(), &imageset)
		if err != nil {
			log.Printf("Unable to create ClusterImageSet: %v\n", err)
			continue
		}
	}
}

func main() {
	images := getImages(os.Getenv("IMAGE_SOURCE"))

	updateSheetStatus, err := updateSheet(
		os.Getenv("GOOGLE_SERVICE_ACCOUNT"),
		os.Getenv("GOOGLE_SHEET_ID"),
		images)
	if err != nil {
		log.Fatalf("Unable to update Google Sheet: Status(%v) %v\n", updateSheetStatus, err)
	}
	log.Println("Updated Google Sheet Successfully")

	sortSheetStatus, err := sortSheet(
		os.Getenv("GOOGLE_SERVICE_ACCOUNT"),
		os.Getenv("GOOGLE_SHEET_ID"))
	if err != nil {
		log.Fatalf("Unable to sort Google Sheet: Status(%v) %v\n", sortSheetStatus, err)
	}
	log.Println("Sorted Google Sheet Successfully")

	updateFormStatus, err := updateForm(
		os.Getenv("GOOGLE_CREDENTIALS"),
		os.Getenv("GOOGLE_TOKEN"),
		os.Getenv("GOOGLE_FORM_ID"))
	if err != nil {
		log.Fatalf("Unable to update Google Form: Status(%v) %v\n", updateFormStatus, err)
	}
	log.Println("Updated Google Form Successfully")

	updateClusterImageSets(images)
	log.Println("Updated ClusterImageSets Successfully")
}
