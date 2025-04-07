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
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	ref "k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

// TODO: Move Environment, EnvironmentRequestOwnerKey
var (
	TestRunOwnerKey            = ".metadata.controller.suiterunner"
	EnvironmentRequestOwnerKey = ".metadata.controller.environmentrequest"
	EnvironmentOwnerKey        = ".metadata.controller.environment"
	APIGroupVersionString      = etosv1alpha1.GroupVersion.String()
	iutProvider                = ".spec.providers.iut"
	executionSpaceProvider     = ".spec.providers.executionSpace"
	logAreaProvider            = ".spec.providers.logarea"
)

// TestRunReconciler reconciles a TestRun object
type TestRunReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Clock
	Cluster *etosv1alpha1.Cluster
}

/*
We'll mock out the clock to make it easier to jump around in time while testing,
the "real" clock just calls `time.Now`.
*/
type realClock struct{}

func (_ realClock) Now() time.Time { return time.Now() }

// Clock knows how to get the current time.
// It can be used to fake out timing for testing.
type Clock interface {
	Now() time.Time
}

// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=testruns,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=testruns/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=testruns/finalizers,verbs=update
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environments,verbs=get;watch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environments/status,verbs=get
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environmentrequests,verbs=get;watch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environmentsrequests/status,verbs=get
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=providers,verbs=get;watch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=providers/status,verbs=get
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *TestRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger = logger.WithValues("namespace", req.Namespace, "name", req.Name)

	testrun := &etosv1alpha1.TestRun{}
	if err := r.Get(ctx, req.NamespacedName, testrun); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Testrun not found, exiting")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Error getting testrun")
		return ctrl.Result{}, err
	}
	if testrun.Status.CompletionTime != nil {
		testrunCondition := meta.FindStatusCondition(testrun.Status.Conditions, StatusActive)
		var retention *metav1.Duration
		if testrunCondition.Reason == "Successful" {
			retention = testrun.Spec.Retention.Success
		} else {
			retention = testrun.Spec.Retention.Failure
		}
		if retention == nil {
			logger.Info("No retention set, ignoring")
			return ctrl.Result{}, nil
		}
		if testrun.Status.CompletionTime.Add(retention.Duration).Before(time.Now()) {
			logger.Info(fmt.Sprintf("Testrun TTL(%s) reached, delete", retention.Duration))
			if err := r.Delete(ctx, testrun, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
				if !apierrors.IsNotFound(err) {
					logger.Error(err, "Failed deletion. Ignoring any errors and won't retry")
				}
			}
		} else {
			next := testrun.Status.CompletionTime.Add(retention.Duration).Sub(r.Now())
			logger.Info(fmt.Sprintf("Testrun queued for deletion in %s", next))
			return ctrl.Result{RequeueAfter: next}, nil
		}
		return ctrl.Result{}, nil

	}
	clusterNamespacedName := types.NamespacedName{
		Name:      testrun.Spec.Cluster,
		Namespace: req.NamespacedName.Namespace,
	}
	cluster := &etosv1alpha1.Cluster{}
	if err := r.Get(ctx, clusterNamespacedName, cluster); err != nil {
		logger.Info("Failed to get cluster resource!")
		return ctrl.Result{}, nil
	}

	if err := r.reconcile(ctx, cluster, testrun); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Reconciliation failed for testrun", "namespace", req.Namespace, "name", req.Name)
		return r.update(ctx, testrun, metav1.ConditionFalse, err.Error())
	}

	return ctrl.Result{}, nil
}

