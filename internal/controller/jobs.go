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
	"strings"

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

// Result describes the status and result of a container inside a pod of an ETOS job
type Result struct {
	Name        string     `json:"name"`
	Conclusion  Conclusion `json:"conclusion"`
	Verdict     Verdict    `json:"verdict,omitempty"`
	Description string     `json:"description,omitempty"`
}

// TODO: L127-163 This is a draft implementation of structs and functions to handle results from multiple pods/containers inside an ETOS job:
// JobResults describes the status and result of an ETOS job which consists of one or more pods
type JobResults struct {
	PodResults []PodResults
}

// PodResults describes the status and result of a single pod of an ETOS job which consists of one or more containers
type PodResults struct {
	Name             string
	ContainerResults []Result
}

// failed determines if the job has failed (at least one container has failed)
func (jr JobResults) failed() bool {
	for _, pod := range jr.PodResults {
		for _, result := range pod.ContainerResults {
			if result.Conclusion == ConclusionFailed {
				return true
			}
		}
	}
	return false
}

// successful determines if the job has been successful (all containers succeeded)
func (jr JobResults) successful() bool {
	for _, pod := range jr.PodResults {
		for _, result := range pod.ContainerResults {
			if result.Conclusion != ConclusionSuccessful {
				return false
			}
		}
	}
	return true
}

// getConclusion returns the conclusion of a job based on individual container conclusions
func (jr JobResults) getConclusion() Conclusion {
	for _, pod := range jr.PodResults {
		for _, result := range pod.ContainerResults {
			if result.Conclusion == ConclusionFailed {
				return ConclusionFailed
			} else if result.Conclusion == ConclusionAborted {
				return ConclusionAborted
			} else if result.Conclusion == ConclusionInconclusive {
				return ConclusionInconclusive
			} else if result.Conclusion == ConclusionTimedOut {
				return ConclusionInconclusive
			}
		}
	}
	return ConclusionSuccessful
}

// getVerdict returns the verdict of a job based on individual container verdicts
func (jr JobResults) getVerdict() Verdict {
	for _, pod := range jr.PodResults {
		for _, result := range pod.ContainerResults {
			if result.Verdict == VerdictFailed {
				return VerdictFailed
			} else if result.Verdict == VerdictInconclusive {
				return VerdictInconclusive
			} else if result.Verdict == VerdictNone {
				return VerdictNone
			}
		}
	}
	return VerdictPassed
}

// TODO: should be possible to do by exact name matching, no need to have prefixes
// getContainerResults returns the result of a single container by pod name/name prefix and container name/name prefix (first found)
func (jr JobResults) getContainerResults(podNamePrefix, containerNamePrefix string) (Result, error) {
	for _, pod := range jr.PodResults {
		if strings.HasPrefix(pod.Name, podNamePrefix) {
			for _, result := range pod.ContainerResults {
				if strings.HasPrefix(result.Name, containerNamePrefix) {
					return result, nil
				}
			}
		}
	}
	return Result{}, errors.New("pod or container not found with the given name prefix")
}

// terminationLogs reads termination-log for each pod/container of the given job returning it as a map (keys: pod names, values: Result instances)
func terminationLogs(ctx context.Context, c client.Reader, job *batchv1.Job) (JobResults, error) {
	logger := log.FromContext(ctx)

	var pods corev1.PodList
	if err := c.List(ctx, &pods, client.InNamespace(job.Namespace), client.MatchingLabels{"job-name": job.Name}); err != nil {
		logger.Error(err, fmt.Sprintf("could not list pods for job %s", job.Name))
		return JobResults{}, err
	}

	if len(pods.Items) == 0 {
		return JobResults{}, fmt.Errorf("no pods found for job %s", job.Name)
	}

	var jobResults JobResults
	for _, pod := range pods.Items {

		podResults := PodResults{}
		podResults.Name = pod.Name

		for _, status := range pod.Status.ContainerStatuses {
			if status.State.Terminated == nil {
				podResults.ContainerResults = append(podResults.ContainerResults, Result{Conclusion: ConclusionFailed})
				continue
			}

			var result Result

			if err := json.Unmarshal([]byte(status.State.Terminated.Message), &result); err != nil {
				logger.Error(err, "failed to unmarshal termination log to a result struct")
				podResults.ContainerResults = append(podResults.ContainerResults, Result{Conclusion: ConclusionFailed, Description: status.State.Terminated.Message})
				continue
			}

			podResults.ContainerResults = append(podResults.ContainerResults, result)
		}
		jobResults.PodResults = append(jobResults.PodResults, podResults)
	}

	return jobResults, nil
}
