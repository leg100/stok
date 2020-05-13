// Code generated by go generate; DO NOT EDIT.
package cmd

import (
	"os"

	"github.com/leg100/stok/pkg/apis/stok/v1alpha1"
	"github.com/spf13/cobra"
)

var cmdPlan = &cobra.Command{
	Use:   "plan [flags] -- [plan args]",
	Short: "Run terraform plan",
	Run: func(cmd *cobra.Command, args []string) {
		runApp(&v1alpha1.Plan{}, "plan", DoubleDashArgsHandler(os.Args))
	},
}

func init() {
	rootCmd.AddCommand(cmdPlan)
}