func (r *TestRunReconciler) reconcile(ctx context.Context, cluster *etosv1alpha1.Cluster, testrun *etosv1alpha1.TestRun) error {
	logger := log.FromContext(ctx)

	// Set initial statuses if not set.
	if meta.FindStatusCondition(testrun.Status.Conditions, StatusActive) == nil {
		meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Status: metav1.ConditionFalse, Type: StatusActive, Message: "Reconciliation started", Reason: "Pending"})
		return r.Status().Update(ctx, testrun)
	}
	if meta.FindStatusCondition(testrun.Status.Conditions, StatusSuiteRunner) == nil {
		meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Status: metav1.ConditionFalse, Type: StatusSuiteRunner, Message: "Reconciliation started", Reason: "Pending"})
		return r.Status().Update(ctx, testrun)
	}
	if meta.FindStatusCondition(testrun.Status.Conditions, StatusEnvironment) == nil {
		meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Status: metav1.ConditionFalse, Type: StatusEnvironment, Message: "Reconciliation started", Reason: "Pending"})
		return r.Status().Update(ctx, testrun)
	}

	// Get active, finished and failed suite runners.
	suiteRunners, err := jobStatus(ctx, r, testrun.Namespace, testrun.Name, TestRunOwnerKey)
	if err != nil {
		return err
	}

	// Add active suite runners to testrun status
	testrun.Status.SuiteRunners = nil
	for _, activeSuiteRunner := range suiteRunners.activeJobs {
		jobRef, err := ref.GetReference(r.Scheme, activeSuiteRunner)
		if err != nil {
			logger.Error(err, "failed to make reference to active suite runner", "suiterunner", activeSuiteRunner)
			continue
		}
		testrun.Status.SuiteRunners = append(testrun.Status.SuiteRunners, *jobRef)
	}
	if err := r.Status().Update(ctx, testrun); err != nil {
		return err
	}
	logger.V(1).Info("suite runner count", "active", len(suiteRunners.activeJobs), "successful", len(suiteRunners.successfulJobs), "failed", len(suiteRunners.failedJobs))

	// Check providers availability
	if err := checkProviders(ctx, r, testrun.Namespace, testrun.Spec.Providers); err != nil {
		return err
	}

	// Create environment request
	err, exit := r.reconcileEnvironmentRequest(ctx, cluster, testrun)
	if err != nil {
		return err
	}
	if exit {
		return nil
	}

	// Check environment
	if err := r.checkEnvironment(ctx, testrun); err != nil {
		return err
	}

	// Reconcile suite runners
	if err := r.reconcileSuiteRunner(ctx, suiteRunners, testrun); err != nil {
		return err
	}

	// Set testrun status
	if suiteRunners.active() {
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusActive, Status: metav1.ConditionTrue, Reason: "Active", Message: "Waiting for suite runners to finish"}) {
			return r.Status().Update(ctx, testrun)
		}
	}
	if suiteRunners.failed() {
		if err := r.complete(ctx, testrun, "Failed", "Suite runners failed to finish"); err != nil {
			return err
		}
	}
	if suiteRunners.successful() {
		if err := r.complete(ctx, testrun, "Successful", "Suite runners finished successfully"); err != nil {
			return err
		}
	}

	return nil
}

// complete sets the completion time and active status on a testrun.
func (r *TestRunReconciler) complete(ctx context.Context, testrun *etosv1alpha1.TestRun, reason, message string) error {
	if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusActive, Status: metav1.ConditionFalse, Reason: reason, Message: message}) {
		testrunCondition := meta.FindStatusCondition(testrun.Status.Conditions, StatusActive)
		testrun.Status.CompletionTime = &testrunCondition.LastTransitionTime
		return r.Status().Update(ctx, testrun)
	}
	return nil
}

// reconcileEnvironmentRequest will check the status of environment requests, create new ones if necessary.
func (r *TestRunReconciler) reconcileEnvironmentRequest(ctx context.Context, cluster *etosv1alpha1.Cluster, testrun *etosv1alpha1.TestRun) (error, bool) {
	logger := log.FromContext(ctx)
	var environmentRequestList etosv1alpha1.EnvironmentRequestList
	if err := r.List(ctx, &environmentRequestList, client.InNamespace(testrun.Namespace), client.MatchingFields{TestRunOwnerKey: testrun.Name}); err != nil {
		if !apierrors.IsNotFound(err) {
			return err, true
		}
	}
	testrun.Status.EnvironmentRequests = nil
	for _, request := range environmentRequestList.Items {
		reqRef, err := ref.GetReference(r.Scheme, &request)
		if err != nil {
			logger.Error(err, "failed to make reference to active environment request", "environmentrequest", request)
			continue
		}
		testrun.Status.EnvironmentRequests = append(testrun.Status.EnvironmentRequests, *reqRef)
	}
	for _, suite := range testrun.Spec.Suites {
		found := false
		for _, request := range environmentRequestList.Items {
			if request.Spec.Name == suite.Name {
				found = true
			}
		}
		if !found {
			request := r.environmentRequest(cluster, testrun, suite)
			if err := ctrl.SetControllerReference(testrun, request, r.Scheme); err != nil {
				return err, true
			}
			logger.Info("Creating a new environment request", "request", request.Name)
			if err := r.Create(ctx, request); err != nil {
				return err, true
			}
		}
	}

	for _, environmentRequest := range environmentRequestList.Items {
		condition := meta.FindStatusCondition(environmentRequest.Status.Conditions, StatusReady)
		if condition != nil && condition.Status == metav1.ConditionFalse && condition.Reason == "Failed" {
			if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusEnvironment, Status: metav1.ConditionFalse, Reason: "Failed", Message: "Failed to create environment for test"}) {
				return r.Status().Update(ctx, testrun), true
			}
			if err := r.complete(ctx, testrun, "Failed", condition.Message); err != nil {
				return err, true
			}
			return nil, true
		} else if condition != nil && condition.Status == metav1.ConditionFalse {
			logger.Info("Environment request is not finished")
		}
	}
	return nil, false
}

