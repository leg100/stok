package github

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v31/github"
	"github.com/leg100/etok/api/etok.dev/v1alpha1"
	cmdutil "github.com/leg100/etok/cmd/util"
	"github.com/leg100/etok/pkg/client"
	"github.com/leg100/etok/pkg/controllers"
	"github.com/leg100/etok/pkg/launcher"
	"github.com/leg100/etok/pkg/util"
	"k8s.io/klog/v2"
)

const (
	// https://github.community/t/undocumented-65535-character-limit-on-requests/117564
	defaultMaxFieldSize = 65535
)

// An etokRun is a etok run that maps to a github check run
type etokRun struct {
	id           string
	args         []string
	command      string
	launcherOpts *launcher.LauncherOptions
	out          []byte
	previous     string
	workspace    *v1alpha1.Workspace
	maxFieldSize int

	completed bool
	err       error

	repo *repo

	// Check Run ID is only populated after the gh client creates the check run.
	// Once populated, we can use it to update the check run.
	checkRunId *int64

	etokAppOptions
}

// Constructor for an etok run obj
func newEtokRun(kClient *client.Client, command, previous string, workspace *v1alpha1.Workspace, repo *repo, opts etokAppOptions) (*etokRun, error) {
	id := fmt.Sprintf("run-%s", util.GenerateRandomString(5))

	args, err := launcherArgs(id, command, previous)
	if err != nil {
		return nil, err
	}

	run := etokRun{
		command:      command,
		id:           id,
		maxFieldSize: defaultMaxFieldSize,
		previous:     previous,
		repo:         repo,
		workspace:    workspace,
	}

	run.launcherOpts = &launcher.LauncherOptions{
		Client:     kClient,
		Workspace:  workspace.Name,
		Namespace:  workspace.Namespace,
		DisableTTY: true,
		Command:    "sh",
		Args:       args,
		Path:       repo.workspacePath(workspace),
		IOStreams: &cmdutil.IOStreams{
			Out: &run,
		},
		RunName:     id,
		Status:      &opts.runStatus,
		GetLogsFunc: opts.getLogsFunc,
	}

	return &run, nil
}

// Run the etok run
func (r *etokRun) run() error {
	l := launcher.NewLauncher(r.launcherOpts)

	run, err := l.Launch(context.Background())
	if err != nil {
		r.err = err
		r.completed = true
		return err
	}

	err = l.Monitor(run, context.Background())
	r.err = err
	r.completed = true

	return err
}

// Provide name for check run. Indicates the etok workspace for which the check
// run is running and summarises its current state. Github only shows the first
// 33 chars (and then an ellipsis) on the check runs page, so it's important to
// use those chars effectively.
func (r *etokRun) name() string {
	// Check run name always begins with full workspace name
	name := r.workspace.Namespace + "/" + r.workspace.Name

	// Next part of name is the command name
	command := r.command
	if r.command == "plan" && r.completed {
		// Upon completion of a plan, instead of showing 'plan', show summary of
		// changes
		plan, err := parsePlanOutput(string(r.out))
		if err != nil {
			// Just fallback to showing 'plan' and log error
			klog.Errorf("error parsing plan output for %s: %s", r.id, err.Error())
		} else {
			command = plan.summary()
		}
	}
	name += fmt.Sprintf("[%s]", command)

	// Next part is the run id (run-123yx). GH is likely to cut this short with
	// a '...' so snip off the redundant prefix 'run-' and just show the ID.
	// That way the ID - the important bit - is more likely to be visible to the
	// user.
	var id string
	idparts := strings.Split(r.id, "-")
	if len(idparts) == 2 {
		id = idparts[1]
	}
	name += " " + id

	return name
}

// Provide current status of check run
func (u *etokRun) status() *string {
	if u.completed {
		return github.String("completed")
	}
	return github.String("in_progress")
}

// Provide the 'title' of a check run
func (u *etokRun) title() string {
	return u.id
}

// Provide the external ID of a check run
func (u *etokRun) externalID() *string {
	return github.String(u.workspace.Namespace + "/" + u.id)
}

// Populate the 'summary' text field of a check run
func (u *etokRun) summary() string {
	if u.err != nil {
		return u.err.Error()
	}

	return fmt.Sprintf("Run `kubectl logs -f pods/%s`", u.id)
}

