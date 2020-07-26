package controllers

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/leg100/stok/api"
	"github.com/leg100/stok/api/command"
	"github.com/leg100/stok/api/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/status"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type WorkspaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

// Reconcile reads that state of the cluster for a Workspace object and makes changes based on the state read
// and what is in the Workspace.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *WorkspaceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	reqLogger := r.Log.WithValues("workspace", req.NamespacedName)
	reqLogger.V(0).Info("Reconciling Workspace")

	// Fetch the Workspace instance
	instance := &v1alpha1.Workspace{}
	err := r.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "Error retrieving workspace")
		return ctrl.Result{}, err
	}

	// Because it is a required attribute we need to set the queue status to an empty array if it
	// is not already set
	if instance.Status.Queue == nil {
		instance.Status.Queue = []string{}
	}

	// Check ServiceAccount exists (if specified)
	if instance.Spec.ServiceAccountName != "" {
		serviceAccountNamespacedName := types.NamespacedName{Name: instance.Spec.ServiceAccountName, Namespace: req.Namespace}
		err = r.Get(context.TODO(), serviceAccountNamespacedName, &corev1.ServiceAccount{})
		if errors.IsNotFound(err) {
			instance.Status.Conditions.SetCondition(status.Condition{
				Type:    v1alpha1.ConditionHealthy,
				Status:  corev1.ConditionFalse,
				Reason:  v1alpha1.ReasonMissingResource,
				Message: "ServiceAccount resource not found",
			})
			if err = r.Status().Update(context.TODO(), instance); err != nil {
				return ctrl.Result{}, fmt.Errorf("Setting healthy condition: %w", err)
			}
			// Pointless proceeding any further or requeuing a request (the service account watch will
			// take care of triggering a request)
			return ctrl.Result{}, nil
		} else if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Flag success if Secret is either:
	// (a) unspecified and thus not required
	// (b) specified and successfully found
	if instance.Spec.SecretName != "" {
		secretNamespacedName := types.NamespacedName{Name: instance.Spec.SecretName, Namespace: req.Namespace}
		err = r.Get(context.TODO(), secretNamespacedName, &corev1.Secret{})
		if errors.IsNotFound(err) {
			instance.Status.Conditions.SetCondition(status.Condition{
				Type:    v1alpha1.ConditionHealthy,
				Status:  corev1.ConditionFalse,
				Reason:  v1alpha1.ReasonMissingResource,
				Message: "Secret resource not found",
			})
			if err = r.Status().Update(context.TODO(), instance); err != nil {
				return ctrl.Result{}, fmt.Errorf("Setting healthy condition: %w", err)
			}
			// Pointless proceeding any further or requeuing a request (the secret watch will
			// take care of triggering a request)
			return ctrl.Result{}, nil
		} else if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Set Healthy Condition since all pre-requisities satisfied
	// TODO: only set this after confirming PVC (see below) is present
	instance.Status.Conditions.SetCondition(status.Condition{
		Type:    v1alpha1.ConditionHealthy,
		Status:  corev1.ConditionTrue,
		Reason:  v1alpha1.ReasonAllResourcesFound,
		Message: "All prerequisite resources found",
	})
	if err := r.Status().Update(context.TODO(), instance); err != nil {
		return ctrl.Result{}, fmt.Errorf("Setting healthy condition: %w", err)
	}

	// Manage PVC for workspace cache dir
	pvc := newPVCForCR(instance)
	if err := r.manageControllee(instance, reqLogger, pvc); err != nil {
		return ctrl.Result{}, err
	}

	// Manage Role for workspace
	role := newRoleForCR(instance)
	if err := r.manageControllee(instance, reqLogger, role); err != nil {
		return ctrl.Result{}, err
	}

	// Manage RoleBinding for workspace
	binding := newRoleBindingForCR(instance, role)
	if err := r.manageControllee(instance, reqLogger, binding); err != nil {
		return ctrl.Result{}, err
	}

	// Fetch list of commands that belong to this workspace (its workspace label specifies this workspace)
	var cmdList []command.Interface
	// Fetch and append each type of command to cmdList
	for _, kind := range command.CommandKinds {
		ccList, err := r.Scheme.New(v1alpha1.SchemeGroupVersion.WithKind(command.CollectionKind(kind)))
		if err != nil {
			return ctrl.Result{}, err
		}

		err = r.List(context.TODO(), ccList, client.InNamespace(req.Namespace), client.MatchingLabels{
			"workspace": req.Name,
		})
		if err != nil {
			return ctrl.Result{}, err
		}

		meta.EachListItem(ccList, func(o runtime.Object) error {
			cmdList = append(cmdList, o.(command.Interface))
			return nil
		})
	}

	// Filter out completed commands
	n := 0
	for _, cmd := range cmdList {
		if cond := cmd.GetConditions().IsTrueFor(v1alpha1.ConditionCompleted); !cond {
			cmdList[n] = cmd
			n++
		}
	}
	cmdList = cmdList[:n]

	// Filter out completed/deleted commands from existing queue, building new queue
	newQueue := []string{}
	for _, cmd := range instance.Status.Queue {
		if i := cmdListMatchingName(cmdList, cmd); i > -1 {
			// add to new queue
			newQueue = append(newQueue, cmd)
			// remove from cmd list
			cmdList = append(cmdList[:i], cmdList[i+1:]...)
		}
		// cmd not found, leave it out of new queue
	}
	// Append remainder of commands
	newQueue = append(newQueue, cmdListNames(cmdList)...)

	// update status if queue has changed
	if !reflect.DeepEqual(newQueue, instance.Status.Queue) {
		reqLogger.Info("Queue updated", "Old", fmt.Sprintf("%#v", instance.Status.Queue), "New", fmt.Sprintf("%#v", newQueue))
		instance.Status.Queue = newQueue
		if err := r.Status().Update(context.TODO(), instance); err != nil {
			return ctrl.Result{}, fmt.Errorf("Failed to update queue status: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

func (r *WorkspaceReconciler) manageControllee(ws *v1alpha1.Workspace, logger logr.Logger, controllee api.Object) error {
	log := logger.WithValues("Controllee.Kind", controllee.GetObjectKind().GroupVersionKind().Kind)

	// Set Workspace instance as the owner and controller
	if err := controllerutil.SetControllerReference(ws, controllee, r.Scheme); err != nil {
		log.Error(err, "Unable to set controller reference")
		return err
	}

	controlleeKey, err := client.ObjectKeyFromObject(controllee)
	if err != nil {
		return err
	}

	err = r.Get(context.TODO(), controlleeKey, controllee)
	if errors.IsNotFound(err) {
		if err = r.Create(context.TODO(), controllee); err != nil {
			log.Error(err, "Failed to create controllee", "Controllee.Name", controllee.GetName())
			return err
		}
	} else if err != nil {
		log.Error(err, "Error retrieving PVC")
		return err
	}

	log.Info("Created controllee", "Controllee.Name", controllee.GetName())
	return nil
}

func newPVCForCR(cr *v1alpha1.Workspace) api.Object {
	labels := map[string]string{
		"app": cr.Name,
	}

	size := v1alpha1.WorkspaceDefaultCacheSize
	if cr.Spec.Cache.Size != "" {
		size = cr.Spec.Cache.Size
	}

	pvc := corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolumeClaim",
			APIVersion: "",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(size),
				},
			},
		},
	}

	if cr.Spec.Cache.StorageClass != "" {
		pvc.Spec.StorageClassName = &cr.Spec.Cache.StorageClass
	}

	return &pvc
}

func newRoleForCR(cr *v1alpha1.Workspace) *rbacv1.Role {
	// Need TypeMeta in order to extract Kind in manageControllee()
	return &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Role",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stok-workspace-" + cr.GetName(),
			Namespace: cr.GetNamespace(),
			Labels: map[string]string{
				"app.kubernetes.io/component": "workspace",
				"workspace":                   cr.GetName(),
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"stok.goalspike.com"},
				Resources: []string{"*"},
				Verbs:     []string{"get"},
			},
		},
	}
}

