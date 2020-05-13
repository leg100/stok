// +build ignore

package main

import (
	"fmt"

	"github.com/iancoleman/strcase"
	"github.com/leg100/stok/crdinfo"
	"github.com/leg100/stok/util"
)

func main() {
	for k, v := range crdinfo.Inventory {
		util.GenerateTemplate(v, cobraCommand, fmt.Sprintf("command_%s.go", strcase.ToSnake(k)))
	}
}

var cobraCommand = `// Code generated by go generate; DO NOT EDIT.
package cmd

import (
	"os"

	"github.com/leg100/stok/pkg/apis/stok/v1alpha1"
	"github.com/spf13/cobra"
)

var cmd{{ .Name | ToCamel }} = &cobra.Command{
	Use:   "{{ .Name }} [flags] -- [{{ .Name }} args]",
	Short: "Run terraform {{ .Name }}",
	Run: func(cmd *cobra.Command, args []string) {
		runApp(&v1alpha1.{{ .Kind | ToCamel }}{}, "{{ .Name }}", {{ .ArgsHandler }}(os.Args))
	},
}

func init() {
	rootCmd.AddCommand(cmd{{ .Name | ToCamel }})
}
`