// Populate the 'details' text field of a check run
func (o *etokRun) details() string {
	diffStart := "```diff\n"
	diffEnd := "\n```\n"

	if (len(diffStart) + len(o.out) + len(diffEnd)) <= o.maxFieldSize {
		return diffStart + string(bytes.TrimSpace(o.out)) + diffEnd
	}

	// Max bytes exceeded. Fetch new start position max bytes into output.
	start := len(o.out) - o.maxFieldSize

	// Account for diff headers
	start += len(diffStart)
	start += len(diffEnd)

	// Add message explaining reason. The number of bytes skipped is inaccurate:
	// it doesn't account for additional bytes skipped in order to accommodate
	// this message.
	exceeded := fmt.Sprintf("--- exceeded limit of %d bytes; skipping first %d bytes ---\n", o.maxFieldSize, start)

	// Adjust start position to account for message
	start += len(exceeded)

	// Ensure output does not start half way through a line. Remove bytes
	// leading up to and including the first new line character.
	if i := bytes.IndexByte(o.out[start:], '\n'); i > -1 {
		start += i + 1
	}

	// Trim off any remaining leading or trailing new lines
	trimmed := bytes.Trim(o.out[start:], "\n")

	return diffStart + exceeded + string(trimmed) + diffEnd
}

// Write implements io.Writer. The launcher calls Write with the logs it streams
// from the pod. As well as storing the logs to an internal buffer for
// populating the text fields of the check run, it provides an opportunity to
// strip out unnecessary content.
func (o *etokRun) Write(p []byte) (int, error) {
	// Total bytes written
	var written int

	r := bufio.NewReader(bytes.NewBuffer(p))
	// Read segments of bytes delimited with a new line.
	for {
		line, err := r.ReadBytes('\n')
		written += len(line)
		if err == io.EOF {
			return written, nil
		}
		if err != nil {
			return written, err
		}

		if o.stripRefreshing && bytes.Contains(line, []byte(": Refreshing state... ")) {
			continue
		}

		if bytes.HasPrefix(line, []byte("  +")) || bytes.HasPrefix(line, []byte("  -")) || bytes.HasPrefix(line, []byte("  ~")) {
			// Trigger diff color highlighting by unindenting lines beginning
			// with +/-/~
			line = bytes.TrimLeft(line, " ")
		}

		o.out = append(o.out, line...)
	}
}

// Update actually updates the check run on GH. It does so idempotently: if the
// CR is yet to be created it will be created, and if it's already created,
// it'll be updated.
func (r *etokRun) invoke(client *GithubClient) error {
	op := etokRunOperation{
		etokRun: r,
	}

	if r.completed {
		op.setAction("Plan", "Re-run plan", "plan")

		if r.command == "plan" {
			op.setAction("Apply", "Apply plan", "apply")
		}

		if r.err != nil {
			op.conclusion = github.String("failure")
		} else {
			op.conclusion = github.String("success")
		}
	}

	if r.checkRunId != nil {
		err := op.update(context.Background(), client, *r.checkRunId)
		if err != nil {
			klog.Errorf("unable to update check run: %s", err.Error())
			return err
		}
		klog.InfoS("updated check run", "id", *r.checkRunId, "ref", r.repo.sha)
	} else {
		id, err := op.create(context.Background(), client)
		if err != nil {
			klog.Errorf("unable to create check run: %s", err.Error())
			return err
		}
		r.checkRunId = &id
		klog.InfoS("created check run", "id", id, "ref", r.repo.sha)
	}

	return nil
}

func launcherArgs(id, command, previous string) ([]string, error) {
	script := new(bytes.Buffer)

	// Default is to create a new plan file with a filename the same as the etok
	// run ID
	planPath := filepath.Join(controllers.PlansMountPath, id)
	if command == "apply" {
		// Apply uses the plan file from the previous run
		planPath = filepath.Join(controllers.PlansMountPath, previous)
	}

	if err := generateEtokRunScript(script, planPath, command); err != nil {
		klog.Errorf("unable to generate check run script: %s", err.Error())
		return nil, err
	}

	return []string{script.String()}, nil
}
