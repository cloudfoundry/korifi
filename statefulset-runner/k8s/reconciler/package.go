package reconciler

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

const (
	IndexEventInvolvedObjectName = ".index.eventOwner.name"
	IndexEventInvolvedObjectKind = ".index.eventOwner.kind"
	IndexEventReason             = ".index.reason"
)
