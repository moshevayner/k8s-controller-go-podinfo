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
	"errors"
	"fmt"
	"reflect"
	"testing"

	podinfoappv1 "github.com/moshevayner/k8s-controller-go-podinfo/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestPodInfoInstanceReconciler_CreateDeploymentForPodInfoInstance(t *testing.T) {
	testScheme := runtime.NewScheme()
	_ = podinfoappv1.AddToScheme(testScheme) // Register podinfoapp/v1 types
	_ = appsv1.AddToScheme(testScheme)       // Register apps/v1 types
	_ = corev1.AddToScheme(testScheme)       // Register core/v1 types
	type args struct {
		ctx context.Context
		pii *podinfoappv1.PodInfoInstance
	}

	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "PodInfoInstance with Redis Disabled",
			args: args{
				ctx: context.Background(),
				pii: &podinfoappv1.PodInfoInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: podinfoappv1.PodInfoInstanceSpec{
						ReplicaCount: 1,
						Resources: podinfoappv1.Resources{
							MemoryRequest: "64Mi",
							MemoryLimit:   "128Mi",
							CPURequest:    "250m",
							CPULimit:      "500m",
						},
						Image: podinfoappv1.Image{
							Repository: "stefanprodan/podinfo",
							Tag:        "latest",
						},
						UI: podinfoappv1.UI{
							Color:   "#ffffff",
							Message: "Hello there!",
						},
						Redis: podinfoappv1.Redis{
							Enabled: false,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "PodInfoInstance with Redis Enabled",
			args: args{
				ctx: context.Background(),
				pii: createPodInfoInstance("test", 1, true),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize the fake client with the necessary objects
			fakeClient := fake.NewClientBuilder().
				WithScheme(testScheme).
				WithRuntimeObjects(tt.args.pii).
				Build()

			r := &PodInfoInstanceReconciler{
				Client: fakeClient,
				Scheme: testScheme,
			}
			if err := r.CreateDeploymentForPodInfoInstance(tt.args.ctx, tt.args.pii); (err != nil) != tt.wantErr {
				t.Errorf("PodInfoInstanceReconciler.CreateDeploymentForPodInfoInstance() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Update the PodInfoInstance object in the fake client
			err := fakeClient.Update(context.Background(), tt.args.pii)
			if err != nil {
				t.Fatalf("Failed to update PodInfoInstance in fake client: %v", err)
			}

			// Use the fake client to check the Deployment and Service
			d := &appsv1.Deployment{}
			err = fakeClient.Get(context.Background(), client.ObjectKey{Name: tt.args.pii.Name, Namespace: tt.args.pii.Namespace}, d)
			if err != nil {
				t.Errorf("Failed to get Deployment: %v", err)
			}
			s := &corev1.Service{}
			err = fakeClient.Get(context.Background(), client.ObjectKey{Name: tt.args.pii.Name, Namespace: tt.args.pii.Namespace}, s)
			if err != nil {
				t.Errorf("Failed to get Service: %v", err)
			}

			// Check that the PodInfoInstance's status was updated
			updatedPii := &podinfoappv1.PodInfoInstance{}
			err = fakeClient.Get(context.Background(), client.ObjectKey{Name: tt.args.pii.Name, Namespace: tt.args.pii.Namespace}, updatedPii)
			if err != nil {
				t.Errorf("Failed to get updated PodInfoInstance: %v", err)
			}
			if updatedPii.Status.AppDeployment.Name != d.Name {
				t.Errorf("PodInfoInstance status was not updated with the Deployment name (got %s, expected %s)", updatedPii.Status.AppDeployment.Name, d.Name)
			}
			if updatedPii.Status.AppService.Name != s.Name {
				t.Errorf("PodInfoInstance status was not updated with the Service name (got %s, expected %s)", updatedPii.Status.AppService.Name, s.Name)
			}

			expectedAppDeployment := generateDeploymentSpecForPodInfoInstance(tt.args.pii)

			if tt.args.pii.Spec.Redis.Enabled {
				// If Redis is enabled, check that the Redis Deployment and Service were created
				expectedRedisDeployment := generateRedisDeploymentSpecForPodInfoInstance(tt.args.pii)
				rd := &appsv1.Deployment{}
				err := fakeClient.Get(context.Background(), client.ObjectKey{Name: tt.args.pii.Name + "-redis", Namespace: tt.args.pii.Namespace}, rd)
				if err != nil {
					t.Errorf("Failed to get Redis Deployment: %v", err)
				}
				if !reflect.DeepEqual(rd.Spec, expectedRedisDeployment.Spec) {
					t.Errorf("Redis Deployment was not created correctly. Got = %v, want %v", rd.Spec, expectedRedisDeployment.Spec)
				}
				rs := &corev1.Service{}
				err = fakeClient.Get(context.Background(), client.ObjectKey{Name: tt.args.pii.Name + "-redis", Namespace: tt.args.pii.Namespace}, rs)
				if err != nil {
					t.Errorf("Failed to get Redis Service: %v", err)
				}
				expectedRedisService := generateRedisServiceSpecForPodInfoInstance(tt.args.pii)
				if !reflect.DeepEqual(rs.Spec, expectedRedisService.Spec) {
					t.Errorf("Redis Service was not created correctly. Got = %v, want %v", rs.Spec, expectedRedisService.Spec)
				}
				expectedAppDeployment.Spec.Template.Spec.Containers[0].Env = append(expectedAppDeployment.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{Name: "PODINFO_CACHE_SERVER", Value: fmt.Sprintf("tcp://%s:%d", rs.Name, rs.Spec.Ports[0].Port)})
			} else {
				// If Redis is disabled, check that the Redis Deployment and Service were not created
				err := fakeClient.Get(context.Background(), client.ObjectKey{Name: tt.args.pii.Name + "-redis", Namespace: tt.args.pii.Namespace}, &appsv1.Deployment{})
				if err == nil {
					t.Errorf("Redis Deployment was created when Redis was disabled")
				}
			}

			if !reflect.DeepEqual(d.Spec, expectedAppDeployment.Spec) {
				t.Errorf("App Deployment was not created correctly. Got = %v, want %v", d.Spec, expectedAppDeployment.Spec)
			}
		})
	}
}

func TestPodInfoInstanceReconciler_CheckAndUpdateExistingDeploymentAsNeeded(t *testing.T) {
	testScheme := runtime.NewScheme()
	_ = podinfoappv1.AddToScheme(testScheme) // Register podinfoapp/v1 types
	_ = appsv1.AddToScheme(testScheme)       // Register apps/v1 types
	_ = corev1.AddToScheme(testScheme)       // Register core/v1 types

	type args struct {
		ctx context.Context
		pii *podinfoappv1.PodInfoInstance
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Check and Update Existing Deployment",
			args: args{
				ctx: context.Background(),
				pii: createPodInfoInstance("test", 2, false),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize the fake client with the necessary objects
			fakeClient := fake.NewClientBuilder().
				WithScheme(testScheme).
				WithRuntimeObjects(tt.args.pii).
				Build()

			r := &PodInfoInstanceReconciler{
				Client: fakeClient,
				Scheme: testScheme,
			}

			// First, create the initial Deployment
			err := r.CreateDeploymentForPodInfoInstance(tt.args.ctx, tt.args.pii)
			if err != nil {
				t.Fatalf("Failed to create initial Deployment: %v", err)
			}

			// Now, modify the PodInfoInstance to trigger an update
			tt.args.pii.Spec.ReplicaCount = 3
			err = fakeClient.Update(context.Background(), tt.args.pii)
			if err != nil {
				t.Fatalf("Failed to update PodInfoInstance in fake client: %v", err)
			}

			// Call the method under test
			if err := r.CheckAndUpdateExistingDeploymentAsNeeded(tt.args.ctx, tt.args.pii); (err != nil) != tt.wantErr {
				t.Errorf("PodInfoInstanceReconciler.CheckAndUpdateExistingDeploymentAsNeeded() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify that the Deployment was updated
			updatedDeployment := &appsv1.Deployment{}
			err = fakeClient.Get(context.Background(), client.ObjectKey{Name: tt.args.pii.Name, Namespace: tt.args.pii.Namespace}, updatedDeployment)
			if err != nil {
				t.Fatalf("Failed to get updated Deployment: %v", err)
			}
			if *updatedDeployment.Spec.Replicas != 3 {
				t.Errorf("Deployment was not updated correctly. Got replicas = %d, want %d", *updatedDeployment.Spec.Replicas, 3)
			}
		})
	}
}

func TestPodInfoInstanceReconciler_ReconcileCreatesResourcesAndIgnoresMissingInstance(t *testing.T) {
	testScheme := runtime.NewScheme()
	_ = podinfoappv1.AddToScheme(testScheme)
	_ = appsv1.AddToScheme(testScheme)
	_ = corev1.AddToScheme(testScheme)

	t.Run("creates resources and persists status", func(t *testing.T) {
		pii := createPodInfoInstance("reconcile-test", 1, false)
		fakeClient := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithStatusSubresource(&podinfoappv1.PodInfoInstance{}).
			WithRuntimeObjects(pii).
			Build()
		r := &PodInfoInstanceReconciler{Client: fakeClient, Scheme: testScheme}

		_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(pii)})
		if err != nil {
			t.Fatalf("Reconcile() error = %v", err)
		}

		updatedPii := &podinfoappv1.PodInfoInstance{}
		if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(pii), updatedPii); err != nil {
			t.Fatalf("Failed to get reconciled PodInfoInstance: %v", err)
		}
		if updatedPii.Status.AppDeployment.Name != pii.Name || updatedPii.Status.AppService.Name != pii.Name {
			t.Errorf("status = %+v, expected app Deployment and Service names to be %q", updatedPii.Status, pii.Name)
		}
		if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(pii), &appsv1.Deployment{}); err != nil {
			t.Errorf("Failed to get reconciled app Deployment: %v", err)
		}
		if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(pii), &corev1.Service{}); err != nil {
			t.Errorf("Failed to get reconciled app Service: %v", err)
		}
	})

	t.Run("ignores a missing instance", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
		r := &PodInfoInstanceReconciler{Client: fakeClient, Scheme: testScheme}

		_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: "missing", Namespace: "default"}})
		if err != nil {
			t.Errorf("Reconcile() error = %v, want nil for a missing instance", err)
		}
	})
}

