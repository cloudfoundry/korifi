package roles

//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfapps,verbs=get;list
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfprocesses,verbs=get;list
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfpackages,verbs=get;list
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfbuilds,verbs=get;list

//+kubebuilder:rbac:groups=services.cloudfoundry.org,resources=cfserviceinstances,verbs=get;list
//+kubebuilder:rbac:groups=services.cloudfoundry.org,resources=cfservicebindings,verbs=get;list

//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfroutes,verbs=get;list
