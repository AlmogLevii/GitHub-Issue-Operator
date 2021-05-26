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

import (
	"context"
	"fmt"

	examplev1alpha1 "github.com/AlmogLevii/example-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// GitHubIssueReconciler reconciles a GitHubIssue object
type GitHubIssueReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
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
	fmt.Print("\n")
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

	ownerDetails := OwnerDetails{Repo: ghIssue.Spec.Repo, Token: getToken()}
	k8sBasedIssue := IssueData{Title: ghIssue.Spec.Title, Description: ghIssue.Spec.Description}

	issueExist, existingIssue := isExist(k8sBasedIssue, ownerDetails)

	//Name our finalizer
	finalizer := "example.training.redhat.com/finalizer"

	// examine DeletionTimestamp to determine if object is under deletion
	if ghIssue.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !containsString(ghIssue.GetFinalizers(), finalizer) {
			controllerutil.AddFinalizer(&ghIssue, finalizer)
			if err := r.Update(ctx, &ghIssue); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if containsString(ghIssue.GetFinalizers(), finalizer) {
			// our finalizer is present, so lets handle any external dependency
			// if the issue isn't on github, skip the external handle and just remove finalizer
			if issueExist {
				if err := r.deleteExternalResources(existingIssue, ownerDetails); err != nil {
					// if fail to delete the external dependency here, return with error
					// so that it can be retried
					return ctrl.Result{}, err
				}
			}
			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(&ghIssue, finalizer)
			if err := r.Update(ctx, &ghIssue); err != nil {
				return ctrl.Result{}, err
			}
		}
		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	var realWorldIssue *IssueData

	if issueExist {
		realWorldIssue = editExistingIssueIfNeeded(&k8sBasedIssue, existingIssue, ownerDetails)
	} else {
		realWorldIssue = createNewIssue(&k8sBasedIssue, ownerDetails)
	}

	patch := client.MergeFrom(ghIssue.DeepCopy())
	ghIssue.Status.State = realWorldIssue.State
	ghIssue.Status.LastUpdatedTimeStamp = realWorldIssue.LastUpdatedTimeStamp
	err = r.Client.Status().Patch(ctx, &ghIssue, patch)

	if !requestSucceeded(err) {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitHubIssueReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&examplev1alpha1.GitHubIssue{}).
		Complete(r)
}
