package descriptors

import (
	"fmt"
	"slices"
	"strings"

	"github.com/BooleanCat/go-functional/v2/it"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TableResultSetDescriptor struct {
	Table *metav1.Table
}

func (d *TableResultSetDescriptor) GUIDs() ([]string, error) {
	nameColumnIndex, _, err := d.getColumn("Name")
	if err != nil {
		return nil, err
	}

	return slices.Collect(it.Map(slices.Values(d.Table.Rows), func(row metav1.TableRow) string {
		return row.Cells[nameColumnIndex].(string)
	})), nil
}

func (d *TableResultSetDescriptor) Sort(column string, desc bool) error {
	sortColumnIndex, sortColumn, err := d.getColumn(column)
	if err != nil {
		return err
	}

	rowCompareFunc, err := compareRows(sortColumn.Type, sortColumnIndex, desc)
	if err != nil {
		return fmt.Errorf("failed to get comparison function for column %s: %w", column, err)
	}
	slices.SortStableFunc(d.Table.Rows, rowCompareFunc)

	return nil
}

func (d *TableResultSetDescriptor) getColumn(column string) (int, metav1.TableColumnDefinition, error) {
	for i, columnDef := range d.Table.ColumnDefinitions {
		if columnDef.Name == column {
			return i, columnDef, nil
		}
	}

	return -1, metav1.TableColumnDefinition{}, fmt.Errorf("column %s not found", column)
}

func compareRows(columnType string, columnIndex int, desc bool) (func(a, b metav1.TableRow) int, error) {
	var compareFunc func(a, b metav1.TableRow) int
	switch columnType {
	case "integer":
		compareFunc = func(a, b metav1.TableRow) int {
			return a.Cells[columnIndex].(int) - b.Cells[columnIndex].(int)
		}
	case "string":
		compareFunc = func(a, b metav1.TableRow) int {
			return strings.Compare(a.Cells[columnIndex].(string), b.Cells[columnIndex].(string))
		}
	default:
		return nil, fmt.Errorf("unsupported column type %q for sorting", columnType)
	}

	if desc {
		return func(a, b metav1.TableRow) int {
			return compareFunc(b, a)
		}, nil
	}
	return compareFunc, nil
}
