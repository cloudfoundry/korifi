package controllers

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfroutes,verbs=get;list;watch;create;update;delete;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfroutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfroutes/finalizers,verbs=update