// reconcileSuiteRunner will check the status of suite runners, create new ones if necessary.
func (r *TestRunReconciler) reconcileSuiteRunner(ctx context.Context, suiteRunners *jobs, testrun *etosv1alpha1.TestRun) error {
	logger := log.FromContext(ctx)
	// Suite runners failed, setting status.
	if suiteRunners.failed() {
		suiteRunner := suiteRunners.failedJobs[0] // TODO
		result, err := terminationLog(ctx, r, suiteRunner, testrun.Name)
		if err != nil {
			result.Description = err.Error()
		}
		logger.Info("Suite runner result", "verdict", result.Verdict, "conclusion", result.Conclusion, "message", result.Description)
		if result.Verdict == "" {
			result.Verdict = VerdictNone
		}
		testrun.Status.Verdict = string(result.Verdict)
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusSuiteRunner, Status: metav1.ConditionFalse, Reason: "Failed", Message: result.Description}) {
			return r.Status().Update(ctx, testrun)
		}
	}
	// Suite runners successful, setting status.
	if suiteRunners.successful() {
		suiteRunner := suiteRunners.successfulJobs[0] // TODO
		result, err := terminationLog(ctx, r, suiteRunner, testrun.Name)
		if err != nil {
			result.Description = err.Error()
		}
		logger.Info("Suite runner result", "verdict", result.Verdict, "conclusion", result.Conclusion, "message", result.Description)
		if result.Verdict == "" {
			result.Verdict = VerdictNone
		}
		testrun.Status.Verdict = string(result.Verdict)
		if result.Conclusion == ConclusionFailed {
			if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusSuiteRunner, Status: metav1.ConditionFalse, Reason: "Failed", Message: result.Description}) {
				return r.Status().Update(ctx, testrun)
			}
		}
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusSuiteRunner, Status: metav1.ConditionFalse, Reason: "Done", Message: "Suite runner finished"}) {
			for _, suiteRunner := range suiteRunners.successfulJobs {
				// TODO: Deletion should probably be done outside of this function
				if err := r.Delete(ctx, suiteRunner, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
					if !apierrors.IsNotFound(err) {
						return err
					}
				}
				if err = r.deleteEnvironmentRequests(ctx, testrun); err != nil {
					return err
				}
			}
			return r.Status().Update(ctx, testrun)
		}
	}
	// Suite runners active, setting status
	if suiteRunners.active() {
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusSuiteRunner, Status: metav1.ConditionTrue, Reason: "Running", Message: "Suite runner is running"}) {
			testrun.Status.Verdict = string(VerdictNone)
			logger.Info("Setting suiterunner (active) true")
			return r.Status().Update(ctx, testrun)
		}
	}
	// No suite runners, create suite runner
	if suiteRunners.empty() {
		suiteRunner := r.suiteRunnerJob(testrun)
		if err := ctrl.SetControllerReference(testrun, suiteRunner, r.Scheme); err != nil {
			return err
		}
		if err := r.Create(ctx, suiteRunner); err != nil {
			return err
		}
	}
	return nil
}

