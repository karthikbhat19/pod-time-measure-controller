/*
Copyright 2025.

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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var PodStartupLogPath = "/data/pod_startup_times.json"

// PodStartupReconciler reconciles a PodStartup object
type PodStartupReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	FileLock sync.Mutex
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=pods/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PodStartup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *PodStartupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// We only care about Pods that are scheduled (assigned to a node)
	if pod.Spec.NodeName == "" {
		return ctrl.Result{}, nil
	}

	// Collect important timestamps
	created := pod.CreationTimestamp.Time
	pending := timeZeroSafe(created)
	initialized := getConditionTime(pod, corev1.PodInitialized)
	scheduled := getConditionTime(pod, corev1.PodScheduled)
	containersStarted := getAllContainersStartedTime(pod)
	running := getPhaseTime(pod, corev1.PodRunning)
	ready := getConditionTime(pod, corev1.PodReady)
	succeeded := getPhaseTime(pod, corev1.PodSucceeded)
	failed := getPhaseTime(pod, corev1.PodFailed)

	// Build a structured record
	data := map[string]interface{}{
		"pod":       pod.Name,
		"namespace": pod.Namespace,
		"node":      pod.Spec.NodeName,
		"phase":     string(pod.Status.Phase),
		"timestamps": map[string]string{
			"created":           fmtTime(created),
			"pending":           fmtTime(pending),
			"initialized":       fmtTime(initialized),
			"scheduled":         fmtTime(scheduled),
			"containersStarted": fmtTime(containersStarted),
			"running":           fmtTime(running),
			"ready":             fmtTime(ready),
			"succeeded":         fmtTime(succeeded),
			"failed":            fmtTime(failed),
		},
	}

	// Calculate durations between states
	durations := map[string]string{}
	if !scheduled.IsZero() {
		durations["toScheduled"] = fmt.Sprintf("%v", scheduled.Sub(created))
	}
	if !initialized.IsZero() {
		durations["toInitialized"] = fmt.Sprintf("%v", initialized.Sub(created))
	}
	if !containersStarted.IsZero() {
		durations["toContainersStarted"] = fmt.Sprintf("%v", containersStarted.Sub(created))
	}
	if !ready.IsZero() {
		durations["toReady"] = fmt.Sprintf("%v", ready.Sub(created))
	}
	if !succeeded.IsZero() {
		durations["toSucceeded"] = fmt.Sprintf("%v", succeeded.Sub(created))
	}
	if !failed.IsZero() {
		durations["toFailed"] = fmt.Sprintf("%v", failed.Sub(created))
	}
	data["durations"] = durations

	jsonData, _ := json.MarshalIndent(data, "", "  ")
	logger.Info("Pod lifecycle event", "json", string(jsonData))

	// --- Persist locally (as JSON array) ---
	var allData []map[string]interface{}

	// Lock to prevent race conditions
	r.FileLock.Lock()
	defer r.FileLock.Unlock() // Ensures the lock is released even if a panic occurs

	// If the file already exists and has content, read it
	if existing, err := os.ReadFile(PodStartupLogPath); err == nil && len(existing) > 0 {
		if err := json.Unmarshal(existing, &allData); err != nil {
			// If the file is corrupt, log it and reset
			logger.Error(err, "Failed to unmarshal existing log file, resetting.")
			allData = []map[string]interface{}{} // Reset to empty slice
		}
	}

	// Append this new pod event data
	allData = append(allData, data)

	// Re-marshal everything as a JSON array
	jsonData, _ = json.MarshalIndent(allData, "", "  ")

	// Write back to the file (overwrites but keeps all previous entries)
	if err := os.WriteFile(PodStartupLogPath, jsonData, 0644); err != nil {
		logger.Error(err, "Failed to write updated log file")
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodStartupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&corev1.Pod{}). // watch Pods directly
		Named("podstartup").
		Complete(r)
}

// --- Helper functions ---

func getConditionTime(pod corev1.Pod, condType corev1.PodConditionType) time.Time {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == condType && cond.Status == corev1.ConditionTrue {
			return cond.LastTransitionTime.Time
		}
	}
	return time.Time{}
}

func getPhaseTime(pod corev1.Pod, phase corev1.PodPhase) time.Time {
	if pod.Status.Phase == phase {
		return time.Now()
	}
	return time.Time{}
}

func getAllContainersStartedTime(pod corev1.Pod) time.Time {
	var latest time.Time
	for _, c := range pod.Status.ContainerStatuses {
		if c.State.Running != nil {
			start := c.State.Running.StartedAt.Time
			if start.After(latest) {
				latest = start
			}
		}
	}
	return latest
}

func timeZeroSafe(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now()
	}
	return t
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
