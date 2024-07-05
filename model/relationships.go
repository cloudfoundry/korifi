package model

type Relationship struct {
	GUID string `json:"guid"`
}

type ToOneRelationship struct {
	//+kubebuilder:validation:Optional
	Data Relationship `json:"data"`
}
