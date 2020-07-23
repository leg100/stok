package command

import (
	"context"
	"fmt"

	v1alpha1 "github.com/leg100/stok/pkg/apis/stok/v1alpha1"
	"github.com/leg100/stok/pkg/apis/stok/v1alpha1/command"
	"github.com/leg100/stok/util/slice"
	operatorstatus "github.com/operator-framework/operator-sdk/pkg/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type CommandReconciler struct {
	client       client.Client
	c            command.Interface
	resourceType string
	scheme       *runtime.Scheme
}

func (r *CommandReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log := logf.Log.WithName("controller_" + r.resourceType)
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.V(0).Info("Reconciling " + r.resourceType)

	err := r.client.Get(context.TODO(), request.NamespacedName, r.c)
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

	// Command completed, nothing more to be done
	if r.c.GetConditions().IsTrueFor(v1alpha1.ConditionCompleted) {
		return reconcile.Result{}, nil
	}

	//TODO: we really shouldn't be using a label for this but a spec field instead
	workspaceName, ok := r.c.GetLabels()["workspace"]
	if !ok {
		// Unrecoverable error, signal completion
		r.c.GetConditions().SetCondition(operatorstatus.Condition{
			Type:    v1alpha1.ConditionCompleted,
			Status:  corev1.ConditionTrue,
			Reason:  v1alpha1.ReasonWorkspaceUnspecified,
			Message: "Error: Workspace label not set",
		})
		_ = r.client.Status().Update(context.TODO(), r.c)
		return reconcile.Result{}, nil
	}

	// Fetch its Workspace object
	workspace := &v1alpha1.Workspace{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: workspaceName, Namespace: request.Namespace}, workspace)
	if errors.IsNotFound(err) {
		// Workspace not found, unlikely to be temporary, signal completion
		r.c.GetConditions().SetCondition(operatorstatus.Condition{
			Type:    v1alpha1.ConditionCompleted,
			Status:  corev1.ConditionTrue,
			Reason:  v1alpha1.ReasonWorkspaceNotFound,
			Message: fmt.Sprintf("Workspace '%s' not found", workspaceName),
		})
		_ = r.client.Status().Update(context.TODO(), r.c)
		return reconcile.Result{}, nil
	}

	// Check workspace queue position
	pos := slice.StringIndex(workspace.Status.Queue, r.c.GetName())
	// TODO: consider removing these status updates
	if pos > 0 {
		// Queued
		r.c.GetConditions().SetCondition(operatorstatus.Condition{
			Type:    v1alpha1.ConditionAttachable,
			Status:  corev1.ConditionFalse,
			Reason:  v1alpha1.ReasonQueued,
			Message: fmt.Sprintf("In workspace queue position %d", pos),
		})
		err = r.client.Status().Update(context.TODO(), r.c)
		return reconcile.Result{}, err
	}
	if pos < 0 {
		// Not yet queued
		r.c.GetConditions().SetCondition(operatorstatus.Condition{
			Type:    v1alpha1.ConditionAttachable,
			Status:  corev1.ConditionFalse,
			Reason:  v1alpha1.ReasonUnscheduled,
			Message: "Not yet scheduled by workspace",
		})
		err = r.client.Status().Update(context.TODO(), r.c)
		return reconcile.Result{}, err
	}

	// Check if client has told us they're ready and set condition accordingly
	if val, ok := r.c.GetAnnotations()["stok.goalspike.com/client"]; ok && val == "Ready" {
		r.c.GetConditions().SetCondition(operatorstatus.Condition{
			Type:    v1alpha1.ConditionClientReady,
			Status:  corev1.ConditionTrue,
			Reason:  v1alpha1.ReasonClientAttached,
			Message: "Client has attached to pod TTY",
		})
		if err = r.client.Status().Update(context.TODO(), r.c); err != nil {
			return reconcile.Result{}, err
		}
	}

	// Currently scheduled to run; get or create pod
	opts := podOpts{
		workspaceName:      workspace.GetName(),
		serviceAccountName: workspace.Spec.ServiceAccountName,
		secretName:         workspace.Spec.SecretName,
		pvcName:            workspace.GetName(),
		configMapName:      r.c.GetConfigMap(),
		configMapKey:       r.c.GetConfigMapKey(),
	}
	return r.reconcilePod(request, &opts)
}