func TestPodInfoInstanceReconciler_UpdatesAppAndRedisLifecycle(t *testing.T) {
	testScheme := runtime.NewScheme()
	_ = podinfoappv1.AddToScheme(testScheme)
	_ = appsv1.AddToScheme(testScheme)
	_ = corev1.AddToScheme(testScheme)

	pii := createPodInfoInstance("lifecycle-test", 1, false)
	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithRuntimeObjects(pii).
		Build()
	r := &PodInfoInstanceReconciler{Client: fakeClient, Scheme: testScheme}
	ctx := context.Background()

	if err := r.CreateDeploymentForPodInfoInstance(ctx, pii); err != nil {
		t.Fatalf("Failed to create initial app resources: %v", err)
	}

	pii.Spec.ReplicaCount = 3
	pii.Spec.Image.Tag = "6.7.0"
	pii.Spec.UI.Color = "#000000"
	pii.Spec.UI.Message = "Updated message"
	pii.Spec.Resources = podinfoappv1.Resources{
		MemoryRequest: "128Mi",
		MemoryLimit:   "256Mi",
		CPURequest:    "500m",
		CPULimit:      "1",
	}
	if err := r.CheckAndUpdateExistingDeploymentAsNeeded(ctx, pii); err != nil {
		t.Fatalf("Failed to update app Deployment: %v", err)
	}

	appDeployment := &appsv1.Deployment{}
	if err := fakeClient.Get(ctx, client.ObjectKeyFromObject(pii), appDeployment); err != nil {
		t.Fatalf("Failed to get updated app Deployment: %v", err)
	}
	container := appDeployment.Spec.Template.Spec.Containers[0]
	if *appDeployment.Spec.Replicas != pii.Spec.ReplicaCount || container.Image != "stefanprodan/podinfo:6.7.0" {
		t.Errorf("app Deployment replicas/image = %d/%q, want %d/%q", *appDeployment.Spec.Replicas, container.Image, pii.Spec.ReplicaCount, "stefanprodan/podinfo:6.7.0")
	}
	if container.Env[0].Value != pii.Spec.UI.Color || container.Env[1].Value != pii.Spec.UI.Message {
		t.Errorf("app Deployment UI environment = %+v, want color %q and message %q", container.Env, pii.Spec.UI.Color, pii.Spec.UI.Message)
	}
	if container.Resources.Requests.Cpu().String() != pii.Spec.Resources.CPURequest || container.Resources.Limits.Memory().String() != pii.Spec.Resources.MemoryLimit {
		t.Errorf("app Deployment resources = %+v, want %+v", container.Resources, pii.Spec.Resources)
	}

	pii.Spec.Redis.Enabled = true
	pii.Spec.Redis.Image = podinfoappv1.Image{Repository: "redis", Tag: "7.4"}
	if err := r.CheckAndUpdateExistingDeploymentAsNeeded(ctx, pii); err != nil {
		t.Fatalf("Failed to enable Redis: %v", err)
	}

	redisDeployment := &appsv1.Deployment{}
	redisKey := client.ObjectKey{Name: pii.Name + "-redis", Namespace: pii.Namespace}
	if err := fakeClient.Get(ctx, redisKey, redisDeployment); err != nil {
		t.Fatalf("Failed to get Redis Deployment: %v", err)
	}
	if pii.Status.RedisDeployment.Name != redisKey.Name || pii.Status.RedisService.Name != redisKey.Name {
		t.Errorf("Redis status = %+v, expected names to be %q", pii.Status, redisKey.Name)
	}
	if err := fakeClient.Get(ctx, client.ObjectKeyFromObject(pii), appDeployment); err != nil {
		t.Fatalf("Failed to get app Deployment after enabling Redis: %v", err)
	}
	if !hasEnvironmentVariable(appDeployment, cacheServerName, "tcp://lifecycle-test-redis:6379") {
		t.Error("app Deployment did not receive the Redis cache server environment variable")
	}

	pii.Spec.Redis.Image.Tag = "7.4.1"
	pii.Spec.Redis.Resources.MemoryLimit = "2Gi"
	if err := r.CheckAndUpdateExistingDeploymentAsNeeded(ctx, pii); err != nil {
		t.Fatalf("Failed to update Redis: %v", err)
	}
	if err := fakeClient.Get(ctx, redisKey, redisDeployment); err != nil {
		t.Fatalf("Failed to get updated Redis Deployment: %v", err)
	}
	redisContainer := redisDeployment.Spec.Template.Spec.Containers[0]
	if redisContainer.Image != "redis:7.4.1" || redisContainer.Resources.Limits.Memory().String() != "2Gi" {
		t.Errorf("Redis Deployment image/resources = %q/%+v, want %q and memory limit %q", redisContainer.Image, redisContainer.Resources, "redis:7.4.1", "2Gi")
	}

	pii.Spec.Redis.Enabled = false
	if err := r.CheckAndUpdateExistingDeploymentAsNeeded(ctx, pii); err != nil {
		t.Fatalf("Failed to disable Redis: %v", err)
	}
	if err := fakeClient.Get(ctx, redisKey, &appsv1.Deployment{}); err == nil {
		t.Error("Redis Deployment still exists after disabling Redis")
	}
	if err := fakeClient.Get(ctx, redisKey, &corev1.Service{}); err == nil {
		t.Error("Redis Service still exists after disabling Redis")
	}
	if err := fakeClient.Get(ctx, client.ObjectKeyFromObject(pii), appDeployment); err != nil {
		t.Fatalf("Failed to get app Deployment after disabling Redis: %v", err)
	}
	if hasEnvironmentVariable(appDeployment, cacheServerName, "") {
		t.Error("app Deployment still contains the Redis cache server environment variable")
	}
	if pii.Status.RedisDeployment.Name != "" || pii.Status.RedisService.Name != "" {
		t.Errorf("Redis status = %+v, expected Redis resource names to be cleared", pii.Status)
	}
}

