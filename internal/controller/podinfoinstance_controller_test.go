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
	"reflect"
	"testing"

	podinfoappv1 "github.com/moshevayner/k8s-controller-go-podinfo/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
	// TODO(moshe): Implement this test
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
