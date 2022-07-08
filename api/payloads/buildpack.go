package payloads

type BuildpackList struct {
	OrderBy string `schema:"order_by"`
}

func (d *BuildpackList) SupportedKeys() []string {
	return []string{"order_by"}
}
