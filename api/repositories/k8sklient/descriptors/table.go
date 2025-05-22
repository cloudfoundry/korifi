package descriptors

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TableResultSetDescriptor struct {
	rawTable *metav1.Table
}

func NewTableResultSetDescriptor(rawTable *metav1.Table) *TableResultSetDescriptor {
	return &TableResultSetDescriptor{
		rawTable: rawTable,
	}
}

func (t *TableResultSetDescriptor) SortedGUIDs(column string, desc bool) ([]string, error) {
	return nil, nil
}