// deleteEnvironmentRequests will delete all environment requests that are a part of a testrun.
func (r *TestRunReconciler) deleteEnvironmentRequests(ctx context.Context, testrun *etosv1alpha1.TestRun) error {
	var environmentRequestList etosv1alpha1.EnvironmentRequestList
	if err := r.List(ctx, &environmentRequestList, client.InNamespace(testrun.Namespace), client.MatchingFields{TestRunOwnerKey: testrun.Name}); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	}
	for _, environmentRequest := range environmentRequestList.Items {
		if err := r.Delete(ctx, &environmentRequest, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}

// update will set the status condition and update the status of the ETOS testrun.
// if the update fails due to conflict the reconciliation will requeue.
func (r *TestRunReconciler) update(ctx context.Context, testrun *etosv1alpha1.TestRun, status metav1.ConditionStatus, message string) (ctrl.Result, error) {
	// TODO: Verify this function.
	if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusActive, Status: status, Reason: "Active", Message: message}) {
		if err := r.Status().Update(ctx, testrun); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// checkEnvironment will check the status of the environment used for test.
func (r *TestRunReconciler) checkEnvironment(ctx context.Context, testrun *etosv1alpha1.TestRun) error {
	logger := log.FromContext(ctx)
	logger.Info("Checking environment")
	var environments etosv1alpha1.EnvironmentList
	if err := r.List(ctx, &environments, client.InNamespace(testrun.Namespace), client.MatchingFields{TestRunOwnerKey: testrun.Name}); err != nil {
		return err
	}
	logger.Info("Listed environments", "environments", len(environments.Items))
	// TODO: this only checks one environment, not all of them if there are many
	if len(environments.Items) > 0 {
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusEnvironment, Status: metav1.ConditionTrue, Reason: "Ready", Message: "Environment ready"}) {
			logger.Info("Set condition")
			return r.Status().Update(ctx, testrun)
		}
	}
	return nil
}

// environmentRequest is the definition for an environment request.
func (r TestRunReconciler) environmentRequest(cluster *etosv1alpha1.Cluster, testrun *etosv1alpha1.TestRun, suite etosv1alpha1.Suite) *etosv1alpha1.EnvironmentRequest {
	eventRepository := cluster.Spec.EventRepository.Host
	if cluster.Spec.ETOS.Config.ETOSEventRepositoryURL != "" {
		eventRepository = cluster.Spec.ETOS.Config.ETOSEventRepositoryURL
	}

	eiffelMessageBus := cluster.Spec.MessageBus.EiffelMessageBus
	if cluster.Spec.MessageBus.EiffelMessageBus.Deploy {
		// eiffelMessageBus.Host = fmt.Sprintf("%s-%s", cluster.Name, eiffelMessageBus.Host)
		eiffelMessageBus.Host = fmt.Sprintf("%s-%s", cluster.Name, "rabbitmq")
	}
	etosMessageBus := cluster.Spec.MessageBus.ETOSMessageBus
	if cluster.Spec.MessageBus.ETOSMessageBus.Deploy {
		// etosMessageBus.Host = fmt.Sprintf("%s-%s", cluster.Name, etosMessageBus.Host)
		etosMessageBus.Host = fmt.Sprintf("%s-%s", cluster.Name, "messagebus")
	}

	databaseHost := cluster.Spec.Database.Etcd.Host
	if databaseHost == "" {
		databaseHost = "etcd-client"
	}
	if cluster.Spec.Database.Deploy {
		databaseHost = fmt.Sprintf("%s-etcd-client", cluster.Name)
	}

	databasePort := cluster.Spec.Database.Etcd.Port
	if databasePort == "" {
		databasePort = "2379"
	}

	return &etosv1alpha1.EnvironmentRequest{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"etos.eiffel-community.github.io/id":      testrun.Spec.ID,
				"etos.eiffel-community.github.io/cluster": testrun.Spec.Cluster,
				"app.kubernetes.io/name":                  "suite-runner",
				"app.kubernetes.io/part-of":               "etos",
			},
			Annotations:  make(map[string]string),
			GenerateName: fmt.Sprintf("%s-", testrun.Name),
			Namespace:    testrun.Namespace,
		},
		Spec: etosv1alpha1.EnvironmentRequestSpec{
			ID:            string(uuid.NewUUID()),
			Name:          suite.Name,
			Identifier:    testrun.Spec.ID,
			Artifact:      testrun.Spec.Artifact,
			Identity:      testrun.Spec.Identity,
			MinimumAmount: 1,
			MaximumAmount: len(suite.Tests),
			Dataset:       suite.Dataset,
			Providers: etosv1alpha1.EnvironmentProviders{
				IUT: etosv1alpha1.IutProvider{
					ID: testrun.Spec.Providers.IUT,
				},
				ExecutionSpace: etosv1alpha1.ExecutionSpaceProvider{
					ID:         testrun.Spec.Providers.ExecutionSpace,
					TestRunner: testrun.Spec.TestRunner.Version,
				},
				LogArea: etosv1alpha1.LogAreaProvider{
					ID: testrun.Spec.Providers.LogArea,
				},
			},
			Splitter: etosv1alpha1.Splitter{
				Tests: suite.Tests,
			},
			Image:              testrun.Spec.EnvironmentProvider.Image,
			ServiceAccountName: fmt.Sprintf("%s-provider", testrun.Spec.Cluster),
			Config: etosv1alpha1.EnvironmentProviderJobConfig{
				EiffelMessageBus:                    eiffelMessageBus,
				EtosMessageBus:                      etosMessageBus,
				EtosApi:                             cluster.Spec.ETOS.Config.ETOSApiURL,
				EncryptionKey:                       cluster.Spec.ETOS.Config.EncryptionKey,
				RoutingKeyTag:                       cluster.Spec.ETOS.Config.RoutingKeyTag,
				GraphQlServer:                       eventRepository,
				EtcdHost:                            databaseHost,
				EtcdPort:                            databasePort,
				WaitForTimeout:                      cluster.Spec.ETOS.Config.EnvironmentTimeout,
				EnvironmentProviderEventDataTimeout: cluster.Spec.ETOS.Config.EventDataTimeout,
				EnvironmentProviderImage:            cluster.Spec.ETOS.EnvironmentProvider.Image.Image,
				EnvironmentProviderImagePullPolicy:  cluster.Spec.ETOS.EnvironmentProvider.ImagePullPolicy,
				EnvironmentProviderServiceAccount:   fmt.Sprintf("%s-provider", cluster.Name),
				EnvironmentProviderTestSuiteTimeout: cluster.Spec.ETOS.Config.TestSuiteTimeout,
				TestRunnerVersion:                   cluster.Spec.ETOS.TestRunner.Version,
			},
		},
	}
}

