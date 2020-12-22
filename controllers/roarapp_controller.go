/*
Copyright 2020 Brent Laster.

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

package controllers

import (
	"context"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"strconv"
	"strings"
	// "k8s.io/apimachinery"
	// "k8s.io/client-go@0.17.0"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	roarappv1alpha1 "github.com/brentlaster/op/api/v1alpha1"
)

// RoarAppReconciler reconciles a instance object
type RoarAppReconciler struct {
	Client client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

var nextPort = 0

// +kubebuilder:rbac:groups=roarapp.roarapp.com,resources=roarapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=roarapp.roarapp.com,resources=roarapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;delete

func (r *RoarAppReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {

	log := r.Log.WithValues("roarapp", req.NamespacedName)

	log.Info("Reconciling instance")

	// Fetch the roarapp instance
	instance := &roarappv1alpha1.RoarApp{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// List all pods owned by this roarapp instance
	podList := &corev1.PodList{}
	lbs := map[string]string{
		"app":     instance.Name,
		"version": "v0.1",
	}
	labelSelector := labels.SelectorFromSet(lbs)
	listOps := &client.ListOptions{Namespace: req.Namespace, LabelSelector: labelSelector}
	if err = r.Client.List(context.TODO(), podList, listOps); err != nil {
		return ctrl.Result{}, err
	}

	// Count the pods that are pending or running as available
	var available []corev1.Pod
	for _, pod := range podList.Items {
		if pod.ObjectMeta.DeletionTimestamp != nil {
			continue
		}
		if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodPending {
			available = append(available, pod)
		}
	}
	numAvailable := int32(len(available))
	availableNames := []string{}
	for _, pod := range available {
		availableNames = append(availableNames, pod.ObjectMeta.Name)
	}

	// Update the status if necessary
	status := roarappv1alpha1.RoarAppStatus{
		PodNames: availableNames,
	}
	if !reflect.DeepEqual(instance.Status, status) {
		instance.Status = status
		err = r.Client.Status().Update(context.TODO(), instance)
		if err != nil {
			log.Error(err, "Failed to update instance status")
			return ctrl.Result{}, err
		}
	}

	if numAvailable > instance.Spec.Replicas {
		log.Info("Scaling down pods", "Currently available", numAvailable, "Required replicas", instance.Spec.Replicas)
		diff := numAvailable - instance.Spec.Replicas
		dpods := available[:diff]
		for _, dpod := range dpods {
			err = r.Client.Delete(context.TODO(), &dpod)
			if err != nil {
				log.Error(err, "Failed to delete pod", "pod.name", dpod.Name)
				return ctrl.Result{}, err
			}
			log.Info("Scaling down corresponding service", "Pod", numAvailable, "Service", instance.Spec.Replicas)
			strPort := dpod.Name[strings.LastIndex(dpod.Name, "-")+1:]
			sName := instance.Name + "-service-" + strPort
			//found := &appsv1.Deployment{}
			s := &corev1.Service{}
			err := r.Client.Get(context.TODO(), types.NamespacedName{
				Name:      sName,
				Namespace: req.Namespace,
			}, s)
			err = r.Client.Delete(context.TODO(), s)
			if err != nil {
				if errors.IsNotFound(err) {
					// Return and don't requeue
					return ctrl.Result{}, nil
				}
				// Error reading the object - requeue the request.
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if numAvailable < instance.Spec.Replicas {
		log.Info("Scaling up pods", "Currently available", numAvailable, "Required replicas", instance.Spec.Replicas)
		// Define a new Pod object
		pod := newPodForCR(instance)
		// Set instance instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, pod, r.Scheme); err != nil {
			return reconcile.Result{}, err
		}
		err = r.Client.Create(context.TODO(), pod)
		if err != nil {
			log.Error(err, "Failed to create pod", "pod.name", pod.Name)
			return ctrl.Result{}, err
		}
		// Define a new Service object
		svc := newServiceForPod(instance)
		// Set instance instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, svc, r.Scheme); err != nil {
			return reconcile.Result{}, err
		}
		err = r.Client.Create(context.TODO(), svc)
		if err != nil {
			log.Error(err, "Failed to create service", "svc.name", svc.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, nil
}

func (r *RoarAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&roarappv1alpha1.RoarApp{}).
		Complete(r)
}

func newServiceForPod(cr *roarappv1alpha1.RoarApp) *corev1.Service {

	strPort := strconv.Itoa(nextPort)
	labels := map[string]string{
		"app": cr.Name,
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-service-" + strPort,
			Namespace: cr.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Protocol:   corev1.ProtocolTCP,
				Port:       8089,
				TargetPort: intstr.FromInt(8080),
				NodePort:   int32(nextPort),
			}},
			Type: corev1.ServiceTypeNodePort,
		},
	}
}

// newPodForCR returns a instance pod with the same name/namespace as the cr
func newPodForCR(cr *roarappv1alpha1.RoarApp) *corev1.Pod {
	if nextPort == 0 {
		nextPort = 32000
	} else {
		nextPort++
	}
	strPort := strconv.Itoa(nextPort)
	labels := map[string]string{
		"app":      cr.Name,
		"version":  "v0.1",
		"nodePort": strPort,
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-pod-" + strPort,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "roar-web",
					Image:   cr.Spec.WebImage,
					Command: []string{"catalina.sh", "run"},
				},
				{
					Name:    "roar-db",
					Image:   cr.Spec.DbImage,
					Env:     []corev1.EnvVar{{Name: "MYSQL_USER", Value: "admin"}, {Name: "MYSQL_PASSWORD", Value: "admin"}, {Name: "MYSQL_ROOT_PASSWORD", Value: "root+1"}, {Name: "MYSQL_DATABASE", Value: "registry"}},
					Command: []string{"/entrypoint.sh"},
					Args:    []string{"mysqld"},
				},
			},
		},
	}
}
