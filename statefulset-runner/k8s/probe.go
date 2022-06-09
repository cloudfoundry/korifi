package k8s

import (
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	livenessFailureThreshold  = 4
	readinessFailureThreshold = 1
)

func CreateLivenessProbe(lrp *eiriniv1.LRP) *v1.Probe {
	initialDelay := toSeconds(lrp.Spec.Health.TimeoutMs)

	if lrp.Spec.Health.Type == "http" {
		return createHTTPProbe(lrp, initialDelay, livenessFailureThreshold)
	}

	if lrp.Spec.Health.Type == "port" {
		return createPortProbe(lrp, initialDelay, livenessFailureThreshold)
	}

	return nil
}

func CreateReadinessProbe(lrp *eiriniv1.LRP) *v1.Probe {
	if lrp.Spec.Health.Type == "http" {
		return createHTTPProbe(lrp, 0, readinessFailureThreshold)
	}

	if lrp.Spec.Health.Type == "port" {
		return createPortProbe(lrp, 0, readinessFailureThreshold)
	}

	return nil
}

func createPortProbe(lrp *eiriniv1.LRP, initialDelay, failureThreshold int32) *v1.Probe {
	return &v1.Probe{
		ProbeHandler: v1.ProbeHandler{
			TCPSocket: tcpSocketAction(lrp),
		},
		InitialDelaySeconds: initialDelay,
		FailureThreshold:    failureThreshold,
	}
}

func createHTTPProbe(lrp *eiriniv1.LRP, initialDelay, failureThreshold int32) *v1.Probe {
	return &v1.Probe{
		ProbeHandler: v1.ProbeHandler{
			HTTPGet: httpGetAction(lrp),
		},
		InitialDelaySeconds: initialDelay,
		FailureThreshold:    failureThreshold,
	}
}

func httpGetAction(lrp *eiriniv1.LRP) *v1.HTTPGetAction {
	return &v1.HTTPGetAction{
		Path: lrp.Spec.Health.Endpoint,
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: lrp.Spec.Health.Port},
	}
}

func tcpSocketAction(lrp *eiriniv1.LRP) *v1.TCPSocketAction {
	return &v1.TCPSocketAction{
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: lrp.Spec.Health.Port},
	}
}

func toSeconds(millis uint) int32 {
	return int32(millis / 1000) //nolint:gomnd
}