// suiteRunnerJob is the job definition for an etos suite runner.
func (r TestRunReconciler) suiteRunnerJob(testrun *etosv1alpha1.TestRun) *batchv1.Job {
	grace := int64(30)
	backoff := int32(0)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"etos.eiffel-community.github.io/id": testrun.Spec.ID,
				"app.kubernetes.io/name":             "suite-runner",
				"app.kubernetes.io/part-of":          "etos",
			},
			Annotations: make(map[string]string),
			Name:        testrun.Name,
			Namespace:   testrun.Namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: testrun.Name,
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "kubexit",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "graveyard",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory},
							},
						},
					},
					TerminationGracePeriodSeconds: &grace,
					ServiceAccountName:            fmt.Sprintf("%s-provider", testrun.Spec.Cluster),
					RestartPolicy:                 "Never",
					InitContainers: []corev1.Container{
						{
							Name:    "kubexit",
							Image:   "karlkfi/kubexit:latest",
							Command: []string{"cp", "/bin/kubexit", "/kubexit/kubexit"},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("64Mi"),
									corev1.ResourceCPU:    resource.MustParse("50m"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("32Mi"),
									corev1.ResourceCPU:    resource.MustParse("25m"),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "kubexit",
									MountPath: "/kubexit",
								},
							},
						},
						{
							Name:            "create-queue",
							Image:           testrun.Spec.LogListener.Image.Image,
							ImagePullPolicy: testrun.Spec.LogListener.ImagePullPolicy,
							Command:         []string{"python", "-u", "-m", "create_queue"},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
									corev1.ResourceCPU:    resource.MustParse("50m"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("128Mi"),
									corev1.ResourceCPU:    resource.MustParse("25m"),
								},
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: fmt.Sprintf("%s-etos-suite-runner-cfg", testrun.Spec.Cluster),
										},
									},
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "IDENTIFIER",
									Value: testrun.Spec.ID,
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            testrun.Name,
							Image:           testrun.Spec.SuiteRunner.Image.Image,
							ImagePullPolicy: testrun.Spec.SuiteRunner.ImagePullPolicy,
							Command:         []string{"/kubexit/kubexit"},
							Args:            []string{"python", "-u", "-m", "etos_suite_runner"},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
									corev1.ResourceCPU:    resource.MustParse("200m"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("128Mi"),
									corev1.ResourceCPU:    resource.MustParse("100m"),
								},
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: fmt.Sprintf("%s-etos-suite-runner-cfg", testrun.Spec.Cluster),
										},
									},
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "ARTIFACT",
									Value: testrun.Spec.Artifact,
								},
								{
									Name:  "IDENTITY",
									Value: testrun.Spec.Identity,
								},
								{
									Name:  "ETOS_ROUTING_KEY_TAG",
									Value: fmt.Sprintf("%s-%s", testrun.Namespace, testrun.Spec.Cluster),
								},
								{
									Name:  "TESTRUN",
									Value: testrun.Name,
								},
								{
									Name:  "IDENTIFIER",
									Value: testrun.Spec.ID,
								},
								{
									Name:  "KUBEXIT_NAME",
									Value: "esr",
								},
								{
									Name:  "KUBEXIT_GRAVEYARD",
									Value: "/graveyard",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "graveyard",
									MountPath: "/graveyard",
								},
								{
									Name:      "kubexit",
									MountPath: "/kubexit",
								},
							},
						},
						{
							Name:            "etos-log-listener",
							Image:           testrun.Spec.LogListener.Image.Image,
							ImagePullPolicy: testrun.Spec.LogListener.ImagePullPolicy,
							Command:         []string{"/kubexit/kubexit"},
							Args:            []string{"python", "-u", "-m", "log_listener"},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
									corev1.ResourceCPU:    resource.MustParse("50m"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("128Mi"),
									corev1.ResourceCPU:    resource.MustParse("25m"),
								},
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: fmt.Sprintf("%s-etos-suite-runner-cfg", testrun.Spec.Cluster),
										},
									},
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "IDENTIFIER",
									Value: testrun.Spec.ID,
								},
								{
									Name:  "KUBEXIT_NAME",
									Value: "log_listener",
								},
								{
									Name:  "KUBEXIT_GRAVE_PERIOD",
									Value: "400s",
								},
								{
									Name:  "KUBEXIT_GRAVEYARD",
									Value: "/graveyard",
								},
								{
									Name:  "KUBEXIT_DEATH_DEPS",
									Value: "esr",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "graveyard",
									MountPath: "/graveyard",
								},
								{
									Name:      "kubexit",
									MountPath: "/kubexit",
								},
							},
						},
					},
				},
			},
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *TestRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Set up real clock, since we are not in a test
	if r.Clock == nil {
		r.Clock = realClock{}
	}

	// Register indexes for faster lookups
	if err := r.registerOwnerIndexForJob(mgr); err != nil {
		return err
	}
	if err := r.registerOwnerIndexForEnvironment(mgr); err != nil {
		return err
	}
	if err := r.registerOwnerIndexForEnvironmentRequest(mgr); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &etosv1alpha1.TestRun{}, iutProvider, func(rawObj client.Object) []string {
		testrun := rawObj.(*etosv1alpha1.TestRun)
		return []string{testrun.Spec.Providers.IUT}
	}); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &etosv1alpha1.TestRun{}, logAreaProvider, func(rawObj client.Object) []string {
		testrun := rawObj.(*etosv1alpha1.TestRun)
		return []string{testrun.Spec.Providers.LogArea}
	}); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &etosv1alpha1.TestRun{}, executionSpaceProvider, func(rawObj client.Object) []string {
		testrun := rawObj.(*etosv1alpha1.TestRun)
		return []string{testrun.Spec.Providers.ExecutionSpace}
	}); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&etosv1alpha1.TestRun{}).
		Owns(&batchv1.Job{}).
		Owns(&etosv1alpha1.EnvironmentRequest{}).
		Watches(
			&etosv1alpha1.Provider{},
			handler.TypedEnqueueRequestsFromMapFunc(r.findTestrunsForIUTProvider),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&etosv1alpha1.Provider{},
			handler.TypedEnqueueRequestsFromMapFunc(r.findTestrunsForExecutionSpaceProvider),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&etosv1alpha1.Provider{},
			handler.TypedEnqueueRequestsFromMapFunc(r.findTestrunsForLogAreaProvider),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&etosv1alpha1.Environment{},
			handler.TypedEnqueueRequestsFromMapFunc(r.findTestrunsForEnvironment),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

// registerOwnerIndexForJob will set an index of the suite runner jobs that a testrun owns.
func (r *TestRunReconciler) registerOwnerIndexForJob(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &batchv1.Job{}, TestRunOwnerKey, func(rawObj client.Object) []string {
		job := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != APIGroupVersionString || owner.Kind != "TestRun" {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}
	return nil
}

// registerOwnerIndexForEnvironment will set an index of the environments that a testrun owns.
func (r *TestRunReconciler) registerOwnerIndexForEnvironment(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &etosv1alpha1.Environment{}, TestRunOwnerKey, func(rawObj client.Object) []string {
		environment := rawObj.(*etosv1alpha1.Environment)
		owner := metav1.GetControllerOf(environment)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != APIGroupVersionString || owner.Kind != "TestRun" {
			return nil
		}
		return []string{owner.Name}
	}); err != nil {
		return err
	}
	return nil
}

// registerOwnerIndexForEnvironmentRequest will set an index of the environment requests that a testrun owns.
func (r *TestRunReconciler) registerOwnerIndexForEnvironmentRequest(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &etosv1alpha1.EnvironmentRequest{}, TestRunOwnerKey, func(rawObj client.Object) []string {
		environmentRequest := rawObj.(*etosv1alpha1.EnvironmentRequest)
		owner := metav1.GetControllerOf(environmentRequest)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != APIGroupVersionString || owner.Kind != "TestRun" {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}
	return nil
}

// findTestrunsForIUTProvider will return reconciliation requests for each Provider object that a testrun has stored
// in its spec as IUT. This will cause reconciliations whenever a Provider gets updated, created, deleted etc.
func (r *TestRunReconciler) findTestrunsForIUTProvider(ctx context.Context, provider client.Object) []reconcile.Request {
	return r.findTestrunsForProvider(ctx, iutProvider, provider)
}

// findTestrunsForIUTProvider will return reconciliation requests for each Provider object that a testrun has stored
// in its spec as execution space. This will cause reconciliations whenever a Provider gets updated, created, deleted etc.
func (r *TestRunReconciler) findTestrunsForExecutionSpaceProvider(ctx context.Context, provider client.Object) []reconcile.Request {
	return r.findTestrunsForProvider(ctx, executionSpaceProvider, provider)
}

// findTestrunsForIUTProvider will return reconciliation requests for each Provider object that a testrun has stored
// in its spec as log area. This will cause reconciliations whenever a Provider gets updated, created, deleted etc.
func (r *TestRunReconciler) findTestrunsForLogAreaProvider(ctx context.Context, provider client.Object) []reconcile.Request {
	return r.findTestrunsForProvider(ctx, logAreaProvider, provider)
}

// findTestrunsForProvider will find testruns for a providerName.
func (r *TestRunReconciler) findTestrunsForProvider(ctx context.Context, providerName string, provider client.Object) []reconcile.Request {
	testrunList := &etosv1alpha1.TestRunList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(providerName, provider.GetName()),
		Namespace:     provider.GetNamespace(),
	}
	err := r.List(ctx, testrunList, listOps)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(testrunList.Items))
	for i, item := range testrunList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		}
	}
	return requests
}

// findTestrunsForEnvironment will return reconciliation requests for Environment objects with a specific testrun as
// owner.
func (r *TestRunReconciler) findTestrunsForEnvironment(ctx context.Context, environment client.Object) []reconcile.Request {
	// TODO: Since we are setting controller to false when creating Environment in the environment provider
	// we need to find the TestRun owner in another way. This way is much more insecure, but since we are going
	// to create environments from the controllers later we wuill be able to fix this to be more in line with
	// how registerOwnerIndexForJob does it.
	refs := environment.GetOwnerReferences()
	for i := range refs {
		if refs[i].APIVersion == APIGroupVersionString && refs[i].Kind == "TestRun" {
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      refs[i].Name,
						Namespace: environment.GetNamespace(),
					},
				},
			}
		}
	}
	return nil
}
