package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"reflect"

	etcdv1alpha1 "github.com/lmdkfs/etcd-operator/pkg/apis/etcd/v1alpha1"
	"github.com/lmdkfs/etcd-operator/pkg/resources/service"
	"github.com/lmdkfs/etcd-operator/pkg/resources/statefulset"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_etcd")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Etcd Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileEtcd{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("etcd-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Etcd
	err = c.Watch(&source.Kind{Type: &etcdv1alpha1.Etcd{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Pods and requeue the owner Etcd
	err = c.Watch(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &etcdv1alpha1.Etcd{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileEtcd implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileEtcd{}

// ReconcileEtcd reconciles a Etcd object
type ReconcileEtcd struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Etcd object and makes changes based on the state read
// and what is in the Etcd.Spec
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileEtcd) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Etcd")

	// Fetch the Etcd instance
	instance := &etcdv1alpha1.Etcd{}
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

	// Check if this etcd association resource already exists
	found := &appsv1.StatefulSet{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating a new headlessService", "Pod.Namespace", instance.Namespace, "Pod.Name", instance.Name)
		headlessService := service.NewService(instance)
		err = r.client.Create(context.TODO(), headlessService)
		if err != nil {
			return reconcile.Result{}, err
		}
		ss := statefulset.NewStatefuleSet(instance)
		err = r.client.Create(context.TODO(), ss)
		if err != nil {
			go r.client.Delete(context.TODO(), headlessService)
			return reconcile.Result{}, err
		}
		instance.Annotations = map[string]string{"etcd.zrq.org.cn/spec": toString(instance)}
		retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			return r.client.Update(context.TODO(), instance)
		})
		if retryErr != nil {
			fmt.Println(retryErr.Error())
		}
		// Pod created successfully - don't requeue
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	}

	if !reflect.DeepEqual(instance.Spec, toSpec(instance.Annotations["etcd.zrq.org.cn/spec"])) {
		reqLogger.Info("etcd Spec has changed")
		ss := statefulset.NewStatefuleSet(instance)
		found.Spec = ss.Spec
		retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			return r.client.Update(context.TODO(), found)
		})
		if retryErr != nil {
			return reconcile.Result{}, err
		}

	}

	return reconcile.Result{}, nil
}

func toString(etcd *etcdv1alpha1.Etcd) string {
	data, _ := json.Marshal(etcd.Spec)
	return string(data)
}

func toSpec(data string) etcdv1alpha1.EtcdSpec {
	spec := etcdv1alpha1.EtcdSpec{}
	json.Unmarshal([]byte(data), &spec)
	return spec
}