func newRoleBindingForCR(cr *v1alpha1.Workspace, role *rbacv1.Role) *rbacv1.RoleBinding {
	// Need TypeMeta in order to extract Kind in manageControllee()
	binding := rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stok-workspace-" + cr.GetName(),
			Namespace: cr.GetNamespace(),
			Labels: map[string]string{
				"app.kubernetes.io/component": "workspace",
				"workspace":                   cr.GetName(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     role.GetName(),
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	if cr.Spec.ServiceAccountName != "" {
		binding.Subjects = []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      cr.Spec.ServiceAccountName,
				Namespace: cr.GetNamespace(),
			},
		}
	} else {
		binding.Subjects = []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "default",
				Namespace: cr.GetNamespace(),
			},
		}
	}

	return &binding
}

func cmdListMatchingName(cmdList []command.Interface, name string) int {
	for i, cmd := range cmdList {
		if cmd.GetName() == name {
			return i
		}
	}
	return -1
}

func cmdListNames(cmdList []command.Interface) []string {
	names := []string{}
	for _, cmd := range cmdList {
		names = append(names, cmd.GetName())
	}
	return names
}

func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	blder := ctrl.NewControllerManagedBy(mgr)

	// Watch for changes to primary resource Workspace
	blder.For(&v1alpha1.Workspace{})

	// Watch for changes to secondary resource PVCs and requeue the owner Workspace
	blder.Owns(&corev1.PersistentVolumeClaim{})

	_ = mgr.GetFieldIndexer().IndexField(context.TODO(), &v1alpha1.Workspace{}, "spec.serviceAccountName", func(o runtime.Object) []string {
		sa := o.(*v1alpha1.Workspace).Spec.ServiceAccountName
		if sa == "" {
			return nil
		}
		return []string{sa}
	})

	_ = mgr.GetFieldIndexer().IndexField(context.TODO(), &v1alpha1.Workspace{}, "spec.secretName", func(o runtime.Object) []string {
		sa := o.(*v1alpha1.Workspace).Spec.SecretName
		if sa == "" {
			return nil
		}
		return []string{sa}
	})

	// Watch for changes to service accounts and secrets, because they may affect the functionality
	// of a Workspace (e.g. the deletion of a service account)
	blder = blder.Watches(&source.Kind{Type: &corev1.ServiceAccount{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
			var reqs []reconcile.Request
			wsList := &v1alpha1.WorkspaceList{}
			filter := client.MatchingFields{"spec.serviceAccountName": a.Meta.GetName()}
			err := r.List(context.TODO(), wsList, client.InNamespace(a.Meta.GetNamespace()), filter)
			if err != nil {
				return reqs
			}
			meta.EachListItem(wsList, func(ws runtime.Object) error {
				reqs = append(reqs, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      ws.(*v1alpha1.Workspace).GetName(),
						Namespace: a.Meta.GetNamespace(),
					},
				})
				return nil
			})
			return reqs
		}),
	})

	blder = blder.Watches(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
			var reqs []reconcile.Request
			wsList := &v1alpha1.WorkspaceList{}
			filter := client.MatchingFields{"spec.secretName": a.Meta.GetName()}
			err := r.List(context.TODO(), wsList, client.InNamespace(a.Meta.GetNamespace()), filter)
			if err != nil {
				return reqs
			}
			meta.EachListItem(wsList, func(ws runtime.Object) error {
				reqs = append(reqs, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      ws.(*v1alpha1.Workspace).GetName(),
						Namespace: a.Meta.GetNamespace(),
					},
				})
				return nil
			})
			return reqs
		}),
	})

	// Watch for changes to command resources and requeue the associated Workspace.
	for _, kind := range command.CommandKinds {
		o, err := r.Scheme.New(v1alpha1.SchemeGroupVersion.WithKind(kind))
		if err != nil {
			return err
		}

		blder = blder.Watches(&source.Kind{Type: o}, &handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
				cmd := a.Object.(command.Interface)
				if ws, ok := cmd.GetLabels()["workspace"]; ok {
					return []reconcile.Request{
						{
							NamespacedName: types.NamespacedName{
								Name:      ws,
								Namespace: a.Meta.GetNamespace(),
							},
						},
					}
				}
				return []reconcile.Request{}
			}),
		})
	}

	return blder.Complete(r)
}