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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ---------------- The actual test ----------------
var _ = Describe("PodStartupReconciler", func() {
	It("should detect a pod transition and record its details", func() {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				CreationTimestamp: metav1.Time{
					Time: time.Now().Add(-5 * time.Second),
				},
			},
			Spec: corev1.PodSpec{
				NodeName: "fake-node",
				Containers: []corev1.Container{
					{Name: "c1", Image: "busybox"},
				},
			},
		}

		By("Creating the test pod (initially Pending)")
		Expect(k8sClient.Create(context.Background(), pod)).To(Succeed())

		// Manually update pod status → Running
		By("Updating pod status to Running")
		Eventually(func() error {
			var existing corev1.Pod
			if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(pod), &existing); err != nil {
				return err
			}
			existing.Status = corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodScheduled, Status: corev1.ConditionTrue, LastTransitionTime: metav1.Now()},
					{Type: corev1.PodInitialized, Status: corev1.ConditionTrue, LastTransitionTime: metav1.Now()},
					{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: metav1.Now()},
				},
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "c1",
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{StartedAt: metav1.Now()},
						},
					},
				},
			}
			return k8sClient.Status().Update(context.Background(), &existing)
		}, 5*time.Second, 500*time.Millisecond).Should(Succeed())

		time.Sleep(1 * time.Second) // Have to remove this, Currently it takes time for the controller to write to the file

		By("Waiting for reconciliation to happen")
		Eventually(func() bool {
			data, err := os.ReadFile("./test_pod_startup_times.json")
			return err == nil && len(data) > 0
		}, 10*time.Second, 1*time.Second).Should(BeTrue(), "controller should have written JSON data")

		By("Verifying JSON structure")
		data, err := os.ReadFile("./test_pod_startup_times.json")
		Expect(err).ToNot(HaveOccurred())

		var arr []map[string]interface{}
		Expect(json.Unmarshal(data, &arr)).To(Succeed())

		latest := arr[len(arr)-1] // pick the most recent pod event
		// Print the latest event for debugging
		fmt.Fprintf(GinkgoWriter, "Latest pod event: %+v\n", latest["phase"])

		Expect(latest["pod"]).To(Equal("test-pod"))
		Expect(latest["namespace"]).To(Equal("default"))
		Expect(latest["phase"]).To(Equal("Running"))
	})
})
