package command

import (
	"bytes"
	"strings"
	"text/template"
)

type Script struct {
	Resource   string
	Tarball    string
	Kind       string
	Entrypoint []string
	Args       []string
}

func (s Script) generate() (string, error) {
	script := `#Extract workspace tarball
tar zxf /tarball/{{ .Tarball }}

# wait for both the client to be ready and
# for the command to be front of the workspace queue
kubectl wait --for=condition=WorkspaceReady --timeout=-1s {{ .Kind }}/{{ .Resource }} > /dev/null
kubectl wait --for=condition=ClientReady --timeout=-1s {{ .Kind }}/{{ .Resource }} > /dev/null

# run stok command
{{ join .Entrypoint " " }}{{ if gt (len .Args) 0 }} {{ join .Args " " }}{{ end }}

`

	tmpl := template.New("script")
	tmpl = tmpl.Funcs(template.FuncMap{"join": strings.Join})
	tmpl, err := tmpl.Parse(script)
	if err != nil {
		return "", err
	}

	buf := new(bytes.Buffer)
	err = tmpl.Execute(buf, s)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}