package services

// +kubebuilder:object:generate=true
type ServiceBroker struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type BrokerCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
