package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type EtcdDumpPhase string

var (
	EtcdDumpRunning   EtcdDumpPhase = "Running"
	EtcdDumpComplated EtcdDumpPhase = "Complated"
	EtcdDumpFailed    EtcdDumpPhase = "Failed"
)

// EtcddumpSpec defines the desired state of Etcddump
type EtcddumpSpec struct {
	Scheduler        string            `json:"scheduler,omitempty"`
	ClusterReference string            `json:"clusterReference"`
	Storage          S3StorageProvider `json:"storage"`
}


type EtcdDumpCondition struct {
	Ready                 bool        `json:"ready"`
	Location              string      `json:"location,omitempty"`
	Reason                string      `json:"reason,omitempty"`
	Message               string      `json:"message,omitempty"`
	LastedTranslationTime metav1.Time `json:"lastedTranslationTime"`
}

type StorageProvider struct {
	S3    *S3StorageProvider    `json:"s3,omitempty"`
	Qiniu *QiniuStorageProvider `json:"qiniu,omitempty"`
}

type S3StorageProvider struct {
	Region            string                       `json:"region,omitempty"`
	Endpoint          string                       `json:"endpoint,omitempty"`
	Bucket            string                       `json:"bucket,omitempty"`
	ForcePathStyle    bool                         `json:"forcePathStyle,omitempty"`
	CredentialsSecret *corev1.LocalObjectReference `json:"credentialsSecret,omitempty"`
}

type QiniuStorageProvider struct {
	AccessKey string `json:"accessKey,omitempty"`
	SecretKey string `json:"secretKey,omitempty"`
	Bucket    string `json:"bucket,omitempty"`
	IO        string `json:"io,omitempty"`
	API       string `json:"api,omitempty"`
	UP        string `json:"up,omitempty"`
}

// EtcddumpStatus defines the observed state of Etcddump
type EtcddumpStatus struct {
	Conditions []EtcdDumpCondition `json:"conditions,omitempty"`
	Phase      EtcdDumpPhase          `json:"phase"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Etcddump is the Schema for the etcddumps API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=etcddumps,scope=Namespaced
type Etcddump struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EtcddumpSpec   `json:"spec,omitempty"`
	Status EtcddumpStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EtcddumpList contains a list of Etcddump
type EtcddumpList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Etcddump `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Etcddump{}, &EtcddumpList{})
}
