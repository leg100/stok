package command

import (
	"context"
	"testing"

	terraformv1alpha1 "github.com/leg100/terraform-operator/pkg/apis/terraform/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var command = terraformv1alpha1.Command{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "command-1",
		Namespace: "operator-test",
		Labels: map[string]string{
			"workspace": "workspace-1",
		},
	},
	Spec: terraformv1alpha1.CommandSpec{
		Args: []string{"version"},
	},
}

var workspaceEmptyQueue = terraformv1alpha1.Workspace{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "workspace-1",
		Namespace: "operator-test",
	},
}

var workspaceQueueOfOne = terraformv1alpha1.Workspace{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "workspace-1",
		Namespace: "operator-test",
	},
	Status: terraformv1alpha1.WorkspaceStatus{
		Queue: []string{"command-1"},
	},
}

var successfullyCompletedPod = corev1.Pod{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "command-1",
		Namespace: "operator-test",
	},
	Status: corev1.PodStatus{
		Phase: corev1.PodSucceeded,
	},
}

func newTrue() *bool {
	b := true
	return &b
}

func TestReconcileCommand(t *testing.T) {
	tests := []struct {
		name                   string
		objs                   []runtime.Object
		wantPod                bool
		wantReadyCondition     corev1.ConditionStatus
		wantActiveCondition    corev1.ConditionStatus
		wantCompletedCondition corev1.ConditionStatus
		wantRequeue            bool
	}{
		{
			name: "Unqueued command",
			objs: []runtime.Object{
				runtime.Object(&command),
				runtime.Object(&workspaceEmptyQueue),
			},
			wantPod:                false,
			wantReadyCondition:     corev1.ConditionTrue,
			wantActiveCondition:    corev1.ConditionFalse,
			wantCompletedCondition: corev1.ConditionUnknown,
			wantRequeue:            false,
		},
		{
			name: "Command at front of queue",
			objs: []runtime.Object{
				runtime.Object(&command),
				runtime.Object(&workspaceQueueOfOne),
			},
			wantPod:                true,
			wantReadyCondition:     corev1.ConditionTrue,
			wantActiveCondition:    corev1.ConditionTrue,
			wantCompletedCondition: corev1.ConditionUnknown,
			wantRequeue:            false,
		},
		{
			name: "Successfully completed command",
			objs: []runtime.Object{
				runtime.Object(&command),
				runtime.Object(&workspaceQueueOfOne),
				runtime.Object(&successfullyCompletedPod),
			},
			wantPod:                true,
			wantReadyCondition:     corev1.ConditionTrue,
			wantActiveCondition:    corev1.ConditionFalse,
			wantCompletedCondition: corev1.ConditionTrue,
			wantRequeue:            false,
		},
	}
	s := scheme.Scheme
	s.AddKnownTypes(terraformv1alpha1.SchemeGroupVersion, &terraformv1alpha1.Workspace{}, &terraformv1alpha1.CommandList{}, &terraformv1alpha1.Command{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := fake.NewFakeClientWithScheme(s, tt.objs...)

			r := &ReconcileCommand{client: cl, scheme: s}
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      command.GetName(),
					Namespace: command.GetNamespace(),
				},
			}
			res, err := r.Reconcile(req)
			if err != nil {
				t.Fatalf("reconcile: (%v)", err)
			}

			if tt.wantRequeue && !res.Requeue {
				t.Error("expected reconcile to requeue")
			}

			pod := &corev1.Pod{}
			err = r.client.Get(context.TODO(), req.NamespacedName, pod)
			if err != nil && !errors.IsNotFound(err) {
				t.Fatalf("error fetching pod %v", err)
			}
			if tt.wantPod && errors.IsNotFound(err) {
				t.Errorf("wanted pod but pod not found")
			}
			if !tt.wantPod && !errors.IsNotFound(err) {
				t.Errorf("did not want pod but pod found")
			}

			command := &terraformv1alpha1.Command{}
			err = r.client.Get(context.TODO(), req.NamespacedName, command)
			if err != nil {
				t.Fatalf("get command: (%v)", err)
			}

			assertCondition(t, command, "Completed", tt.wantCompletedCondition)
			assertCondition(t, command, "Ready", tt.wantReadyCondition)
			assertCondition(t, command, "Active", tt.wantActiveCondition)
		})
	}
}

func assertCondition(t *testing.T, command *terraformv1alpha1.Command, conditionType string, want corev1.ConditionStatus) {
	if command.Status.Conditions.IsUnknownFor(status.ConditionType(conditionType)) && want != corev1.ConditionUnknown ||
		command.Status.Conditions.IsTrueFor(status.ConditionType(conditionType)) && want != corev1.ConditionTrue ||
		command.Status.Conditions.IsFalseFor(status.ConditionType(conditionType)) && want != corev1.ConditionFalse {

		t.Errorf("expected %s status to be %v, got %v", conditionType, want, command.Status.Conditions.GetCondition(status.ConditionType(conditionType)))
	}
}