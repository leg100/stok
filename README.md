# Etok

**E**xecute **T**erraform **O**n **K**ubernetes

![demo](./demo.svg)

## Why

* Leverage Kubernetes' RBAC for terraform operations and state
* Single platform for end-user and CI/CD usage
* Queue terraform operations
* Leverage GCP workspace identity and other secret-less mechanisms
* Deploy infrastructure alongside applications

## Requirements

* A kubernetes cluster

## Install

Download and install the CLI from [releases](https://github.com/leg100/etok/releases).

Deploy the kubernetes operator onto your cluster:

```bash
etok install
```

## First run

Ensure you're in a directory containing terraform configuration:

```bash
$ cat random.tf
resource "random_id" "test" {
  byte_length = 2
}
```
You also want to specify the kubernetes backend like so:

```bash
$ cat backend.tf
terraform {
  backend "kubernetes" {}
}
```

Create a workspace:

```bash
etok workspace new default
```

Run terraform commands:

```bash
etok init
etok plan
etok apply
```

## Usage

Usage is similar to the terraform CLI:

```
Usage:
  etok [command]

Available Commands:
  apply        Run terraform apply
  console      Run terraform console
  destroy      Run terraform destroy
  fmt          Run terraform fmt
  force-unlock Run terraform force-unlock
  get          Run terraform get
  graph        Run terraform graph
  help         Help about any command
  import       Run terraform import
  init         Run terraform init
  install      Install etok operator
  output       Run terraform output
  plan         Run terraform plan
  providers    Run terraform providers
  refresh      Run terraform refresh
  sh           Run shell session in workspace
  show         Run terraform show
  state        Terraform state management
  taint        Run terraform taint
  untaint      Run terraform untaint
  validate     Run terraform validate
  version      Print client version information
  workspace    Etok workspace management

Flags:
      --add_dir_header                   If true, adds the file directory to the header of the log messages
      --alsologtostderr                  log to standard error as well as files
  -h, --help                             help for etok
      --log_backtrace_at traceLocation   when logging hits line file:N, emit a stack trace (default :0)
      --log_dir string                   If non-empty, write log files in this directory
      --log_file string                  If non-empty, use this log file
      --log_file_max_size uint           Defines the maximum size a log file can grow to. Unit is megabytes. If the value is 0, the maximum file size is unlimited. (defa
ult 1800)
      --logtostderr                      log to standard error instead of files (default true)
      --skip_headers                     If true, avoid header prefixes in the log messages
      --skip_log_headers                 If true, avoid headers when opening log files
      --stderrthreshold severity         logs at or above this threshold go to stderr (default 2)
  -v, --v Level                          number for the log level verbosity
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging

Use "etok [command] --help" for more information about a command.
```

## RBAC

The `install` command also installs ClusterRoles (and ClusterRoleBindings) for your convenience:

* [etok-user](./config/rbac/user.yaml): includes the permissions necessary for running unprivileged commands
* [etok-admin](./config/rbac/admin.yaml): additional permissions for managing workspaces and running [privileged commands](#privileged commands)

Amend the bindings accordingly to add/remove users. For example to amend the etok-user binding:

```
kubectl edit clusterrolebinding etok-user
```

Note: To restrict users to individual namespaces you'll want to create RoleBindings referencing the ClusterRoles.

## Privileged Commands

Commands can be specified as privileged. Only users possessing the RBAC permission to update the workspace (see above) can run privileged commands. Pass them via the `--privileged-commands` flag when creating a new workspace with `workspace new`.

## State

It's highly recommended that you use the [terraform kubernetes backend](https://www.terraform.io/docs/backends/types/kubernetes.html). If you do so, it's important you specify an empty backend configuration like so:

```
terraform {
  backend "kubernetes" {}
}
```

### State Persistence

Persistence of state to cloud storage is supported. Every update to the state is backed up to a cloud storage bucket. The state is restored if it's deleted.

To enable persistence, pass the name of an existing bucket via the `--backup-bucket` flag when creating a new workspace with `workspace new`. Note: only GCS is supported at present.

The operator is responsible for persisting the state. Therefore be sure to provide the appropriate credentials to the operator at install time. Either provide the path to a file containing a GCP service account key via the `--secret-file` flag, or setup workload identity (see below). The service account needs the following permissions on the bucket:

```
storage.buckets.get
storage.objects.create
storage.objects.delete
storage.objects.get
```

## Credentials

Etok looks for credentials in a secret named `etok`. If found, the credentials contained within are made available to terraform as environment variables.

For instance to set credentials for the [GCP provider](https://www.terraform.io/docs/providers/google/guides/provider_reference.html#full-reference):

```
kubectl create secret generic etok --from-file=GOOGLE_CREDENTIALS=[path to service account key]
```

Or, to set credentials for the [AWS provider](https://www.terraform.io/docs/providers/aws/index.html):

```
kubectl create secret generic etok \
  --from-literal=AWS_ACCESS_KEY_ID="youraccesskeyid"  \
  --from-literal=AWS_SECRET_ACCESS_KEY="yoursecretaccesskey"
```

### Workload Identity

https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity

To use Workload Identity for the operator, first ensure you have a GCP service account (GSA). Then bind a policy to the GSA, like so:

```bash
gcloud iam service-accounts add-iam-policy-binding \
    --role roles/iam.workloadIdentityUser \
    --member "serviceAccount:[PROJECT_ID].svc.id.goog[etok/etok]" \
    [GSA_NAME]@[PROJECT_ID].iam.gserviceaccount.com
```

Where `[etok/etok]` refers to the kubernetes service account (KSA) named `etok` in the namespace `etok` (the installation defaults).

Then install the operator with a service account annotation:

```bash
etok install --sa-annotations iam.gke.io/gcp-service-account=[GSA_NAME]@[PROJECT_ID].iam.gserviceaccount.com
```

To use Workload Identity for workspaces, bind a policy to a GSA, as above, but setting the namespace to that of the workspace. The add the annotation to the KSA named `etok` in the namespace of the workspace:

`kubectl annotate serviceaccounts etok iam.gke.io/gcp-service-account=[GSA_NAME]@[PROJECT_ID].iam.gserviceaccount.com`

(`workspace new` creates the KSA if it doesn't already exist)

## Restrictions

Both the terraform configuration and the terraform state, after compression, are subject to a 1MiB limit. This due to the fact that they are stored in a config map and a secret respectively, and the data stored in either cannot exceed 1MiB.

## FAQ

### What is uploaded to the pod when running a plan/apply/destroy?

The contents of the root module (the current working directory, or the value of the `path` flag) is uploaded. Additionally, if the root module configuration contains references to other modules on the local filesystem, then these too are uploaded, along with all such modules recursively referenced (modules referencing modules, and so forth). The directory structure containing all modules is maintained on the kubernetes pod, ensuring relative references remain valid (e.g. `./modules/vpc` or `../modules/vpc`).

Etok supports the use of a [`.terraformignore`](https://www.terraform.io/docs/backends/types/remote.html#excluding-files-from-upload-with-terraformignore) file. Etok expects to find the file in a directory that is an ancestor of the modules to be uploaded. For example, if the modules to be uploaded are in `/tf/modules/prod` and `/tf/modules/vpc`, then the following paths will be checked:

* `/tf/modules/.terraformignore`
* `/tf/.terraformignore`
* `/.terraformignore`

If not found then the default set of rules apply as documented in the link above.

