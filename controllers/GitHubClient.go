package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	examplev1alpha1 "github.com/AlmogLevii/example-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type GitHubClient interface {
	IsExist(k8sBasedIssue IssueData) (bool, *IssueData, *InfoError)
	Create(k8sBasedIssue IssueData) (*IssueData, *InfoError)
	EditIfNeeded(k8sBasedIssue IssueData, existIssue IssueData) (*IssueData, *InfoError)
	Close(existIssue IssueData) *InfoError
	DeleteIfNeeded(ghIssue examplev1alpha1.GitHubIssue, r *GitHubIssueReconciler, issueExist bool, ctx context.Context, existingIssue IssueData) (bool, *InfoError)
}

type RealGitHubClient struct {
	httpClient http.Client
	token      string
	repo       string
}

func newRealGitHubClient(repoURL string) RealGitHubClient {
	return RealGitHubClient{
		httpClient: http.Client{},
		token:      os.Getenv("GITOKEN"),
		repo:       repoURL,
	}
}

func (rc *RealGitHubClient) Create(k8sBasedIssue IssueData) (*IssueData, *InfoError) {
	apiURL := getApiUrl(rc.repo)
	jsonData, _ := json.Marshal(&k8sBasedIssue)
	var realWorldIssue IssueData

	body, ie := rc.connect("POST", apiURL, jsonData, http.StatusCreated)

	if requestSucceeded(ie.Err) {
		json.Unmarshal(body, &realWorldIssue)
		fmt.Println("indecation1")
		iep := newInfoError(nil, "Issue was post successfully")
		ie = &iep
	}

	return &realWorldIssue, ie
}

func (rc *RealGitHubClient) EditIfNeeded(k8sBasedIssue IssueData, existIssue IssueData) (*IssueData, *InfoError) {
	apiURL := getApiUrl(rc.repo) + fmt.Sprintf("/%d", existIssue.Number)
	needEdit := existIssue.Description != k8sBasedIssue.Description || existIssue.State != k8sBasedIssue.State //ntc change the status

	var realWorldIssue *IssueData
	ie := &(InfoError{})

	if needEdit {
		existIssue.Description = k8sBasedIssue.Description
		jsonData, _ := json.Marshal(&existIssue)
		body, ie := rc.connect("PATCH", apiURL, jsonData, http.StatusOK)

		if requestSucceeded(ie.Err) {
			json.Unmarshal(body, &realWorldIssue)
			iep := newInfoError(nil, "Issue was edit successfully")
			ie = &iep
		}

	} else {
		realWorldIssue = &existIssue
	}

	return realWorldIssue, ie
}

func (rc *RealGitHubClient) IsExist(k8sBasedIssue IssueData) (bool, *IssueData, *InfoError) {

	exist := false
	existingIssue := &k8sBasedIssue

	issues, ie := rc.getIssuesList(getApiUrl(rc.repo))

	if requestSucceeded(ie.Err) {

		for _, issue := range issues {
			if issue.Title == k8sBasedIssue.Title {
				exist = true
				existingIssue = &issue
				break
			}
		}
	}

	return exist, existingIssue, ie
}

func (rc *RealGitHubClient) Close(existIssue IssueData) *InfoError {

	apiURL := getApiUrl(rc.repo) + fmt.Sprintf("/%d", existIssue.Number)
	existIssue.State = "closed"
	jsonData, _ := json.Marshal(&existIssue)

	_, ie := rc.connect("PATCH", apiURL, jsonData, http.StatusOK)

	return ie
}

func (rc *RealGitHubClient) getIssuesList(apiURL string) ([]IssueData, *InfoError) {
	query := "?state=all"
	apiURL += query

	ownerAndRepo := strings.Split(apiURL, "/")

	jsonData, _ := json.Marshal(struct {
		Repo  string
		Owner string
	}{
		Repo:  ownerAndRepo[0],
		Owner: ownerAndRepo[1],
	})

	var issues []IssueData

	body, ie := rc.connect("GET", apiURL, jsonData, http.StatusOK)

	if requestSucceeded(ie.Err) {
		json.Unmarshal(body, &issues)
	}
	return issues, ie
}

func (rc *RealGitHubClient) connect(method string, apiURL string, jsonData []byte, statusCode int) ([]byte, *InfoError) {
	client := rc.httpClient
	req, _ := http.NewRequest(method, apiURL, bytes.NewReader(jsonData))
	req.Header.Set("Authorization", "token "+rc.token)
	resp, err := client.Do(req)
	var ie InfoError
	var body []byte

	if !requestSucceeded(err) {
		ie = newInfoError(err, fmt.Sprintf("failed to connect with %s method", method))
	} else {

		defer resp.Body.Close()

		if resp.StatusCode != statusCode {
			ie = newInfoError(err, "status code is differnet from the expected")
		} else {
			body, _ = ioutil.ReadAll(resp.Body)
		}
	}

	return body, &ie
}

func (rc *RealGitHubClient) DeleteIfNeeded(ghIssue examplev1alpha1.GitHubIssue, r *GitHubIssueReconciler, issueExist bool, ctx context.Context, existingIssue IssueData) (bool, *InfoError) {
	needToReturn := false
	ie := InfoError{}
	finalizer := "example.training.redhat.com/finalizer"

	if ghIssue.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object.
		// This is equivalent registering our finalizer.
		if !containsString(ghIssue.GetFinalizers(), finalizer) {
			controllerutil.AddFinalizer(&ghIssue, finalizer)
			if err := r.Update(ctx, &ghIssue); err != nil {
				ie = newInfoError(err, "failed to update the finalizer")
				needToReturn = true
				//return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if containsString(ghIssue.GetFinalizers(), finalizer) {
			// our finalizer is present, so lets handle any external dependency
			// if the issue isn't on github, skip the external handle and just remove finalizer
			if issueExist {
				if ierr := rc.Close(existingIssue); ierr.Err != nil {
					// if fail to delete the external dependency here, return with error
					// so that it can be retried
					ie = newInfoError(ierr.Err, "failed to delete the external dependency")
					needToReturn = true
					//return ctrl.Result{}, err
				}
			}
			if !needToReturn {
				// remove our finalizer from the list and update it.
				controllerutil.RemoveFinalizer(&ghIssue, finalizer)
				if err := r.Update(ctx, &ghIssue); err != nil {
					ie = newInfoError(err, "failed to update the list after removal our finalizer")
					needToReturn = true
					//return ctrl.Result{}, err
				}
			}
		}
		// Stop reconciliation as the item is being deleted
		needToReturn = true
		ie = newInfoError(nil, "Issue was deleted successfully")
	}

	return needToReturn, &ie
}

/* func (rc *RealGitHubClient) UpdateStatus(ghIssue examplev1alpha1.GitHubIssue, realWorldIssue IssueData, ctx context.Context, r *GitHubIssueReconciler) *InfoError {

	patch := client.MergeFrom(ghIssue.DeepCopy())
	ghIssue.Status.State = realWorldIssue.State
	ghIssue.Status.LastUpdatedTimeStamp = realWorldIssue.LastUpdatedTimeStamp
	err := r.Client.Status().Patch(ctx, &ghIssue, patch)

	ie := InfoError{}
	if !requestSucceeded(err) {
		ie = newInfoError(err, "Falied to update issue")
	}

	return &ie
} */