func TestPodInfoInstanceReconciler_RecordsClientOperationErrors(t *testing.T) {
	testScheme := runtime.NewScheme()
	_ = podinfoappv1.AddToScheme(testScheme)
	_ = appsv1.AddToScheme(testScheme)
	_ = corev1.AddToScheme(testScheme)
	ctx := context.Background()
	clientError := errors.New("API unavailable")

	t.Run("returns a PodInfoInstance get error", func(t *testing.T) {
		pii := createPodInfoInstance("get-error", 1, false)
		fakeClient := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithRuntimeObjects(pii).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
					return clientError
				},
			}).
			Build()
		r := &PodInfoInstanceReconciler{Client: fakeClient, Scheme: testScheme}

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(pii)})
		if !errors.Is(err, clientError) {
			t.Errorf("Reconcile() error = %v, want %v", err, clientError)
		}
	})

	t.Run("records a Deployment create error", func(t *testing.T) {
		pii := createPodInfoInstance("create-error", 1, false)
		fakeClient := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithInterceptorFuncs(interceptor.Funcs{
				Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
					return clientError
				},
			}).
			Build()
		r := &PodInfoInstanceReconciler{Client: fakeClient, Scheme: testScheme}

		err := r.CreateDeploymentForPodInfoInstance(ctx, pii)
		if !errors.Is(err, clientError) || len(pii.Status.AppDeployment.Errors) != 1 {
			t.Errorf("CreateDeploymentForPodInfoInstance() error/status = %v/%+v, want %v and one app Deployment error", err, pii.Status, clientError)
		}
	})

	t.Run("records an app Deployment update error", func(t *testing.T) {
		pii := createPodInfoInstance("update-error", 1, false)
		deployment := generateDeploymentSpecForPodInfoInstance(pii)
		fakeClient := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithRuntimeObjects(deployment).
			WithInterceptorFuncs(interceptor.Funcs{
				Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
					return clientError
				},
			}).
			Build()
		r := &PodInfoInstanceReconciler{Client: fakeClient, Scheme: testScheme}
		pii.Status.AppDeployment.Name = pii.Name
		pii.Spec.ReplicaCount = 2

		err := r.CheckAndUpdateExistingDeploymentAsNeeded(ctx, pii)
		if !errors.Is(err, clientError) || len(pii.Status.AppDeployment.Errors) != 1 {
			t.Errorf("CheckAndUpdateExistingDeploymentAsNeeded() error/status = %v/%+v, want %v and one app Deployment error", err, pii.Status, clientError)
		}
	})

	t.Run("records a Redis Deployment delete error", func(t *testing.T) {
		pii := createPodInfoInstance("delete-error", 1, false)
		pii.Status.AppDeployment.Name = pii.Name
		pii.Status.RedisDeployment.Name = pii.Name + "-redis"
		pii.Status.RedisService.Name = pii.Name + "-redis"
		appDeployment := generateDeploymentSpecForPodInfoInstance(pii)
		appDeployment.Spec.Template.Spec.Containers[0].Env = append(appDeployment.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{Name: cacheServerName, Value: "tcp://delete-error-redis:6379"})
		redisDeployment := generateRedisDeploymentSpecForPodInfoInstance(pii)
		redisService := generateRedisServiceSpecForPodInfoInstance(pii)
		fakeClient := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithRuntimeObjects(appDeployment, redisDeployment, redisService).
			WithInterceptorFuncs(interceptor.Funcs{
				Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
					return clientError
				},
			}).
			Build()
		r := &PodInfoInstanceReconciler{Client: fakeClient, Scheme: testScheme}

		err := r.CheckAndUpdateExistingDeploymentAsNeeded(ctx, pii)
		if !errors.Is(err, clientError) || len(pii.Status.RedisDeployment.Errors) != 1 {
			t.Errorf("CheckAndUpdateExistingDeploymentAsNeeded() error/status = %v/%+v, want %v and one Redis Deployment error", err, pii.Status, clientError)
		}
	})
}

