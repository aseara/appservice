/*
Copyright 2022 aseara.
*/

package controllers

import (
	"context"
	"encoding/json"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appv1 "github.com/aseara/appservice/api/v1"
	"github.com/aseara/appservice/resource"
)

// AppServiceReconciler reconciles a AppService object
type AppServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=aseara.github.com.aseara.github.com,resources=appservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=aseara.github.com.aseara.github.com,resources=appservices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=aseara.github.com.aseara.github.com,resources=appservices/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods;services;endpoints;persistentvolumeclaims;events;configmaps;secrets,verbs=*
//+kubebuilder:rbac:groups="",resources=namespace,verbs=get
//+kubebuilder:rbac:groups=apps,resources=deployments;daemonsets;replicasets;statefulsets,verbs=*
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AppService object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *AppServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (rt ctrl.Result, err error) {
	l := log.FromContext(ctx, "Request.Namespace", req.Namespace, "Request.Name", req.Name)
	l.Info("Reconciling AppService")

	// Fetch the AppService instance
	instance := &appv1.AppService{}
	err = r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			err = nil
		}
		// Error reading the object - requeue the request.
		return
	}

	if instance.DeletionTimestamp != nil {
		return
	}

	var f bool = true
	defer func() {
		if f && err == nil {
			err = r.refreshSpec(ctx, instance)
		}
	}()

	deploy := &appsv1.Deployment{}
	if err = r.Client.Get(ctx, req.NamespacedName, deploy); err != nil {
		if errors.IsNotFound(err) {
			err = r.createResource(ctx, instance, req.NamespacedName)
		}
		return
	}

	oldspec := &appv1.AppServiceSpec{}
	if err = json.Unmarshal([]byte(instance.Annotations["spec"]), oldspec); err != nil {
		return
	}

	// spec 没有变化，无需更新
	if reflect.DeepEqual(instance.Spec, oldspec) {
		f = false
		return
	}

	err = r.updateResource(ctx, instance, req.NamespacedName)
	return
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appv1.AppService{}).
		Complete(r)
}

// createResource 创建关联资源
func (r *AppServiceReconciler) createResource(ctx context.Context, instance *appv1.AppService, key client.ObjectKey) error {
	// 1. 创建 Deploy
	deploy := resource.NewDeploy(instance)
	if err := r.Client.Create(ctx, deploy); err != nil {
		return err
	}
	// 2. 创建 Service
	service := resource.NewService(instance)
	return r.Client.Create(ctx, service)
}

// updateResource 更新关联资源
func (r *AppServiceReconciler) updateResource(ctx context.Context, instance *appv1.AppService, key client.ObjectKey) error {
	newDeploy := resource.NewDeploy(instance)
	oldDeploy := &appsv1.Deployment{}
	if err := r.Client.Get(ctx, key, oldDeploy); err != nil {
		return err
	}
	oldDeploy.Spec = newDeploy.Spec
	if err := r.Client.Update(ctx, oldDeploy); err != nil {
		return err
	}
	newService := resource.NewService(instance)
	oldService := &corev1.Service{}
	if err := r.Client.Get(ctx, key, oldService); err != nil {
		return err
	}
	oldService.Spec = newService.Spec
	return r.Client.Update(ctx, oldService)
}

// refreshSpec 更新AppService Annotation spec
func (r *AppServiceReconciler) refreshSpec(ctx context.Context, instance *appv1.AppService) error {
	// 关联 Annotations
	data, _ := json.Marshal(instance.Spec)
	if instance.Annotations != nil {
		instance.Annotations["spec"] = string(data)
	} else {
		instance.Annotations = map[string]string{"spec": string(data)}
	}
	return r.Client.Update(ctx, instance)
}
