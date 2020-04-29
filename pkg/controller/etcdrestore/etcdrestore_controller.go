package etcdrestore

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/util/retry"
	"github.com/lmdkfs/etcd-operator/pkg/resources/statefulset"

	etcdv1alpha1 "github.com/lmdkfs/etcd-operator/pkg/apis/etcd/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_etcdrestore")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new EtcdRestore Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileEtcdRestore{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("etcdrestore-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource EtcdRestore
	err = c.Watch(&source.Kind{Type: &etcdv1alpha1.EtcdRestore{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Pods and requeue the owner EtcdRestore
	err = c.Watch(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType: &etcdv1alpha1.EtcdRestore{},
	})
	if err != nil {
		return err
	}
	err = c.Watch(&source.Kind{Type: &corev1.PersistentVolumeClaim{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileEtcdRestore implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileEtcdRestore{}

// ReconcileEtcdRestore reconciles a EtcdRestore object
type ReconcileEtcdRestore struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

func (r *ReconcileEtcdRestore) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling EtcdRestore")

	// Fetch the EtcdRestore instance
	instance := &etcdv1alpha1.EtcdRestore{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if instance.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	condition := etcdv1alpha1.EtcdRestoreCondition{
		Ready:                 true,
		LastedTranslationTime: metav1.Now(),
		Reason:                "begin etcd cluster restore",
		Message:               "",
	}

	if err := r.updateStatus(instance, condition, etcdv1alpha1.EtcdRestoreRunning); err != nil {
		log.Info(err.Error())
		return reconcile.Result{}, err
	}

	// restoring
	ss := &appsv1.StatefulSet{}
	if err := r.client.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Spec.ClusterReference}, ss); err != nil {
		condition := etcdv1alpha1.EtcdRestoreCondition{
			Ready:                 false,
			LastedTranslationTime: metav1.Now(),
			Reason:                "get reference statefulset failed",
			Message:               err.Error(),
		}
		if err := r.updateStatus(instance, condition, etcdv1alpha1.EtcdRestoreFailed); err != nil {
			log.Info(err.Error())
			return reconcile.Result{}, err
		}
	}
	pods, err := r.listPod(ss)
	if err != nil {
		condition := etcdv1alpha1.EtcdRestoreCondition{Ready: false, LastedTranslationTime: metav1.Now(), Reason: "get reference pod failed", Message: err.Error()}
		if updateErr := r.updateStatus(instance, condition,etcdv1alpha1.EtcdRestoreFailed); updateErr != nil {
			log.Info(updateErr.Error())
			return reconcile.Result{}, updateErr
		}
	}

	for _, pod := range pods {
		for _, v := range pod.Spec.Volumes {
			if v.VolumeSource.PersistentVolumeClaim != nil {
				if err := r.client.Delete(context.TODO(), &corev1.PersistentVolumeClaim{
					TypeMeta: metav1.TypeMeta{
						Kind: "PersistentVolumeClaim",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: v.VolumeSource.PersistentVolumeClaim.ClaimName,
						Namespace: ss.Namespace,
					},
				}); err != nil {
					condition := etcdv1alpha1.EtcdRestoreCondition{Ready: false, LastedTranslationTime: metav1.Now(), Reason: "delete reference pvc failed", Message: err.Error()}
					if updateErr := r.updateStatus(instance, condition, etcdv1alpha1.EtcdRestoreFailed); updateErr != nil {
						log.Info(updateErr.Error())
						return reconcile.Result{}, updateErr
					}
					return reconcile.Result{}, err
				}
			}
		}
	}
	ss.Spec.Template.Spec.InitContainers = statefulset.NewEtcdClusterInitContainers(ss, instance)
	if err := r.client.Update(context.TODO(), ss); err != nil {
		condition := etcdv1alpha1.EtcdRestoreCondition{
			Ready: false,
			LastedTranslationTime: metav1.Now(),
			Reason: "delete reference pvc failed",
			Message: err.Error(),
		}
		if updateErr := r.updateStatus(instance, condition, etcdv1alpha1.EtcdRestoreFailed); updateErr != nil {
			log.Info(updateErr.Error())
			return reconcile.Result{}, updateErr
		}
		return reconcile.Result{}, err
	}

	// store success
	condition = etcdv1alpha1.EtcdRestoreCondition{Ready: true, LastedTranslationTime: metav1.Now(), Reason: "ok",  Message: err.Error()}
	if updateErr := r.updateStatus(instance, condition, etcdv1alpha1.EtcdRestoreComplated); updateErr != nil {
		log.Info(updateErr.Error())
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileEtcdRestore) updateStatus(restore *etcdv1alpha1.EtcdRestore, condition etcdv1alpha1.EtcdRestoreCondition, phase etcdv1alpha1.EtcdRestorePhase) error {
	restore.Status.Phase = phase
	restore.Status.Conditions = append(restore.Status.Conditions, condition)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.client.Status().Update(context.TODO(), restore)
	})
}

func (r *ReconcileEtcdRestore) listPod(ss *appsv1.StatefulSet) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(ss.Namespace),
		client.MatchingLabels{"etcd.zrq.org.cn/v1alpha1": ss.Name},
	}

	if err := r.client.List(context.TODO(), podList, listOpts...); err != nil {
		return nil, err

	}
	return podList.Items, nil

}
