package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

func getToken() string {
	return os.Getenv("GITOKEN")
}

func getApiUrl(ownerRepo string) string {
	return "https://api.github.com/repos/" + ownerRepo + "/issues"
}

func requestSucceeded(err error) bool {
	return err == nil
}

func isExist(k8sBasedIssue IssueData, ownerDetails OwnerDetails) (bool, *IssueData) {
	var issues []IssueData = getIssuesList(getApiUrl(ownerDetails.Repo))
	for _, issue := range issues {
		if issue.Title == k8sBasedIssue.Title {
			return true, &issue
		}
	}
	return false, &k8sBasedIssue
}

func connectToRealWorld(method string, apiURL string, token string, issue IssueData, statusCode int) *IssueData {
	//make it json
	jsonData, _ := json.Marshal(&issue)
	//creating client to set custom headers for Authorization
	client := &http.Client{}

	req, _ := http.NewRequest(method, apiURL, bytes.NewReader(jsonData))
	req.Header.Set("Authorization", "token "+token)
	resp, err := client.Do(req)

	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != statusCode {
		fmt.Printf("Response code is is %d\n", resp.StatusCode)
		body, _ := ioutil.ReadAll(resp.Body)
		//print body as it may contain hints in case of errors
		fmt.Println(string(body))
		log.Fatal(err)
	}

	var realWorldIssue IssueData
	body, _ := ioutil.ReadAll(resp.Body)
	json.Unmarshal(body, &realWorldIssue)

	return &realWorldIssue
}

func createNewIssue(k8sBasedIssue IssueData, ownerDetails OwnerDetails) *IssueData {
	apiURL := getApiUrl(ownerDetails.Repo)
	realWordIssue := connectToRealWorld("POST", apiURL, ownerDetails.Token, k8sBasedIssue, http.StatusCreated)
	fmt.Printf("Issue \"%s\" was upload successfully\n", k8sBasedIssue.Title)
	return realWordIssue
}

func editExistingIssueIfNeeded(k8sBasedIssue IssueData, existIssue IssueData, ownerDetails OwnerDetails) *IssueData {
	apiURL := getApiUrl(ownerDetails.Repo) + fmt.Sprintf("/%d", existIssue.Number)
	needEdit := existIssue.Description != k8sBasedIssue.Description
	//if no edit was done  we need an update if the state is not the same
	//needUpdate := needEdit || existIssue.State != k8sIssue.State

	if needEdit {
		existIssue.Description = k8sBasedIssue.Description
		realWordIssue := connectToRealWorld("PATCH", apiURL, ownerDetails.Token, existIssue, http.StatusOK)
		fmt.Printf("Issue \"%s\"was edit successfully", existIssue.Title)
		return realWordIssue
	} else {
		return &existIssue
	}
}

func getIssuesList(apiURL string) []IssueData {
	query := "?state=all"
	apiURL += query

	ownerAndRepo := strings.Split(apiURL, "/")
	//make it json
	jsonData, _ := json.Marshal(struct {
		Repo  string
		Owner string
	}{
		Repo:  ownerAndRepo[0],
		Owner: ownerAndRepo[1],
	})
	//creating client to set custom headers for Authorization
	client := &http.Client{}
	req, _ := http.NewRequest("GET", apiURL, bytes.NewReader(jsonData))
	req.Header.Set("Authorization", "token "+getToken())
	resp, err := client.Do(req)

	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	var issues []IssueData
	body, _ := ioutil.ReadAll(resp.Body)
	json.Unmarshal(body, &issues)
	return issues
}

//Close read world github issue associated with the existIssue
func (r *GitHubIssueReconciler) deleteExternalResources(existIssue IssueData, ownerDetails OwnerDetails) error {
	apiURL := getApiUrl(ownerDetails.Repo) + fmt.Sprintf("/%d", existIssue.Number)
	existIssue.State = "closed"
	connectToRealWorld("PATCH", apiURL, ownerDetails.Token, existIssue, http.StatusOK)
	fmt.Printf("Issue \"%s\"was closed successfully\n", existIssue.Title)
	return nil
}

// Helper functions to check and remove string from a slice of strings.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
