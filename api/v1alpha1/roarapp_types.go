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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RoarAppSpec defines the desired state of RoarApp
type RoarAppSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Replicas int32  `json:"replicas"`
	WebImage string `json:"webImage,omitempty"`
	DbImage  string `json:"dbImage,omitempty"`
}

// RoarAppStatus defines the observed state of RoarApp
type RoarAppStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	PodNames []string `json:"podNames"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RoarApp is the Schema for the roarapps API
type RoarApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RoarAppSpec   `json:"spec,omitempty"`
	Status RoarAppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RoarAppList contains a list of RoarApp
type RoarAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RoarApp `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RoarApp{}, &RoarAppList{})
}
