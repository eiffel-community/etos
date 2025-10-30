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
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// job helps manage jobs for an owner
type job struct {
	client.Client
	ownerKey  string
	name      string
	namespace string

	activeJobs     []*batchv1.Job
	successfulJobs []*batchv1.Job
	failedJobs     []*batchv1.Job
}

// NewJob creates a new job manager
func NewJob(c client.Client, ownerKey, name, namespace string) Job {
	return &job{Client: c, ownerKey: ownerKey, name: name, namespace: namespace}
}

// Status gets the current status the jobs
func (j *job) Status(ctx context.Context) (Status, error) {
	if err := j.jobStatus(ctx); err != nil {
		return StatusNone, err
	}
	if j.active() {
		return StatusActive, nil
	}
	if j.successful() {
		return StatusSuccessful, nil
	}
	if j.failed() {
		return StatusFailed, nil
	}
	if j.empty() {
		return StatusNone, nil
	}
	return StatusNone, errors.New("found no status on job")
}

// Result returns the result of either successful or failed jobs depending on the current status of the jobs
func (j *job) Result(ctx context.Context) Result {
	var jobs []*batchv1.Job
	if j.successful() {
		jobs = j.successfulJobs
	} else if j.failed() {
		jobs = j.failedJobs
	}

	result := Result{Verdict: VerdictNone}
	for _, job := range jobs {
		jobResult, err := terminationLog(ctx, j, job, j.name)
		if err != nil {
			jobResult.Description = err.Error()
		}
		if jobResult.Description == "" {
			jobResult.Description = fmt.Sprintf("No description on pod %s - Unknown error", j.name)
		}
		result.Description = fmt.Sprintf("%s; %s: %s", result.Description, job.Name, jobResult.Description)
		// Only set conclusion if it is not already failed.
		if jobResult.Conclusion == ConclusionFailed && result.Conclusion != ConclusionFailed {
			result.Conclusion = ConclusionFailed
		}
		if jobResult.Verdict == VerdictFailed && result.Verdict != VerdictFailed {
			result.Verdict = jobResult.Verdict
		} else if result.Verdict == VerdictNone {
			result.Verdict = jobResult.Verdict
		}
	}
	if result.Conclusion != ConclusionFailed {
		result.Conclusion = ConclusionSuccessful
	}
	return result
}

// Create creates a new job
func (j job) Create(ctx context.Context, obj client.Object, jobSpecFunc JobSpecFunc) error {
	jobSpec, err := jobSpecFunc(ctx, obj)
	if err != nil {
		return fmt.Errorf("failed to create job spec: %v", err)
	}
	return j.Client.Create(ctx, jobSpec)
}

// Delete all owned jobs
func (j job) Delete(ctx context.Context) error {
	var multiErr error
	for _, job := range j.all() {
		if err := j.Client.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			if !apierrors.IsNotFound(err) {
				multiErr = errors.Join(multiErr, err)
			}
		}
	}
	return multiErr
}

// all returns all jobs owned by the ownerKey in this job manager
func (j job) all() []*batchv1.Job {
	jobs := []*batchv1.Job{}
	jobs = append(jobs, j.activeJobs...)
	jobs = append(jobs, j.failedJobs...)
	jobs = append(jobs, j.successfulJobs...)
	return jobs
}

// active returns true if there are any active jobs and no failed or successful jobs
func (j job) active() bool {
	return len(j.successfulJobs) == 0 && len(j.failedJobs) == 0 && len(j.activeJobs) > 0
}

// failed returns true if there are failed jobs and no active jobs
func (j job) failed() bool {
	return len(j.failedJobs) > 0 && len(j.activeJobs) == 0
}

// successful returns true if there are successful jobs and no active jobs
func (j job) successful() bool {
	return len(j.successfulJobs) > 0 && len(j.activeJobs) == 0
}

// empty returns true if there are no jobs
func (j job) empty() bool {
	return len(j.successfulJobs) == 0 && len(j.failedJobs) == 0 && len(j.activeJobs) == 0
}

// jobStatus creates a job struct with jobs that are active, failed or successful.
func (j *job) jobStatus(ctx context.Context) error {
	var active []*batchv1.Job
	var successful []*batchv1.Job
	var failed []*batchv1.Job
	var joblist batchv1.JobList

	if err := j.List(ctx, &joblist, client.InNamespace(j.namespace), client.MatchingFields{j.ownerKey: j.name}); err != nil {
		return err
	}
	logger := logf.FromContext(ctx)
	logger.Info("Jobs", "count", len(joblist.Items))

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
	j.activeJobs = active
	j.failedJobs = failed
	j.successfulJobs = successful
	return nil
}

// isJobFinished checks if a job has status Complete or Failed.
func isJobFinished(job batchv1.Job) (bool, batchv1.JobConditionType) {
	if isJobStatusConditionPresentAndEqual(job.Status.Conditions, batchv1.JobComplete, corev1.ConditionTrue) {
		return true, batchv1.JobComplete
	}
	if isJobStatusConditionPresentAndEqual(job.Status.Conditions, batchv1.JobFailed, corev1.ConditionTrue) {
		return true, batchv1.JobFailed
	}
	return false, ""
}

// isStatusConditionPresentAndEqual returns true when conditionType is present and equal to status.
func isJobStatusConditionPresentAndEqual(conditions []batchv1.JobCondition, conditionType batchv1.JobConditionType, status corev1.ConditionStatus) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition.Status == status
		}
	}
	return false
}

// getLatestPodByCreationTimestamp returns the latest created pod from the given list
func getLatestPodByCreationTimestamp(pods []v1.Pod) *v1.Pod {
	if len(pods) == 0 {
		return nil
	}
	latestPod := pods[0]
	for _, pod := range pods[1:] {
		if pod.CreationTimestamp.After(latestPod.CreationTimestamp.Time) {
			latestPod = pod
		}
	}
	return &latestPod
}

// terminationLog reads the termination-log part of the ESR pod and returns it.
func terminationLog(ctx context.Context, c client.Reader, job *batchv1.Job, containerName string) (*Result, error) {
	logger := log.FromContext(ctx)
	var pods corev1.PodList
	if err := c.List(ctx, &pods, client.InNamespace(job.Namespace), client.MatchingLabels{"job-name": job.Name}); err != nil {
		logger.Error(err, fmt.Sprintf("could not list pods for job %s", job.Name))
		return &Result{Conclusion: ConclusionFailed}, err
	}
	if len(pods.Items) == 0 {
		return &Result{Conclusion: ConclusionFailed}, fmt.Errorf("no pods found for job %s", job.Name)
	}
	pod := getLatestPodByCreationTimestamp(pods.Items)
	logger.Info("Reading termination-log from pod: ", "pod", pod)

	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == containerName {
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
