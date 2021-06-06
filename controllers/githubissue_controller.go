/*  Copyright 2021.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package controllers

/*
TODO:
1.add issue name (name of operator) for printing using
2. fix isExist checking that if the issue is close - open it (or maybe fix the update?)
3.watch the guided videos
4. unitesting:
	4.1 create fake client
	4.2 testing
*/

import (
	"context"

	examplev1alpha1 "github.com/AlmogLevii/example-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GitHubIssueReconciler reconciles a GitHubIssue object
type GitHubIssueReconciler struct {
	client.Client
	Log          logr.Logger
	Scheme       *runtime.Scheme
	GitHubClient GitHubClient
}
type IssueData struct {
	Title                string `json:"title"`
	Description          string `json:"body"`
	Number               int    `json:"number,omitempty"`
	State                string `json:"state,,omitempty"`
	LastUpdatedTimeStamp string `json:"updated_at,omitempty"`
}
type OwnerDetails struct {
	Repo  string
	Token string
}

//+kubebuilder:rbac:groups=example.training.redhat.com,resources=githubissues,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=example.training.redhat.com,resources=githubissues/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=example.training.redhat.com,resources=githubissues/finalizers,verbs=update
// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GitHubIssue object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.2/pkg/reconcile
func (r *GitHubIssueReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("githubissue", req.NamespacedName)

	//connect to k8s and get the ghIssue from the server
	ghIssue := examplev1alpha1.GitHubIssue{}
	err := r.Get(ctx, req.NamespacedName, &ghIssue)

	if !requestSucceeded(err) {
		if errors.IsNotFound(err) {
			log.Info("The object is not exist")
			return ctrl.Result{}, nil
		} else {
			log.Info("Other err")
			return ctrl.Result{}, err
		}
	}

	realClient := newRealGitHubClient(ghIssue.Spec.Repo)
	r.GitHubClient = &realClient
	k8sBasedIssue := IssueData{Title: ghIssue.Spec.Title, Description: ghIssue.Spec.Description}

	issueExist, existingIssue, ie := r.GitHubClient.IsExist(k8sBasedIssue)

	r.logMessage(*ie, log)
	if !requestSucceeded(ie.Err) {
		//log.Info(ie.Message)
		//ntc - which err need to be returned
		return ctrl.Result{}, nil
	}

	needToReturn, ie := r.GitHubClient.DeleteIfNeeded(ghIssue, r, issueExist, ctx, *existingIssue)

	r.logMessage(*ie, log)
	if needToReturn {
		return ctrl.Result{}, ie.Err
	}

	var realWorldIssue *IssueData
	if issueExist {
		realWorldIssue, ie = r.GitHubClient.EditIfNeeded(k8sBasedIssue, *existingIssue) //editExistingIssueIfNeeded(k8sBasedIssue, *existingIssue, ownerDetails)
	} else {
		realWorldIssue, ie = r.GitHubClient.Create(k8sBasedIssue) //createNewIssue(k8sBasedIssue, ownerDetails) //r.GitHubClient.create(k8sBasedIssue)
	}

	r.logMessage(*ie, log)
	if !requestSucceeded(ie.Err) {
		//ntc - which err need to be returned
		return ctrl.Result{}, nil
	}

	ie = r.UpdateStatus(ghIssue, *realWorldIssue, ctx)
	r.logMessage(*ie, log)
	if !requestSucceeded(ie.Err) {
		//ntc - which err need to be returned
		return ctrl.Result{}, nil
	}

	/* patch := client.MergeFrom(ghIssue.DeepCopy())
	ghIssue.Status.State = realWorldIssue.State
	ghIssue.Status.LastUpdatedTimeStamp = realWorldIssue.LastUpdatedTimeStamp
	err = r.Client.Status().Patch(ctx, &ghIssue, patch) */

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitHubIssueReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&examplev1alpha1.GitHubIssue{}).
		Complete(r)
}

func (r *GitHubIssueReconciler) logMessage(ie InfoError, log logr.Logger) {

	if !isEmpty(ie.Message) {
		log.Info(ie.Message)
	}
}

func isEmpty(s string) bool {
	return s == ""
}

func (r *GitHubIssueReconciler) UpdateStatus(ghIssue examplev1alpha1.GitHubIssue, realWorldIssue IssueData, ctx context.Context) *InfoError {
	patch := client.MergeFrom(ghIssue.DeepCopy())
	ghIssue.Status.State = realWorldIssue.State
	ghIssue.Status.LastUpdatedTimeStamp = realWorldIssue.LastUpdatedTimeStamp
	err := r.Client.Status().Patch(ctx, &ghIssue, patch)

	ie := InfoError{}
	if !requestSucceeded(err) {
		ie = newInfoError(nil, "Falied to update status")
	}

	return &ie
}
