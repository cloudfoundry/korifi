package stset

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

const (
	AppSourceType = "APP"

	AnnotationAppName              = "korifi.cloudfoundry.org/application-name"
	AnnotationVersion              = "korifi.cloudfoundry.org/version"
	AnnotationAppID                = "korifi.cloudfoundry.org/application-id"
	AnnotationSpaceName            = "korifi.cloudfoundry.org/space-name"
	AnnotationOrgName              = "korifi.cloudfoundry.org/org-name"
	AnnotationOrgGUID              = "korifi.cloudfoundry.org/org-guid"
	AnnotationSpaceGUID            = "korifi.cloudfoundry.org/space-guid"
	AnnotationProcessGUID          = "korifi.cloudfoundry.org/process-guid"
	AnnotationLastReportedAppCrash = "korifi.cloudfoundry.org/last-reported-app-crash"
	AnnotationLastReportedLRPCrash = "korifi.cloudfoundry.org/last-reported-lrp-crash"

	LabelGUID        = "korifi.cloudfoundry.org/guid"
	LabelOrgGUID     = AnnotationOrgGUID
	LabelOrgName     = AnnotationOrgName
	LabelSpaceGUID   = AnnotationSpaceGUID
	LabelSpaceName   = AnnotationSpaceName
	LabelVersion     = "korifi.cloudfoundry.org/version"
	LabelAppGUID     = "korifi.cloudfoundry.org/app-guid"
	LabelProcessType = "korifi.cloudfoundry.org/process-type"
	LabelSourceType  = "korifi.cloudfoundry.org/source-type"

	ApplicationContainerName = "opi"

	PdbMinAvailableInstances          = 1
	PrivateRegistrySecretGenerateName = "private-registry-"
)
