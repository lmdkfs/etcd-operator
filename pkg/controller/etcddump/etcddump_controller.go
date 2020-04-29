package etcddump

import (
	"context"
	"encoding/json"
	"fmt"
	etcdv1alpha1 "github.com/lmdkfs/etcd-operator/pkg/apis/etcd/v1alpha1"
	cron "gopkg.in/robfig/cron.v2"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"os/exec"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

var log = logf.Log.WithName("controller_etcddump")

var (
	dumpCron                = cron.New()
	DefaultDumpFileTemplate = "root/%v_%v_%v.db"
	location                = "storageUrl/fileName"

	LocalFileDir = "/tmp/etcd.db"
)

func init() {
	dumpCron.Start()
}

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Etcddump Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileEtcddump{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("etcddump-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Etcddump
	err = c.Watch(&source.Kind{Type: &etcdv1alpha1.Etcddump{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	//// Watch for changes to secondary resource Pods and requeue the owner Etcddump
	//err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
	//	IsController: true,
	//	OwnerType:    &etcdv1alpha1.Etcddump{},
	//})
	//if err != nil {
	//	return err
	//}

	return nil
}

// blank assignment to verify that ReconcileEtcddump implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileEtcddump{}

// ReconcileEtcddump reconciles a Etcddump object
type ReconcileEtcddump struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Etcddump object and makes changes based on the state read
// and what is in the Etcddump.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileEtcddump) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Etcddump")

	// Fetch the Etcddump instance
	instance := &etcdv1alpha1.Etcddump{}
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
		// remove cron dump
		return reconcile.Result{}, nil
	}

	if !reflect.DeepEqual(instance.Spec, toSpec(instance.Annotations["etcd.zrq.org.cn/spec"])) {
		if err := r.ProcessDumpItem(instance); err != nil {
			return reconcile.Result{}, err
		}
		if instance.Annotations != nil {
			instance.Annotations["etcd.zrq.org.cn/spec"] = toString(instance.Spec)
		} else {
			instance.Annotations = map[string]string{"etcd.zrq.org.cn/spec": toString(instance.Spec)}
		}
		retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			return r.client.Update(context.TODO(), instance)
		})
		if retryErr != nil {
			log.Info("when dump etcd success, update etcddump %v err: %v", request.String(), err)
		}

	}
	log.Info("but EtcdDump spec no change, so it's ignore")
	return reconcile.Result{}, nil
}

// newPodForCR returns a busybox pod with the same name/namespace as the cr

func (r *ReconcileEtcddump) ProcessDumpItem(dump *etcdv1alpha1.Etcddump) error {
	if dump.Spec.Scheduler != "" {
		dumpCron.AddFunc(dump.Spec.Scheduler, func() {
			if err := r.CreateManulDump(dump); err != nil {
				log.Info("cron dump etcd cluster %v/%v err: %v", dump.Namespace, dump.Spec.ClusterReference, err)
				return
			}
			log.Info("cron dump etcd cluster %v/%v success", dump.Namespace, dump.Spec.ClusterReference)
		})
	} else {
		if err := r.CreateManulDump(dump); err != nil {
			log.Info("manul dump etcd cluster %v/%v err: %v", dump.Namespace, dump.Spec.ClusterReference, err)
			return err
		}
		log.Info("manul dump etcd cluster %v/%v sucess", dump.Namespace, dump.Spec.ClusterReference)
	}
	return nil

}

func (r *ReconcileEtcddump) CreateManulDump(dump *etcdv1alpha1.Etcddump) error {
	// dump before
	dump.Status = etcdv1alpha1.EtcddumpStatus{}
	running := etcdv1alpha1.EtcdDumpCondition{
		Ready:                 true,
		Location:              "",
		LastedTranslationTime: metav1.Now(),
		Reason:                "begin dump",
		Message:               "",
	}
	if err := r.updateStatus(dump, running, etcdv1alpha1.EtcdDumpRunning); err != nil {
		return err
	}

	// exec dump cmd
	dumpFileName := fmt.Sprintf(DefaultDumpFileTemplate, dump.Namespace, dump.Spec.ClusterReference, time.Now().Format("2020040405"))
	dumpArgs := fmt.Sprintf("kubectl -n %v exec %v-0 -- sh -c 'ETCDCTL_API=3 etcdctl snapshot save %v'", dump.Namespace, dump.Spec.ClusterReference, dumpFileName)
	dumpCmd := exec.Command("/bin/sh", "-c", dumpArgs)
	dumpOut, err := dumpCmd.CombinedOutput()
	if err != nil {
		execDump := etcdv1alpha1.EtcdDumpCondition{
			Ready:                 false,
			Location:              "",
			LastedTranslationTime: metav1.Now(),
			Reason:                "dump cmd exec failed",
			Message:               fmt.Sprintf("exec cmd : %v, cmd response : %v", dumpArgs, string(dumpOut)),
		}
		if updateErr := r.updateStatus(dump, execDump, etcdv1alpha1.EtcdDumpRunning); updateErr != nil {
			return updateErr
		}
		return fmt.Errorf("exec cmd: %v, cmd response : %v", dumpArgs, string(dumpOut))
	}

	execDump := etcdv1alpha1.EtcdDumpCondition{
		Ready:                 true,
		Location:              "",
		LastedTranslationTime: metav1.Now(),
		Reason:                "dump cmd exec success",
		Message:               "",
	}
	if updateErr := r.updateStatus(dump, execDump, etcdv1alpha1.EtcdDumpRunning); updateErr != nil {
		return updateErr
	}

	// get dump file to operator container
	cpArgs := fmt.Sprintf("kubectl cp %v/%v-0:%v %v", dump.Namespace, dump.Spec.ClusterReference, dumpFileName, LocalFileDir)
	cpCmd := exec.Command("/bin/sh", "-c", cpArgs)
	cpOut, err := cpCmd.CombinedOutput()
	if err != nil {
		execDump := etcdv1alpha1.EtcdDumpCondition{
			Ready:                 false,
			Location:              "",
			LastedTranslationTime: metav1.Now(),
			Reason:                "cp cmd exec failed",
			Message:               fmt.Sprintf("exec cmd : %v, cmd response : %v", cpArgs, string(cpOut)),
		}
		if updateErr := r.updateStatus(dump, execDump, etcdv1alpha1.EtcdDumpRunning); updateErr != nil {
			return updateErr
		}
		return fmt.Errorf("exec cmd : %v, cmd response: %v", cpArgs, string(cpOut))
	}
	cpDump := etcdv1alpha1.EtcdDumpCondition{
		Ready:                 true,
		Location:              "",
		LastedTranslationTime: metav1.Now(),
		Reason:                "cp cmd exec success",
		Message:               "",
	}
	if updateErr := r.updateStatus(dump, cpDump, etcdv1alpha1.EtcdDumpRunning); updateErr != nil {
		return updateErr
	}
	rmCmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("kubectl -n %v exec  %v-0 -- rm -f %v", dump.Namespace, dump.Spec.ClusterReference, dumpFileName))
	rmOut, err := rmCmd.CombinedOutput()
	if err != nil {
		log.Info("exec cmd: %v, cmd response : %v", fmt.Sprintf("kubectl -n %v exec %v-0 -- rm -f %v", dump.Namespace, dump.Spec.ClusterReference, dumpFileName), string(rmOut))
	}

	upload := etcdv1alpha1.EtcdDumpCondition{Ready: true, Location: location, LastedTranslationTime: metav1.Now(), Reason: "upload dump data to store sunccess", Message: ""}
	if updateErr := r.updateStatus(dump, upload, etcdv1alpha1.EtcdDumpComplated); updateErr != nil {
		return updateErr
	}
	return nil

}

func (r *ReconcileEtcddump) updateStatus(dump *etcdv1alpha1.Etcddump, condition etcdv1alpha1.EtcdDumpCondition, phase etcdv1alpha1.EtcdDumpPhase) error {
	dump.Status.Phase = phase
	dump.Status.Conditions = append(dump.Status.Conditions, condition)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.client.Status().Update(context.TODO(), dump)
	})
}
func toString(dumpSpec etcdv1alpha1.EtcddumpSpec) string {
	data, _ := json.Marshal(dumpSpec)
	return string(data)

}

func toSpec(data string) etcdv1alpha1.EtcddumpSpec {
	spec := etcdv1alpha1.EtcddumpSpec{}
	json.Unmarshal([]byte(data), &spec)
	return spec
}
