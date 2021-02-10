package catalog2

import (
	v1 "k8s.io/api/core/v1"
)

// DataModel is used to keep relations between objects
type DataModel struct {
	namespaces map[string]Namespace
}

// NewDataModel returns a new DataModel instance
func NewDataModel() *DataModel {
	return &DataModel{
		namespaces: make(map[string]Namespace),
	}
}

// Namespace wraps a K8s Namespace object pointer.
// Will allow for graphing interfaces later on.
type Namespace struct {
	namespace *v1.Namespace
}
