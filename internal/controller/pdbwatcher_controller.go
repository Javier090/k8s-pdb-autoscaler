package controllers

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	myappsv1 "github.com/Javier090/k8s-pdb-autoscaler/api/v1"
)

// PDBWatcherReconciler reconciles a PDBWatcher object
type PDBWatcherReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps.mydomain.com,resources=pdbwatchers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.mydomain.com,resources=pdbwatchers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.mydomain.com,resources=pdbwatchers/finalizers,verbs=update

// this will only get called when the pdb watcher updates. Watching deployment (and PDB) still a todo?
func (r *PDBWatcherReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the PDBWatcher instance
	pdbWatcher := &myappsv1.PDBWatcher{}
	err := r.Get(ctx, req.NamespacedName, pdbWatcher)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the PDB
	// FOr now you (the developer) adn manually setting the pdb on the spec right? (As opposed to automatically making oe of these for every pdb)
	pdb := &policyv1.PodDisruptionBudget{}
	err = r.Get(ctx, types.NamespacedName{Name: pdbWatcher.Spec.PDBName, Namespace: pdbWatcher.Namespace}, pdb)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Check if PDB overlaps with multiple deployments
	selector, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
	if err != nil {
		return ctrl.Result{}, err
	}

	podList := &corev1.PodList{}
	err = r.List(ctx, podList, &client.ListOptions{Namespace: pdbWatcher.Namespace, LabelSelector: selector})
	if err != nil {
		return ctrl.Result{}, err
	}

	deploymentMap := make(map[string]struct{})
	for _, pod := range podList.Items {
		for _, ownerRef := range pod.OwnerReferences {
			if ownerRef.Kind == "ReplicaSet" {
				replicaSet := &appsv1.ReplicaSet{}
				err = r.Get(ctx, types.NamespacedName{Name: ownerRef.Name, Namespace: pdbWatcher.Namespace}, replicaSet)
				if err != nil {
					return ctrl.Result{}, err
				}

				// Get the Deployment that owns this ReplicaSet
				for _, rsOwnerRef := range replicaSet.OwnerReferences {
					if rsOwnerRef.Kind == "Deployment" {
						deploymentMap[rsOwnerRef.Name] = struct{}{}
					}
				}
			}
		}
	}

	if len(deploymentMap) > 1 {
		//ideally you want to event on your watcher here so user can see it.
		//should we also event if there are no pods owned by deployments?
		return ctrl.Result{}, fmt.Errorf("PDB %s/%s overlaps with multiple deployments", pdbWatcher.Namespace, pdbWatcher.Spec.PDBName)
	}

	// Fetch the Deployment
	// see comments in crd whats the intention of putting deployment in spec. if its optional it could override what you find in
	// deploymentMap.
	deploymentName := pdbWatcher.Spec.DeploymentName
	if len(deploymentMap) == 1 {
		for name := range deploymentMap {
			deploymentName = name
		}
	}

	deployment := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: pdbWatcher.Namespace}, deployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Check the DisruptionsAllowed field
	if pdb.Status.DisruptionsAllowed == 0 {
		// Scale up the Deployment
		newReplicas := *deployment.Spec.Replicas + pdbWatcher.Spec.ScaleFactor
		// ideally want to use deployments max surge here rathr than duplicaing into pdb watcher
		if newReplicas > pdbWatcher.Spec.MaxReplicas {
			newReplicas = pdbWatcher.Spec.MaxReplicas
		}
		// I don't think you want to do this unless theres recent evictions. Otherwise anyone touching your pdb watcher resource will scale it up.
		deployment.Spec.Replicas = &newReplicas
		err = r.Update(ctx, deployment)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Log the scaling action
		logger.Info(fmt.Sprintf("Scaled up Deployment %s/%s to %d replicas", deployment.Namespace, deployment.Name, newReplicas))
	}

	// Process Eviction Logs
	if len(pdbWatcher.Status.EvictionLogs) > 0 {
		for _, log := range pdbWatcher.Status.EvictionLogs {
			evictionTime, err := time.Parse(time.RFC3339, log.EvictionTime)
			if err != nil {
				logger.Error(err, "Failed to parse eviction time")
				continue
			}

			// Check if the eviction was recent (within the last 5 minutes)
			if time.Since(evictionTime) < 5*time.Minute { //make this configurable?
				logger.Info(fmt.Sprintf("Recent eviction for Pod %s at %s", log.PodName, log.EvictionTime))
			}
		}

		// Clear eviction logs after processing
		pdbWatcher.Status.EvictionLogs = []myappsv1.EvictionLog{}
		err = r.Status().Update(ctx, pdbWatcher)
		if err != nil {
			logger.Error(err, "Failed to update PDBWatcher status")
			return ctrl.Result{}, err
		}
	}
	// What will return us to orginal. state. Assuming we need a watch on pdb for it to come off

	return ctrl.Result{}, nil
}

func (r *PDBWatcherReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&myappsv1.PDBWatcher{}).
		Complete(r)
}