func hasEnvironmentVariable(deployment *appsv1.Deployment, name, value string) bool {
	for _, environmentVariable := range deployment.Spec.Template.Spec.Containers[0].Env {
		if environmentVariable.Name == name && (value == "" || environmentVariable.Value == value) {
			return true
		}
	}
	return false
}

// Helper functions to create test instances and expected objects
func createPodInfoInstance(name string, replicaCount int32, redisEnabled bool) *podinfoappv1.PodInfoInstance {
	return &podinfoappv1.PodInfoInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: podinfoappv1.PodInfoInstanceSpec{
			ReplicaCount: replicaCount,
			Resources: podinfoappv1.Resources{
				MemoryRequest: "64Mi",
				MemoryLimit:   "128Mi",
				CPURequest:    "250m",
				CPULimit:      "500m",
			},
			Image: podinfoappv1.Image{
				Repository: "stefanprodan/podinfo",
				Tag:        "latest",
			},
			UI: podinfoappv1.UI{
				Color:   "#ffffff",
				Message: "Hello there!",
			},
			Redis: podinfoappv1.Redis{
				Enabled: redisEnabled,
				Resources: podinfoappv1.Resources{
					MemoryRequest: "32Mi",
					MemoryLimit:   "1Gi",
					CPURequest:    "50m",
					CPULimit:      "1",
				},
			},
		},
	}
}
