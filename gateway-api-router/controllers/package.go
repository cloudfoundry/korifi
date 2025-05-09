package controllers

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cforgs,verbs=get;list;watch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfspaces,verbs=get;list;watch

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfroutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfroutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfroutes/finalizers,verbs=update

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get

//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

const (
	GatewayApiRouterReconcilerName = "gateway-api-router"
)
