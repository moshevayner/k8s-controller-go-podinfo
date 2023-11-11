/*
Copyright 2023.

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
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	podinfoappv1 "github.com/moshevayner/k8s-controller-go-podinfo/api/v1"
)

// PodInfoInstanceReconciler reconciles a PodInfoInstance object
type PodInfoInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=podinfo-app.podinfo.vayner.me,resources=podinfoinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=podinfo-app.podinfo.vayner.me,resources=podinfoinstances/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=podinfo-app.podinfo.vayner.me,resources=podinfoinstances/finalizers,verbs=update

// Additional Permissions to control apps/v1/Deployment objects
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *PodInfoInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.V(5).Info(fmt.Sprintf("Req: %v", req))

	// TODO(moshe): (Generic across all go-client invocations): Add retry logic for possible network issues, 429, etc., so that in case of a transient error, the controller will retry the API call

	// Fetch the PodInfoInstance instance. If the request fails, return an error unless it's a 404 (Not Found Error), in which case we can just return
	pii := &podinfoappv1.PodInfoInstance{}
	err := r.Get(ctx, req.NamespacedName, pii)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			l.V(5).Info(fmt.Sprintf("PodInfoInstance %v:%v not found in the cluster, which means it was probably deleted. Ignoring this error and moving on..", req.NamespacedName, pii))
			return ctrl.Result{}, nil
		} else {
			return ctrl.Result{}, err
		}
	}
	l.V(5).Info(fmt.Sprintf("Got PodInfoInstance %v:%v", req.NamespacedName, pii))

	// If the PodInfoInstance does not have a Deployment yet, create one (with underlying resources as needed)
	if pii.Status.AppDeployment.Name == "" {
		l.V(5).Info(fmt.Sprintf("PodInfoInstance %v does not have a Deployment yet, creating one", pii.Name))
		err := r.CreateDeploymentForPodInfoInstance(ctx, pii)
		l.V(5).Info("Updating PodInfoInstance status")
		uErr := r.Status().Update(ctx, pii)
		if uErr != nil {
			l.Error(uErr, fmt.Sprintf("Error while attempting to update PodInfoInstance %v status", pii.Name))
		}
		return ctrl.Result{}, err
	} else {
		l.V(5).Info(fmt.Sprintf("PodInfoInstance %v already has a Deployment: %v. Checking if it needs an update", pii.Name, pii.Status.AppDeployment.Name))
		err = r.CheckAndUpdateExistingDeploymentAsNeeded(ctx, pii)
		l.V(5).Info("Updating PodInfoInstance status")
		uErr := r.Status().Update(ctx, pii)
		if uErr != nil {
			l.Error(uErr, fmt.Sprintf("Error while attempting to update PodInfoInstance %v status", pii.Name))
		}
		return ctrl.Result{}, err
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodInfoInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&podinfoappv1.PodInfoInstance{}).
		Complete(r)
}

// CreateDeploymentForPodInfoInstance creates a new Deployment for the given PodInfoInstance
func (r *PodInfoInstanceReconciler) CreateDeploymentForPodInfoInstance(ctx context.Context, pii *podinfoappv1.PodInfoInstance) error {
	l := log.FromContext(ctx)
	// Create a Deployment object with the needed fields (name, namespace, labels, owner reference, etc.)
	d := generateDeploymentSpecForPodInfoInstance(pii)

	// If Redis is enabled, invoke AddRedisToDeployment to add a Redis Deployment and Service and configure the Pod's environment variables
	if pii.Spec.Redis.Enabled {
		l.V(5).Info(fmt.Sprintf("Redis is enabled for PodInfoInstance %s, creating Redis Deployment and Service", pii.Name))
		err := r.CreateRedisDeploymentAndService(ctx, &d.Spec.Template, pii)
		if err != nil {
			return err
		}
	}

	l.V(5).Info(fmt.Sprintf("Creating App Deployment %s", d.Name))
	err := r.Create(ctx, d)
	if err != nil {
		l.Error(err, fmt.Sprintf("Error while attempting to create App Deployment %s", d.Name))
		pii.Status.AppDeployment.Errors = append(pii.Status.AppDeployment.Errors, fmt.Sprintf("Error creating Deployment %s: %v", d.Name, err))
		return err
	}

	s := generateServiceSpecForPodInfoInstance(pii)
	l.V(5).Info(fmt.Sprintf("Creating App Service %s", s.Name))
	err = r.Create(ctx, s)
	if err != nil {
		l.Error(err, fmt.Sprintf("Error while attempting to create App Service %s", s.Name))
		pii.Status.AppService.Errors = append(pii.Status.AppService.Errors, fmt.Sprintf("Error creating Service %s: %v", s.Name, err))
		return err
	}

	// Update the PodInfoInstance's status with the name of the Deployment
	pii.Status.AppDeployment.Name = d.Name
	pii.Status.AppService.Name = s.Name
	return nil
}

// CreateRedisDeploymentAndService creates a new Redis Deployment and adds the Service's Host and Port to the PodInfoInstance's Deployment's Pod's environment variables
func (r *PodInfoInstanceReconciler) CreateRedisDeploymentAndService(ctx context.Context, pts *corev1.PodTemplateSpec, pii *podinfoappv1.PodInfoInstance) error {
	l := log.FromContext(ctx)
	// Create a new Redis Deployment
	rd := generateRedisDeploymentSpecForPodInfoInstance(pii)
	// Invoke the API call to create the Redis Deployment
	err := r.Create(ctx, rd)
	if err != nil {
		l.Error(err, fmt.Sprintf("Error while attempting to create Redis Deployment %s", rd.Name))
		pii.Status.RedisDeployment.Errors = append(pii.Status.RedisDeployment.Errors, fmt.Sprintf("Error creating Redis Deployment %s: %v", rd.Name, err))
		return err
	}

	// Update the PodInfoInstance's status with the name of the Redis Deployment
	pii.Status.RedisDeployment.Name = rd.Name

	// Create a new Redis Service
	rs := generateRedisServiceSpecForPodInfoInstance(pii)
	// Invoke the API call to create the Redis Service
	err = r.Create(ctx, rs)
	if err != nil {
		l.Error(err, fmt.Sprintf("Error while attempting to create Redis Service %s", rs.Name))
		pii.Status.RedisService.Errors = append(pii.Status.RedisService.Errors, fmt.Sprintf("Error creating Redis Service %s: %v", rs.Name, err))
		return err
	}

	// Update the PodInfoInstance's status with the name of the Redis Service
	pii.Status.RedisService.Name = rs.Name
	pii.Status.RedisService.Errors = nil

	pts.Spec.Containers[0].Env = append(pts.Spec.Containers[0].Env, corev1.EnvVar{
		Name:  "PODINFO_CACHE_SERVER",
		Value: fmt.Sprintf("tcp://%s:%d", rs.Name, rs.Spec.Ports[0].Port),
	})

	return nil
}

// CheckAndUpdateExistingDeploymentAsNeeded checks the existing Deployment for the given PodInfoInstance and updates it as needed
// This function involves a few steps:
// 1. Check if the main App Deployment needs to be updated and update it as needed
// 2. If there's an existing Redis Deployment and Redis is still enabled, check if it needs to be updated and update it as needed
// 3. If there's an existing Redis Deployment but Redis has been disabled, delete the Redis Deployment and Service and remove the Redis Host and Port from the main app's environment variables
// 4. If Redis is enabled but there's no existing Redis Deployment, create one (Deployment and Service) and add the Redis Host and Port to the main app's environment variables
func (r *PodInfoInstanceReconciler) CheckAndUpdateExistingDeploymentAsNeeded(ctx context.Context, pii *podinfoappv1.PodInfoInstance) error {
	l := log.FromContext(ctx)
	// Fetch the Deployment object for the PodInfoInstance
	d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: pii.Status.AppDeployment.Name, Namespace: pii.Namespace}}
	err := r.Get(ctx, client.ObjectKeyFromObject(d), d)
	if err != nil {
		l.Error(err, fmt.Sprintf("Error while attempting to get Deployment %s", pii.Status.AppDeployment.Name))
		pii.Status.AppDeployment.Errors = append(pii.Status.AppDeployment.Errors, fmt.Sprintf("Error getting Deployment %s: %v", pii.Status.AppDeployment.Name, err))
		return err
	}

	appDeploymentUpdateRequired := checkAndUpdateAppDeploymentSpec(ctx, d, pii)

	// If we have an existing Redis deployment, check if it needs to be updated or deleted
	if pii.Status.RedisDeployment.Name != "" {
		// Fetch the Redis Deployment object for the PodInfoInstance
		rd := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: pii.Status.RedisDeployment.Name, Namespace: pii.Namespace}}
		err := r.Get(ctx, client.ObjectKeyFromObject(rd), rd)
		if err != nil {
			l.Error(err, fmt.Sprintf("Error while attempting to get Redis Deployment %s", pii.Status.RedisDeployment.Name))
			pii.Status.RedisDeployment.Errors = append(pii.Status.RedisDeployment.Errors, fmt.Sprintf("Error getting Redis Deployment %s: %v", pii.Status.RedisDeployment.Name, err))
			return err
		}
		if !pii.Spec.Redis.Enabled {
			// Since we have a Redis Deployment but Redis is disabled, delete the Redis Deployment and Service
			l.V(5).Info(fmt.Sprintf("Redis is disabled for PodInfoInstance %s, but the Redis Deployment and Service have not been deleted yet. Deleting them now", pii.Name))
			// Delete the Redis Deployment
			err := r.Delete(ctx, rd)
			if err != nil {
				l.Error(err, fmt.Sprintf("Error while attempting to delete Redis Deployment %s", rd.Name))
				pii.Status.RedisDeployment.Errors = append(pii.Status.RedisDeployment.Errors, fmt.Sprintf("Error deleting Redis Deployment %s: %v", rd.Name, err))
				return err
			}
			// Delete the Redis Service
			rs := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: pii.Status.RedisService.Name, Namespace: pii.Namespace}}
			err = r.Delete(ctx, rs)
			if err != nil {
				l.Error(err, fmt.Sprintf("Error while attempting to delete Redis Service %s", rs.Name))
				pii.Status.RedisService.Errors = append(pii.Status.RedisService.Errors, fmt.Sprintf("Error deleting Redis Service %s: %v", rs.Name, err))
				return err
			}
			// Update the Pod's environment variables to remove the Redis Host and Port
			for i, envVar := range d.Spec.Template.Spec.Containers[0].Env {
				if envVar.Name == "PODINFO_CACHE_SERVER" {
					d.Spec.Template.Spec.Containers[0].Env = append(d.Spec.Template.Spec.Containers[0].Env[:i], d.Spec.Template.Spec.Containers[0].Env[i+1:]...)
					break
				}
			}
			// Update the PodInfoInstance's status to reflect that the Redis Deployment and Service have been deleted
			pii.Status.RedisDeployment.Name = ""
			pii.Status.RedisService.Name = ""
			pii.Status.RedisDeployment.Errors = nil
			pii.Status.RedisService.Errors = nil
			appDeploymentUpdateRequired = true
		} else {
			// Redis is enabled, check if the Redis Deployment needs to be updated and update it as needed
			if checkAndUpdateRedisDeploymentSpec(ctx, rd, pii) {
				err = r.Update(ctx, rd)
				if err != nil {
					l.Error(err, fmt.Sprintf("Error while attempting to update Redis Deployment %s", rd.Name))
					pii.Status.RedisDeployment.Errors = append(pii.Status.RedisDeployment.Errors, fmt.Sprintf("Error updating Redis Deployment %s: %v", rd.Name, err))
					return err
				}
			}
		}
	} else {
		// Check if Redis is enabled and if so, create a Redis Deployment and Service
		if pii.Spec.Redis.Enabled {
			l.V(5).Info(fmt.Sprintf("Redis is enabled for PodInfoInstance %s, but no Redis Deployment or Service exists yet. Creating them now", pii.Name))
			err := r.CreateRedisDeploymentAndService(ctx, &d.Spec.Template, pii)
			if err != nil {
				return err
			}
			appDeploymentUpdateRequired = true
		}
	}

	if appDeploymentUpdateRequired {
		err = r.Update(ctx, d)
		if err != nil {
			l.Error(err, fmt.Sprintf("Error while attempting to update Deployment %s", d.Name))
			pii.Status.AppDeployment.Errors = append(pii.Status.AppDeployment.Errors, fmt.Sprintf("Error updating Deployment %s: %v", d.Name, err))
			return err
		}
	}
	return nil
}

// checkAndUpdateAppDeploymentSpec checks the given Deployment's spec and updates it as needed
func checkAndUpdateAppDeploymentSpec(ctx context.Context, d *appsv1.Deployment, pii *podinfoappv1.PodInfoInstance) bool {
	l := log.FromContext(ctx)
	updateRequired := false
	// Check if the Deployment's replica count needs to be updated
	if *d.Spec.Replicas != pii.Spec.ReplicaCount {
		l.V(5).Info(fmt.Sprintf("Replica count for Deployment %s needs to be updated, updating it now (old: %v, new: %v)", d.Name, *d.Spec.Replicas, pii.Spec.ReplicaCount))
		// Update the Deployment's replica count
		d.Spec.Replicas = &pii.Spec.ReplicaCount
		updateRequired = true
	}

	// Check if the image needs to be updated
	if d.Spec.Template.Spec.Containers[0].Image != fmt.Sprintf("%s:%s", pii.Spec.Image.Repository, pii.Spec.Image.Tag) {
		l.V(5).Info(fmt.Sprintf("Image for Deployment %s needs to be updated, updating it now (old: %v, new: %v)", d.Name, d.Spec.Template.Spec.Containers[0].Image, fmt.Sprintf("%s:%s", pii.Spec.Image.Repository, pii.Spec.Image.Tag)))
		// Update the Deployment's image
		d.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("%s:%s", pii.Spec.Image.Repository, pii.Spec.Image.Tag)
		updateRequired = true
	}

	// Check if the Deployment's UI color needs to be updated
	if d.Spec.Template.Spec.Containers[0].Env[0].Value != pii.Spec.UI.Color {
		l.V(5).Info(fmt.Sprintf("UI color for Deployment %s needs to be updated, updating it now (old: %v, new: %v)", d.Name, d.Spec.Template.Spec.Containers[0].Env[0].Value, pii.Spec.UI.Color))
		// Update the Deployment's UI color
		d.Spec.Template.Spec.Containers[0].Env[0].Value = pii.Spec.UI.Color
		updateRequired = true
	}

	// Check if the Deployment's UI message needs to be updated
	if d.Spec.Template.Spec.Containers[0].Env[1].Value != pii.Spec.UI.Message {
		l.V(5).Info(fmt.Sprintf("UI message for Deployment %s needs to be updated, updating it now (old: %v, new: %v)", d.Name, d.Spec.Template.Spec.Containers[0].Env[1].Value, pii.Spec.UI.Message))
		// Update the Deployment's UI message
		d.Spec.Template.Spec.Containers[0].Env[1].Value = pii.Spec.UI.Message
		updateRequired = true
	}

	// Check if the Deployment's resources need to be updated
	if d.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().String() != pii.Spec.Resources.MemoryRequest || d.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().String() != pii.Spec.Resources.CPURequest || d.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String() != pii.Spec.Resources.MemoryLimit || d.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().String() != pii.Spec.Resources.CPULimit {
		l.V(5).Info(fmt.Sprintf("Resources for Deployment %s need to be updated, updating them now (old: %v, new: %v)", d.Name, d.Spec.Template.Spec.Containers[0].Resources, pii.Spec.Resources))
		// Update the Deployment's resources
		d.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(pii.Spec.Resources.CPURequest),
				corev1.ResourceMemory: resource.MustParse(pii.Spec.Resources.MemoryRequest),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(pii.Spec.Resources.CPULimit),
				corev1.ResourceMemory: resource.MustParse(pii.Spec.Resources.MemoryLimit),
			},
		}
		updateRequired = true
	}

	return updateRequired
}

// checkAndUpdateRedisDeploymentSpec checks the given Redis Deployment's spec and updates it as needed
func checkAndUpdateRedisDeploymentSpec(ctx context.Context, rd *appsv1.Deployment, pii *podinfoappv1.PodInfoInstance) bool {
	l := log.FromContext(ctx)
	updateRequired := false
	// Check if the Redis Deployment's image needs to be updated
	if rd.Spec.Template.Spec.Containers[0].Image != fmt.Sprintf("%s:%s", pii.Spec.Redis.Image.Repository, pii.Spec.Redis.Image.Tag) {
		l.V(5).Info(fmt.Sprintf("Image for Redis Deployment %s needs to be updated, updating it now (old: %v, new: %v)", rd.Name, rd.Spec.Template.Spec.Containers[0].Image, fmt.Sprintf("%s:%s", pii.Spec.Redis.Image.Repository, pii.Spec.Redis.Image.Tag)))
		// Update the Redis Deployment's image
		rd.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("%s:%s", pii.Spec.Redis.Image.Repository, pii.Spec.Redis.Image.Tag)
		updateRequired = true
	}

	// Check if the Redis Deployment's resources need to be updated
	if rd.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().String() != pii.Spec.Redis.Resources.MemoryRequest || rd.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().String() != pii.Spec.Redis.Resources.CPURequest || rd.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String() != pii.Spec.Redis.Resources.MemoryLimit || rd.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().String() != pii.Spec.Redis.Resources.CPULimit {
		l.V(5).Info(fmt.Sprintf("Resources for Redis Deployment %s need to be updated, updating them now (old: %v, new: %v)", rd.Name, rd.Spec.Template.Spec.Containers[0].Resources, pii.Spec.Redis.Resources))
		// Update the Redis Deployment's resources
		rd.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(pii.Spec.Redis.Resources.CPURequest),
				corev1.ResourceMemory: resource.MustParse(pii.Spec.Redis.Resources.MemoryRequest),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(pii.Spec.Redis.Resources.CPULimit),
				corev1.ResourceMemory: resource.MustParse(pii.Spec.Redis.Resources.MemoryLimit),
			},
		}
		updateRequired = true
	}
	return updateRequired
}

// generateDeploymentSpecForPodInfoInstance generates a new Deployment object for the given PodInfoInstance and returns it
func generateDeploymentSpecForPodInfoInstance(pii *podinfoappv1.PodInfoInstance) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pii.Name,
			Namespace: pii.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "podinfo",
				"app.kubernetes.io/part-of":   pii.Name,
				"app.kubernetes.io/component": "app",
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(pii, podinfoappv1.GroupVersion.WithKind("PodInfoInstance")),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &pii.Spec.ReplicaCount,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":      "podinfo",
					"app.kubernetes.io/part-of":   pii.Name,
					"app.kubernetes.io/component": "app",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":      "podinfo",
						"app.kubernetes.io/part-of":   pii.Name,
						"app.kubernetes.io/component": "app",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "podinfo",
							Image: fmt.Sprintf("%s:%s", pii.Spec.Image.Repository, pii.Spec.Image.Tag),
							Env: []corev1.EnvVar{
								{
									Name:  "PODINFO_UI_COLOR",
									Value: pii.Spec.UI.Color,
								},
								{
									Name:  "PODINFO_UI_MESSAGE",
									Value: pii.Spec.UI.Message,
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 9898,
								},
								{
									Name:          "https",
									ContainerPort: 9899,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(pii.Spec.Resources.CPURequest),
									corev1.ResourceMemory: resource.MustParse(pii.Spec.Resources.MemoryRequest),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(pii.Spec.Resources.CPULimit),
									corev1.ResourceMemory: resource.MustParse(pii.Spec.Resources.MemoryLimit),
								},
							},
						},
					},
				},
			},
		},
	}
}

// generateRedisServiceSpec generates a new Redis Service object for the given PodInfoInstance and returns it
func generateServiceSpecForPodInfoInstance(pii *podinfoappv1.PodInfoInstance) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pii.Name,
			Namespace: pii.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "podinfo",
				"app.kubernetes.io/part-of":   pii.Name,
				"app.kubernetes.io/component": "app",
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(pii, podinfoappv1.GroupVersion.WithKind("PodInfoInstance")),
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app.kubernetes.io/name":      "podinfo",
				"app.kubernetes.io/part-of":   pii.Name,
				"app.kubernetes.io/component": "app",
			},
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: 9898,
				},
				{
					Name: "https",
					Port: 9899,
				},
			},
		},
	}
}

// generateRedisDeploymentSpecForPodInfoInstance generates a new Redis Deployment object for the given PodInfoInstance and returns it
func generateRedisDeploymentSpecForPodInfoInstance(pii *podinfoappv1.PodInfoInstance) *appsv1.Deployment {
	// TODO(moshe): we may want to consider making the Redis Deployment's replica count configurable in the future
	replicasCount := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-redis", pii.Name),
			Labels: map[string]string{
				"app.kubernetes.io/name":      fmt.Sprintf("%v-redis", pii.Name),
				"app.kubernetes.io/part-of":   pii.Name,
				"app.kubernetes.io/component": "redis",
			},
			Namespace: pii.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(pii, podinfoappv1.GroupVersion.WithKind("PodInfoInstance")),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicasCount,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":      fmt.Sprintf("%v-redis", pii.Name),
					"app.kubernetes.io/part-of":   pii.Name,
					"app.kubernetes.io/component": "redis",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":      fmt.Sprintf("%v-redis", pii.Name),
						"app.kubernetes.io/part-of":   pii.Name,
						"app.kubernetes.io/component": "redis",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: fmt.Sprintf("%s:%s", pii.Spec.Redis.Image.Repository, pii.Spec.Redis.Image.Tag),
							Ports: []corev1.ContainerPort{
								{
									Name:          "redis",
									ContainerPort: 6379,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(pii.Spec.Redis.Resources.CPURequest),
									corev1.ResourceMemory: resource.MustParse(pii.Spec.Redis.Resources.MemoryRequest),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(pii.Spec.Redis.Resources.CPULimit),
									corev1.ResourceMemory: resource.MustParse(pii.Spec.Redis.Resources.MemoryLimit),
								},
							},
						},
					},
				},
			},
		},
	}
}

// generateRedisServiceSpecForPodInfoInstance generates a new Redis Service object for the given PodInfoInstance and returns it
func generateRedisServiceSpecForPodInfoInstance(pii *podinfoappv1.PodInfoInstance) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-redis", pii.Name),
			Labels: map[string]string{
				"app.kubernetes.io/name":      fmt.Sprintf("%v-redis", pii.Name),
				"app.kubernetes.io/part-of":   pii.Name,
				"app.kubernetes.io/component": "redis",
			},
			Namespace: pii.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(pii, podinfoappv1.GroupVersion.WithKind("PodInfoInstance")),
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app.kubernetes.io/name":      fmt.Sprintf("%v-redis", pii.Name),
				"app.kubernetes.io/part-of":   pii.Name,
				"app.kubernetes.io/component": "redis",
			},
			Ports: []corev1.ServicePort{
				{
					Name: "redis",
					Port: 6379,
				},
			},
		},
	}
}
