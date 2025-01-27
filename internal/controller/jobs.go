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

// ContainerResult describes a container inside a pod of an ETOS job
type Result struct {
       Results     []Result
       Conclusion  Conclusion
       Verdict     Verdict
       Description string `json:"description,omitempty"`
       Name        string `json:"name"`
}

// getConclusion returns the conclusion of a single job based on individual container conclusions
func (r Result) getConclusion() Conclusion {
	if len(r.Results) == 0 {
		return r.Conclusion
	}

	hasAborted := false
	hasTimedOut := false
	hasInconclusive := false
	hasFailed := false

	for _, item := range r.Results {
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

// getVerdict determines the verdict based on a list of job results
func (r Result) getVerdict() Verdict {
	if len(r.Results) == 0 {
		return r.Verdict
	}
	hasInconclusive := false
	hasFailed := false
	hasNone := false
	for _, result := range r.Results {
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

// getContainerResults returns the result of a single container by pod/container name
func (r Result) getContainerResult(podName, containerName string) (Result, error) {
	for _, pod := range r.Results {
		if pod.Name == podName {
			for _, result := range pod.Results {
				if result.Name == containerName {
					return result, nil
				}
			}
		}
	}
	return Result{}, errors.New("pod or container not found with the given name")
}

// terminationLogs reads termination-log for each pod/container of the given job returning it as a JobResult instance
func terminationLogs(ctx context.Context, c client.Reader, job *batchv1.Job) (Result, error) {
	logger := log.FromContext(ctx)

	var pods corev1.PodList
	if err := c.List(ctx, &pods, client.InNamespace(job.Namespace), client.MatchingLabels{"job-name": job.Name}); err != nil {
		logger.Error(err, fmt.Sprintf("could not list pods for job %s", job.Name))
		return Result{}, err
	}

	if len(pods.Items) == 0 {
		return Result{}, fmt.Errorf("no pods found for job %s", job.Name)
	}

	var jobResult Result
	for _, pod := range pods.Items {

		podResults := Result{}
		podResults.Name = pod.Name

		for _, status := range pod.Status.ContainerStatuses {
			if status.State.Terminated == nil {
				podResults.Results = append(podResults.Results, Result{Name: status.Name, Conclusion: ConclusionFailed})
				continue
			}

			var containerResult Result
			t_msg := status.State.Terminated.Message
			// The termination message may not be available for some containers such as etos-log-listener, which are not included in the output.
			if t_msg != "" {
				if err := json.Unmarshal([]byte(t_msg), &containerResult); err != nil {
					msg := fmt.Sprintf("failed to unmarshal termination log to a result struct: %s: '%s'", status.Name, t_msg)
					logger.Error(err, msg)
					podResults.Results = append(podResults.Results, Result{Name: status.Name, Conclusion: ConclusionFailed, Description: t_msg})
					continue
				}
			}
			containerResult.Name = status.Name
			podResults.Results = append(podResults.Results, containerResult)
		}
		jobResult.Results = append(jobResult.Results, podResults)
	}

	return jobResult, nil
}
