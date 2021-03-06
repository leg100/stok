package workspace

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/leg100/etok/api/etok.dev/v1alpha1"
	etokerrors "github.com/leg100/etok/pkg/errors"
	"github.com/leg100/etok/pkg/handlers"
	"github.com/leg100/etok/pkg/repo"

	"github.com/leg100/etok/cmd/envvars"
	cmdutil "github.com/leg100/etok/cmd/util"
	"github.com/leg100/etok/pkg/env"
	"github.com/leg100/etok/pkg/logstreamer"
	"github.com/leg100/etok/pkg/testobj"
	"github.com/leg100/etok/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestNewWorkspace(t *testing.T) {
	var fakeError = errors.New("fake error")

	// Sanitize env vars
	envvars.UnsetEtokVars()

	tests := []struct {
		name             string
		args             []string
		envVars          map[string]string
		err              error
		overrideStatus   func(*v1alpha1.WorkspaceStatus)
		objs             []runtime.Object
		factoryOverrides func(*cmdutil.Factory)
		// Skip creating a mock git repo for the test (to deliberately trigger
		// an error)
		skipGitRepo bool
		assertions  func(*testutil.T, *newOptions)
	}{
		{
			name: "missing workspace name",
			args: []string{},
			err:  errWorkspaceNameArg,
		},
		{
			name: "create workspace",
			args: []string{"foo"},
			objs: []runtime.Object{testobj.WorkspacePod("default", "foo")},
			assertions: func(t *testutil.T, o *newOptions) {
				// Confirm workspace resource has been created
				_, err := o.WorkspacesClient("default").Get(context.Background(), "foo", metav1.GetOptions{})
				require.NoError(t, err)

				/// Confirm env file has been written
				etokenv, err := env.Read(o.path)
				require.NoError(t, err)
				assert.Equal(t, "default", etokenv.Namespace)
				assert.Equal(t, "foo", etokenv.Workspace)
			},
		},
		{
			name: "non-default namespace",
			args: []string{"foo", "--namespace", "bar"},
			objs: []runtime.Object{testobj.WorkspacePod("bar", "foo")},
			assertions: func(t *testutil.T, o *newOptions) {
				assert.Equal(t, "bar", o.namespace)
			},
		},
		{
			name: "cleanup resources upon error",
			args: []string{"foo"},
			objs: []runtime.Object{testobj.WorkspacePod("default", "foo")},
			err:  fakeError,
			factoryOverrides: func(f *cmdutil.Factory) {
				f.GetLogsFunc = func(ctx context.Context, opts logstreamer.Options) (io.ReadCloser, error) {
					return nil, fakeError
				}
			},
			assertions: func(t *testutil.T, o *newOptions) {
				_, err := o.WorkspacesClient(o.namespace).Get(context.Background(), o.workspace, metav1.GetOptions{})
				assert.True(t, kerrors.IsNotFound(err))
			},
		},
		{
			name: "do not cleanup resources upon error",
			args: []string{"foo", "--no-cleanup"},
			objs: []runtime.Object{testobj.WorkspacePod("default", "foo")},
			err:  fakeError,
			factoryOverrides: func(f *cmdutil.Factory) {
				f.GetLogsFunc = func(ctx context.Context, opts logstreamer.Options) (io.ReadCloser, error) {
					return nil, fakeError
				}
			},
			assertions: func(t *testutil.T, o *newOptions) {
				_, err := o.WorkspacesClient(o.namespace).Get(context.Background(), o.workspace, metav1.GetOptions{})
				assert.NoError(t, err)
			},
		},
		{
			name: "default storage class is nil",
			args: []string{"foo"},
			objs: []runtime.Object{testobj.WorkspacePod("default", "foo")},
			assertions: func(t *testutil.T, o *newOptions) {
				// Get workspace
				ws, err := o.WorkspacesClient(o.namespace).Get(context.Background(), o.workspace, metav1.GetOptions{})
				require.NoError(t, err)

				assert.Nil(t, ws.Spec.Cache.StorageClass)
			},
		},
		{
			name: "explicitly set storage class to empty string",
			args: []string{"foo", "--storage-class", ""},
			objs: []runtime.Object{testobj.WorkspacePod("default", "foo")},
			assertions: func(t *testutil.T, o *newOptions) {
				// Get workspace
				ws, err := o.WorkspacesClient(o.namespace).Get(context.Background(), o.workspace, metav1.GetOptions{})
				require.NoError(t, err)

				assert.Equal(t, "", *ws.Spec.Cache.StorageClass)
			},
		},
		{
			name:    "explicitly set storage class via environment variable",
			args:    []string{"foo"},
			envVars: map[string]string{"ETOK_STORAGE_CLASS": "premium-super-fast"},
			objs:    []runtime.Object{testobj.WorkspacePod("default", "foo")},
			assertions: func(t *testutil.T, o *newOptions) {
				// Get workspace
				ws, err := o.WorkspacesClient(o.namespace).Get(context.Background(), o.workspace, metav1.GetOptions{})
				require.NoError(t, err)

				if assert.NotNil(t, ws.Spec.Cache.StorageClass) {
					assert.Equal(t, "premium-super-fast", *ws.Spec.Cache.StorageClass)
				}
			},
		},
		{
			name: "with cache settings",
			args: []string{"foo", "--size", "999Gi", "--storage-class", "lumpen-proletariat"},
			objs: []runtime.Object{testobj.WorkspacePod("default", "foo")},
			assertions: func(t *testutil.T, o *newOptions) {
				// Get workspace
				ws, err := o.WorkspacesClient(o.namespace).Get(context.Background(), o.workspace, metav1.GetOptions{})
				require.NoError(t, err)

				assert.Equal(t, "999Gi", ws.Spec.Cache.Size)
				assert.Equal(t, "lumpen-proletariat", *ws.Spec.Cache.StorageClass)
			},
		},
		{
			name: "with kube context flag",
			args: []string{"foo", "--context", "oz-cluster"},
			objs: []runtime.Object{testobj.WorkspacePod("default", "foo")},
			assertions: func(t *testutil.T, o *newOptions) {
				assert.Equal(t, "oz-cluster", o.kubeContext)
			},
		},
		{
			name: "log stream output",
			args: []string{"foo"},
			objs: []runtime.Object{testobj.WorkspacePod("default", "foo")},
			assertions: func(t *testutil.T, o *newOptions) {
				assert.Contains(t, o.Out.(*bytes.Buffer).String(), "fake logs")
			},
		},
		{
			name: "non-zero exit code",
			args: []string{"foo"},
			objs: []runtime.Object{testobj.WorkspacePod("default", "foo", testobj.WithInstallerExitCode(5))},
			err:  etokerrors.NewExitError(5),
		},
		{
			name: "set terraform version",
			args: []string{"foo", "--terraform-version", "0.12.17"},
			objs: []runtime.Object{testobj.WorkspacePod("default", "foo")},
			assertions: func(t *testutil.T, o *newOptions) {
				// Get workspace
				ws, err := o.WorkspacesClient(o.namespace).Get(context.Background(), o.workspace, metav1.GetOptions{})
				require.NoError(t, err)

				assert.Equal(t, "0.12.17", ws.Spec.TerraformVersion)
			},
		},
		{
			name: "set terraform variables",
			args: []string{"foo", "--variables", "foo=bar,baz=haj"},
			objs: []runtime.Object{testobj.WorkspacePod("default", "foo")},
			assertions: func(t *testutil.T, o *newOptions) {
				// Get workspace
				ws, err := o.WorkspacesClient(o.namespace).Get(context.Background(), o.workspace, metav1.GetOptions{})
				require.NoError(t, err)

				assert.Contains(t, ws.Spec.Variables, &v1alpha1.Variable{Key: "foo", Value: "bar"})
				assert.Contains(t, ws.Spec.Variables, &v1alpha1.Variable{Key: "baz", Value: "haj"})
			},
		},
		{
			name: "set environment variables",
			args: []string{"foo", "--environment-variables", "foo=bar,baz=haj"},
			objs: []runtime.Object{testobj.WorkspacePod("default", "foo")},
			assertions: func(t *testutil.T, o *newOptions) {
				// Get workspace
				ws, err := o.WorkspacesClient(o.namespace).Get(context.Background(), o.workspace, metav1.GetOptions{})
				require.NoError(t, err)

				assert.Contains(t, ws.Spec.Variables, &v1alpha1.Variable{Key: "foo", Value: "bar", EnvironmentVariable: true})
				assert.Contains(t, ws.Spec.Variables, &v1alpha1.Variable{Key: "baz", Value: "haj", EnvironmentVariable: true})
			},
		},
		{
			name: "set privileged commands",
			args: []string{"foo", "--privileged-commands", "apply,destroy,sh"},
			objs: []runtime.Object{testobj.WorkspacePod("default", "foo")},
			assertions: func(t *testutil.T, o *newOptions) {
				// Get workspace
				ws, err := o.WorkspacesClient(o.namespace).Get(context.Background(), o.workspace, metav1.GetOptions{})
				require.NoError(t, err)

				assert.Equal(t, []string{"apply", "destroy", "sh"}, ws.Spec.PrivilegedCommands)
			},
		},
		{
			// Mock a absent/misbehaving operator
			name: "reconcile timeout exceeded",
			args: []string{"foo", "--reconcile-timeout", "10ms"},
			overrideStatus: func(status *v1alpha1.WorkspaceStatus) {
				// Unset conditions, which should trigger timeout
				status.Conditions = []metav1.Condition{}
			},
			err: errReconcileTimeout,
		},
		{
			name: "pod timeout exceeded",
			args: []string{"foo", "--pod-timeout", "10ms"},
			// Deliberately omit pod
			objs: []runtime.Object{},
			err:  errPodTimeout,
		},
		{
			name: "workspace failure",
			args: []string{"foo"},
			objs: []runtime.Object{testobj.WorkspacePod("default", "foo")},
			overrideStatus: func(status *v1alpha1.WorkspaceStatus) {
				status.Conditions = []metav1.Condition{
					{
						Type:    v1alpha1.WorkspaceReadyCondition,
						Status:  metav1.ConditionFalse,
						Reason:  v1alpha1.FailureReason,
						Message: "mock failure",
					},
				}
			},
			err: handlers.ErrWorkspaceFailed,
		},
		{
			name: "ready timeout exceeded",
			args: []string{"foo", "--ready-timeout", "100ms"},
			objs: []runtime.Object{testobj.WorkspacePod("default", "foo")},
			overrideStatus: func(status *v1alpha1.WorkspaceStatus) {
				// Mock operator failing to provide ready condition status
				status.Conditions = nil
			},
			err: errReadyTimeout,
		},
		{
			name:        "path not within git repo",
			args:        []string{"foo"},
			skipGitRepo: true,
			err:         repo.ErrRepositoryNotFound,
		},
		{
			name: "workspace vcs settings",
			args: []string{"foo"},
			objs: []runtime.Object{testobj.WorkspacePod("default", "foo")},
			assertions: func(t *testutil.T, o *newOptions) {
				// Get workspace
				ws, err := o.WorkspacesClient(o.namespace).Get(context.Background(), o.workspace, metav1.GetOptions{})
				require.NoError(t, err)

				assert.Equal(t, ".", ws.Spec.VCS.WorkingDir)
				assert.Equal(t, "https://github.com/leg100/etok.git", ws.Spec.VCS.Repository)
			},
		},
	}

	for _, tt := range tests {
		testutil.Run(t, tt.name, func(t *testutil.T) {
			out := new(bytes.Buffer)
			f := cmdutil.NewFakeFactory(out, tt.objs...)

			if tt.factoryOverrides != nil {
				tt.factoryOverrides(f)
			}

			cmd, opts := newCmd(f)
			cmd.SetOut(out)
			cmd.SetArgs(tt.args)

			// Override flags with env vars
			t.SetEnvs(tt.envVars)
			envvars.SetFlagsFromEnvVariables(cmd)

			// Override path
			path := t.NewTempDir().Chdir().Root()
			opts.path = path

			// Make the path a git repo unless test specifies otherwise
			if !tt.skipGitRepo {
				repo, err := git.PlainInit(path, false)
				require.NoError(t, err)

				// Set a remote so that we can check that the code successfully
				// sets the remote URL in the workspace spec
				_, err = repo.CreateRemote(&config.RemoteConfig{
					Name: "origin",
					URLs: []string{"git@github.com:leg100/etok.git"},
				})
			}

			// Mock the workspace controller by setting status up front
			status := v1alpha1.WorkspaceStatus{
				Phase: v1alpha1.WorkspacePhaseReady,
				Conditions: []metav1.Condition{
					{
						Type:    v1alpha1.WorkspaceReadyCondition,
						Status:  metav1.ConditionTrue,
						Reason:  v1alpha1.ReadyReason,
						Message: "mock ready",
					},
				},
			}
			// Permit individual tests to override workspace status
			if tt.overrideStatus != nil {
				tt.overrideStatus(&status)
			}
			opts.status = &status

			err := cmd.ExecuteContext(context.Background())
			if !assert.True(t, errors.Is(err, tt.err)) {
				t.Logf("wanted %v but got %v", tt.err, err)
			}

			if tt.assertions != nil {
				tt.assertions(t, opts)
			}
		})
	}
}
