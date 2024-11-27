// Copyright Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type jobs struct {
	activeJobs     []*batchv1.Job
	successfulJobs []*batchv1.Job
	failedJobs     []*batchv1.Job
}

func (j jobs) active() bool {
	return len(j.successfulJobs) == 0 && len(j.failedJobs) == 0 && len(j.activeJobs) > 0
}

func (j jobs) failed() bool {
	return len(j.failedJobs) > 0 && len(j.activeJobs) == 0
}

func (j jobs) successful() bool {
	return len(j.successfulJobs) > 0 && len(j.activeJobs) == 0
}

func (j jobs) empty() bool {
	return len(j.successfulJobs) == 0 && len(j.failedJobs) == 0 && len(j.activeJobs) == 0
}

// jobStatus creates a job struct with jobs that are active, failed or successful.
func jobStatus(ctx context.Context, c client.Reader, namespace string, name string, ownerKey string) (*jobs, error) {
	var active []*batchv1.Job
	var successful []*batchv1.Job
	var failed []*batchv1.Job
	var joblist batchv1.JobList

	if err := c.List(ctx, &joblist, client.InNamespace(namespace), client.MatchingFields{ownerKey: name}); err != nil {
		return &jobs{}, err
	}

	for i, job := range joblist.Items {
		_, finishedType := isJobFinished(job)
		switch finishedType {
		case "": // Ongoing
			active = append(active, &joblist.Items[i])
		case batchv1.JobFailed:
			failed = append(failed, &joblist.Items[i])
		case batchv1.JobComplete:
			successful = append(successful, &joblist.Items[i])
		}
	}
	return &jobs{activeJobs: active, failedJobs: failed, successfulJobs: successful}, nil
}

// isJobFinished checks if a job has status Complete or Failed.
func isJobFinished(job batchv1.Job) (bool, batchv1.JobConditionType) {
	if IsJobStatusConditionPresentAndEqual(job.Status.Conditions, batchv1.JobComplete, corev1.ConditionTrue) {
		return true, batchv1.JobComplete
	}
	if IsJobStatusConditionPresentAndEqual(job.Status.Conditions, batchv1.JobFailed, corev1.ConditionTrue) {
		return true, batchv1.JobFailed
	}
	return false, ""
}

// IsStatusConditionPresentAndEqual returns true when conditionType is present and equal to status.
func IsJobStatusConditionPresentAndEqual(conditions []batchv1.JobCondition, conditionType batchv1.JobConditionType, status corev1.ConditionStatus) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition.Status == status
		}
	}
	return false
}

type (
	Conclusion string
	Verdict    string
)

const (
	ConclusionSuccessful   Conclusion = "Successful"
	ConclusionFailed       Conclusion = "Failed"
	ConclusionAborted      Conclusion = "Aborted"
	ConclusionTimedOut     Conclusion = "TimedOut"
	ConclusionInconclusive Conclusion = "Inconclusive"
)

const (
	VerdictPassed       Verdict = "Passed"
	VerdictFailed       Verdict = "Failed"
	VerdictInconclusive Verdict = "Inconclusive"
	VerdictNone         Verdict = "None"
)

// Result describes the status and result of an ETOS job
type Result struct {
	Conclusion  Conclusion `json:"conclusion"`
	Verdict     Verdict    `json:"verdict,omitempty"`
	Description string     `json:"description,omitempty"`
}

// terminationLog reads the termination-log part of the ESR pod and returns it.
func terminationLog(ctx context.Context, c client.Reader, job *batchv1.Job) (*Result, error) {
	logger := log.FromContext(ctx)
	var pods corev1.PodList
	if err := c.List(ctx, &pods, client.InNamespace(job.Namespace), client.MatchingLabels{"job-name": job.Name}); err != nil {
		logger.Error(err, fmt.Sprintf("could not list pods for job %s", job.Name))
		return &Result{Conclusion: ConclusionFailed}, err
	}
	if len(pods.Items) == 0 {
		return &Result{Conclusion: ConclusionFailed}, fmt.Errorf("no pods found for job %s", job.Name)
	}
	if len(pods.Items) > 1 {
		// TODO: check specific
		logger.Info("found more than 1 pod active. Will only check termination-log for the first one", "pod", pods.Items[0])
	}
	pod := pods.Items[0]

	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == job.Name || status.Name == job.Labels["etos.eiffel-community.github.io/name"] {
			if status.State.Terminated == nil {
				return &Result{Conclusion: ConclusionFailed}, errors.New("could not read termination log from pod")
			}
			var result Result
			if err := json.Unmarshal([]byte(status.State.Terminated.Message), &result); err != nil {
				logger.Error(err, "failed to unmarshal termination log to a result struct")
				return &Result{Conclusion: ConclusionFailed, Description: status.State.Terminated.Message}, nil
			}
			return &result, nil
		}
	}
	return &Result{Conclusion: ConclusionFailed}, errors.New("found no container status for pod")
}
