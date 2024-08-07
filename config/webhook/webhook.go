package main

import (
	"context"
	"net/http"
	"os"
	"time"

	myappsv1 "github.com/Javier090/k8s-pdb-autoscaler/api/v1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	logger logr.Logger
)

func main() {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{})
	if err != nil {
		logger.Error(err, "Unable to create manager")
		os.Exit(1)
	}

	server := mgr.GetWebhookServer()
	server.Register("/validate-eviction", &admission.Webhook{Handler: &EvictionHandler{
		Client: mgr.GetClient(),
	}})

	logger = mgr.GetLogger().WithName("webhook")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logger.Error(err, "Problem running manager")
		os.Exit(1)
	}
}

type EvictionHandler struct {
	Client  client.Client
	decoder *admission.Decoder
}

func (e *EvictionHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	logger.Info("Received eviction request", "namespace", req.Namespace, "name", req.Name)

	// Log eviction request
	evictionLog := myappsv1.EvictionLog{
		PodName:      req.Name,
		EvictionTime: time.Now().Format(time.RFC3339),
	}

	// Fetch the pod to get its labels
	pod := &corev1.Pod{}
	err := e.Client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, pod)
	if err != nil {
		logger.Error(err, "Unable to fetch Pod")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// List all PDBWatchers in the namespace
	pdbWatcherList := &myappsv1.PDBWatcherList{}
	err = e.Client.List(ctx, pdbWatcherList, &client.ListOptions{Namespace: req.Namespace})
	if err != nil {
		logger.Error(err, "Unable to list PDBWatchers")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// Find the applicable PDBWatcher
	var applicablePDBWatcher *myappsv1.PDBWatcher
	for _, pdbWatcher := range pdbWatcherList.Items {
		// Fetch the associated PDB
		pdb := &policyv1.PodDisruptionBudget{}
		err := e.Client.Get(ctx, types.NamespacedName{Name: pdbWatcher.Spec.PDBName, Namespace: pdbWatcher.Namespace}, pdb)
		if err != nil {
			logger.Error(err, "Unable to fetch PDB", "pdbName", pdbWatcher.Spec.PDBName)
			continue
		}

		// Check if the PDB selector matches the evicted pod's labels
		selector, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
		if err != nil {
			logger.Error(err, "Invalid PDB selector", "pdbName", pdbWatcher.Spec.PDBName)
			continue
		}

		if selector.Matches(labels.Set(pod.Labels)) {
			applicablePDBWatcher = &pdbWatcher
			break
		}
	}

	if applicablePDBWatcher == nil {
		logger.Info("No applicable PDBWatcher found")
		return admission.Allowed("no applicable PDBWatcher")
	}

	// Update the PDBWatcher status with the new eviction log
	// Ensure the eviction log does not grow indefinitely
	maxLogs := 100
	if len(applicablePDBWatcher.Status.EvictionLogs) >= maxLogs {
		applicablePDBWatcher.Status.EvictionLogs = applicablePDBWatcher.Status.EvictionLogs[1:]
	}
	applicablePDBWatcher.Status.EvictionLogs = append(applicablePDBWatcher.Status.EvictionLogs, evictionLog)

	err = e.Client.Status().Update(ctx, applicablePDBWatcher)
	if err != nil {
		logger.Error(err, "Unable to update PDBWatcher status")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	logger.Info("Eviction logged successfully", "podName", req.Name, "evictionTime", evictionLog.EvictionTime)
	return admission.Allowed("eviction allowed")
}

func (e *EvictionHandler) InjectDecoder(d *admission.Decoder) error {
	e.decoder = d
	return nil
}
