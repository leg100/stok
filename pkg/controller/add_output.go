// Code generated by go generate; DO NOT EDIT.
package controller

import (
		outputController "github.com/leg100/stok/pkg/controller/output"
)

func init() {
        // AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
        AddToManagerFuncs = append(AddToManagerFuncs, outputController.Add)
}
