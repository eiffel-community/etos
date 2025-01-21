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

// jobs groups Job instances by their status and provides functions to handle them groupwise
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

// HasConclusion describes a single atomic item that can have a conclusion, such as a single container result
type HasConclusion struct {
	Conclusion Conclusion `json:"conclusion"`
}

// getConclusion returns the conclusion
func (hs HasConclusion) getConclusion() Conclusion {
	return hs.Conclusion
}

// IterableHasConclusion describes an iterable collection of items that can have a joint conclusion, such as a pod consisting of multiple containers
type IterableHasConclusion struct {
	HasConclusion
	Items []HasConclusion `json:"items"`
}

// getConclusion returns the conclusion of a single job based on individual container conclusions
func (ihc IterableHasConclusion) getConclusion() Conclusion {
	hasAborted := false
	hasTimedOut := false
	hasInconclusive := false
	hasFailed := false

	for _, item := range ihc.Items {
		switch item.Conclusion {
		case ConclusionAborted:
			hasAborted = true
		case ConclusionTimedOut:
			hasTimedOut = true
		case ConclusionInconclusive:
			hasInconclusive = true
		case ConclusionFailed:
			hasFailed = true
		}
	}

	if hasAborted {
		// at least one Aborted -> ConclusionAborted
		return ConclusionAborted
	} else if hasTimedOut {
		// at least one TimedOut and no Aborted -> ConclusionTimedOut
		return ConclusionTimedOut
	} else if hasInconclusive {
		// at least one Inconclusive and no Aborted/TimedOut -> ConclusionAborted
		return ConclusionInconclusive
	} else if hasFailed {
		// at least one Failed and no Aborted/TimedOut/Inconclusive -> ConclusionFailed
		return ConclusionFailed
	}
	// no Aborted/TimedOut/Inconclusive/Failed
	return ConclusionSuccessful
}

// HasVerdict describes a single atomic item that can have a verdict, such as a single container result
type HasVerdict struct {
	Verdict Verdict `json:"verdict"`
}

// getConclusion returns the conclusion
func (hv HasVerdict) getVerdict() Verdict {
	return hv.Verdict
}

// IterableHasVerdict describes an iterable collection of items that can have a joint verdict, such as a pod consisting of multiple containers
type IterableHasVerdict struct {
	HasVerdict
	Items []HasVerdict `json:"items"`
}

// getVerdict determines the verdict based on a list of job results
func (ihv IterableHasVerdict) getVerdict() Verdict {
	hasInconclusive := false
	hasFailed := false
	hasNone := false

	for _, result := range ihv.Items {
		switch result.getVerdict() {
		case VerdictInconclusive:
			hasInconclusive = true
		case VerdictNone:
			hasNone = true
		case VerdictFailed:
			hasFailed = true
		}
	}

	if hasInconclusive {
		// at least one Inconclusive -> VerdictInconclusive
		return VerdictInconclusive
	} else if hasNone {
		// at least one None and no Inconclusive -> VerdictNone
		return VerdictNone
	} else if hasFailed {
		// at least one Failed and no Inconclusive/None -> VerdictFailed
		return VerdictFailed
	}
	// no Inconclusive/None/Failed
	return VerdictPassed
}

// Result describes a container inside a pod of an ETOS job
type ContainerResult struct {
	HasConclusion
	HasVerdict
	Conclusion  Conclusion `json:"conclusion"`
	Verdict     Verdict    `json:"verdict,omitempty"`
	Description string     `json:"description,omitempty"`
	Name        string     `json:"name"`
}

// PodResult describes a pod of an ETOS job which consists of one or more containers
type PodResult struct {
	HasConclusion
	HasVerdict
	Name  string
	Items []ContainerResult
}

// JobResult describes an ETOS job which consists of one or more pods
type JobResult struct {
	HasConclusion
	HasVerdict
	Items []PodResult
}

// failed determines if the job has failed (at least one container has failed)
func (jr JobResult) failed() bool {
	for _, pod := range jr.Items {
		for _, result := range pod.Items {
			if result.Conclusion == ConclusionFailed {
				return true
			}
		}
	}
	return false
}

// successful determines if the job has been successful (all containers succeeded)
func (jr JobResult) successful() bool {
	for _, pod := range jr.Items {
		for _, result := range pod.Items {
			if result.Conclusion != ConclusionSuccessful {
				return false
			}
		}
	}
	return true
}

// getContainerResults returns the result of a single container by pod name/name prefix and container name/name prefix (first found)
func (jr JobResult) getContainerResult(podName, containerName string) (ContainerResult, error) {
	for _, pod := range jr.Items {
		if pod.Name == podName {
			for _, result := range pod.Items {
				if result.Name == containerName {
					return result, nil
				}
			}
		}
	}
	return ContainerResult{}, errors.New("pod or container not found with the given name")
}

// JobResults contains methods to handle multiple JobResult instances (such as verdict, conclusion)
type JobResults struct {
	HasConclusion
	HasVerdict
	Items []JobResult
}

// terminationLogs reads termination-log for each pod/container of the given job returning it as a map (keys: pod names, values: Result instances)
func terminationLogs(ctx context.Context, c client.Reader, job *batchv1.Job) (JobResult, error) {
	logger := log.FromContext(ctx)

	var pods corev1.PodList
	if err := c.List(ctx, &pods, client.InNamespace(job.Namespace), client.MatchingLabels{"job-name": job.Name}); err != nil {
		logger.Error(err, fmt.Sprintf("could not list pods for job %s", job.Name))
		return JobResult{}, err
	}

	if len(pods.Items) == 0 {
		return JobResult{}, fmt.Errorf("no pods found for job %s", job.Name)
	}

	var jobResult JobResult
	for _, pod := range pods.Items {

		podResults := PodResult{}
		podResults.Name = pod.Name

		for _, status := range pod.Status.ContainerStatuses {
			if status.State.Terminated == nil {
				podResults.Items = append(podResults.Items, ContainerResult{Conclusion: ConclusionFailed})
				continue
			}

			var result ContainerResult
			if err := json.Unmarshal([]byte(status.State.Terminated.Message), &result); err != nil {
				logger.Error(err, "failed to unmarshal termination log to a result struct")
				podResults.Items = append(podResults.Items, ContainerResult{Conclusion: ConclusionFailed, Description: status.State.Terminated.Message})
				continue
			}

			podResults.Items = append(podResults.Items, result)
		}
		jobResult.Items = append(jobResult.Items, podResults)
	}

	return jobResult, nil
}
