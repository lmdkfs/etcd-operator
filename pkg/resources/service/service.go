package service

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"github.com/lmdkfs/etcd-operator/pkg/apis/etcd/v1alpha1"
)
func NewService(etcd *v1alpha1.Etcd) *corev1.Service{
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: etcd.Name,
			Namespace: etcd.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(etcd, schema.GroupVersionKind{
					Group: v1alpha1.SchemeGroupVersion.Group,
					Version: v1alpha1.SchemeGroupVersion.Version,
					Kind: "Etcd",
				}),
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Port: 2379,
					Name: "etcd-client",
				},
				corev1.ServicePort{
					Port: 2380,
					Name: "etcd-server",
				},
			},
			ClusterIP: corev1.ClusterIPNone,
			Selector: map[string]string{
				"etcd.zrq.org.cn/v1alpha1": etcd.Name,
			},
		},
	}
}
