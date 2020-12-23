package e2e

import (
	"context"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	expect "github.com/google/goexpect"
	"github.com/stretchr/testify/require"
)

// E2E test of workspace commands (new, list, etc)
func TestWorkspace(t *testing.T) {
	// The e2e tests, each composed of multiple steps
	tests := []test{
		{
			name:      "defaults",
			namespace: "e2e-workspace-tests",
		},
	}

	// Enumerate e2e tests
	for _, tt := range tests {
		// Create namespace for each e2e test
		_, err := client.KubeClient.CoreV1().Namespaces().Create(context.Background(), newNamespace(tt.namespace), metav1.CreateOptions{})
		if err != nil {
			// Only a namespace already exists error is acceptable
			require.True(t, errors.IsAlreadyExists(err))
		}

		t.Parallel()
		t.Run(tt.name, func(t *testing.T) {
			// Create temp dir of terraform configs and set pwd to root module
			createTerraformConfigs(t)

			t.Run("create workspace foo", func(t *testing.T) {
				require.NoError(t, step(tt,
					[]string{buildPath, "workspace", "new", "foo",
						"--namespace", tt.namespace,
						"--context", *kubectx,
					},
					[]expect.Batcher{
						&expect.BExp{R: fmt.Sprintf("Created workspace %s/foo", tt.namespace)},
						&expect.BExp{R: "Skipping terraform installation"},
					}))
			})

			t.Run("create workspace bar with custom terraform version", func(t *testing.T) {
				version := "0.12.17"
				require.NoError(t, step(tt,
					[]string{buildPath, "workspace", "new", "bar",
						"--namespace", tt.namespace,
						"--context", *kubectx,
						"--terraform-version", version,
					},
					[]expect.Batcher{
						&expect.BExp{R: fmt.Sprintf("Created workspace %s/bar", tt.namespace)},
						&expect.BExp{R: fmt.Sprintf("Requested terraform version is %s", version)},
						&expect.BExp{R: fmt.Sprintf("Downloading terraform %s", version)},
					}))
			})

			t.Run("list workspaces", func(t *testing.T) {
				require.NoError(t, step(tt,
					[]string{buildPath, "workspace", "list",
						"--context", *kubectx,
					},
					[]expect.Batcher{
						&expect.BExp{R: fmt.Sprintf("\\*\t%s_%s\n\t%s_%s", tt.namespace, "bar", tt.namespace, "foo")},
					}))
			})

			t.Run("show current workspace bar", func(t *testing.T) {
				require.NoError(t, step(tt,
					[]string{buildPath, "workspace", "show"},
					[]expect.Batcher{
						&expect.BExp{R: fmt.Sprintf("%s_%s", tt.namespace, "bar")},
					}))
			})

			t.Run("select workspace foo", func(t *testing.T) {
				require.NoError(t, step(tt,
					[]string{buildPath, "workspace", "select", "foo", "--namespace", tt.namespace},
					[]expect.Batcher{
						&expect.BExp{R: fmt.Sprintf("Current workspace now: %s_%s", tt.namespace, "foo")},
					}))
			})

			t.Run("delete workspace foo", func(t *testing.T) {
				require.NoError(t, step(tt,
					[]string{buildPath, "workspace", "delete", "foo",
						"--namespace", tt.namespace,
						"--context", *kubectx},
					[]expect.Batcher{
						&expect.BExp{R: fmt.Sprintf("Deleted workspace %s/foo", tt.namespace)},
					}))
			})

			t.Run("delete workspace bar", func(t *testing.T) {
				require.NoError(t, step(tt,
					[]string{buildPath, "workspace", "delete", "bar",
						"--namespace", tt.namespace,
						"--context", *kubectx},
					[]expect.Batcher{
						&expect.BExp{R: fmt.Sprintf("Deleted workspace %s/bar", tt.namespace)},
					}))
			})
		})

		// Delete namespace for each e2e test, ignore any errors
		if !*disableNamespaceDelete {
			_ = client.KubeClient.CoreV1().Namespaces().Delete(context.Background(), tt.namespace, metav1.DeleteOptions{})
		}
	}
}
