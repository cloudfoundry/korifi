package services

import (
	jellidation "github.com/jellydator/validation"
)

// +kubebuilder:object:generate=true
type ServiceBroker struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type BrokerCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (c BrokerCredentials) Validate() error {
	return jellidation.ValidateStruct(&c,
		jellidation.Field(&c.Username, jellidation.Required),
		jellidation.Field(&c.Password, jellidation.Required),
	)
}
