/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	managedsecretv1alpha1 "github.com/lib250/managed-secret-operator.git/api/v1alpha1"
)

// ManagedSecretReconciler reconciles a ManagedSecret object
type ManagedSecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=managed-secret.lib250.domain,resources=managedsecrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=managed-secret.lib250.domain,resources=managedsecrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=managed-secret.lib250.domain,resources=managedsecrets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ManagedSecret object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *ManagedSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here
	log.Info("Entering ManagedSecret reconcile", "req", req)

	const managedSecretFinalizer = "managedsecret.lib250.domain/finalizer"

	var managedSecret v1alpha1.ManagedSecret

	if err := r.Get(ctx, req.NamespacedName, &managedSecret); err != nil {
		log.Error(err, "could not fetch ManagedSecret")
		return ctrl.Result{}, ignoreNotFound(err)
	}


	if managedSecret.ObjectMeta.DeletionTimestamp.IsZero() {
		if !containsString(managedSecret.ObjectMeta.Finalizers, managedSecretFinalizer) {
			managedSecret.ObjectMeta.Finalizers = append(managedSecret.ObjectMeta.Finalizers, managedSecretFinalizer)
			if err := r.Update(ctx, &managedSecret); err != nil {
				log.Error(err, "Failed to update finalizzer", "Namespace", namespace, "Name", req.Name)
				return ctrl.Result{}, err
			}
		}
	} else {
		if containsString(managedSecret.ObjectMeta.Finalizers, managedSecretFinalizer) {

			if err := r.cleanupSecrets(ctx, managedSecret); err != nil {
				log.Error(err, "Failed to clean up", "Namespace", namespace, "Name", req.Name)
				return ctrl.Result{}, err
			}

			managedSecret.ObjectMeta.Finalizers = removeString(managedSecret.ObjectMeta.Finalizers, managedSecretFinalizer)
			if err := r.Update(ctx, &managedSecret); err != nil {
				log.Error(err, "Failed to remove finalizer", "Namespace", namespace, "Name", req.Name)
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, err
	}


	for _, namespace := range managedSecret.Spec.Namespaces {
		secret := &corev1.Secret{}
		secretName := types.NamespacedName{
			Namespace: namespace,
			Name: req.Name
		}

		err := r.Get(ctx, secretName, secret)
		if err != nil && errors.IsNotFound(err) {
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: req.Name,
					Namespace: namespace
				},
				Data: managedSecret.Spec.Secret
			}
			if err := r.Create(ctx, secret); err != nil {
				log.Error(err, "Failed to create Secret", "Namespace", namespace, "Name", req.Name)
				continue
			}
			log.Info("Created Secret", "Namespace", namespace, "Name", req.Name)
		} else if err == nil {
			secret.Data = managedSecret.Spec.Secret
			if err := r.Status().Update(ctx, secret); err != nil {
				log.Error(err, "Failed to update Secret", "Namespace", namespace, "Name", req.Name)
				continue
			}
			log.Info("Updated Secret", "Namespace", namespace, "Name", req.Name)
		} else {
			log.Error(err, "Failed to get Secret", "Namespace", namespace, "Name", req.Name)
			continue
		}
	}

	return ctrl.Result{}, nil
}

func (r *ManagedSecretReconciler) cleanupSecrets(ctx context.Context, managedSecret v1alpha1.ManagedSecret) error {
    
    for _, namespace := range managedSecret.Spec.Namespaces {
        
        var secretList corev1.SecretList
        listOpts := []client.ListOption{
            client.InNamespace(namespace),
            client.MatchingLabels{"managed-by": "managedsecret-operator", "managedsecret-name": managedSecret.Name},
        }

        if err := r.List(ctx, &secretList, listOpts...); err != nil {
            log.Error(err, "Failed to List Secrets during cleanup", "Namespace", namespace)
            return err
        }

        for _, secret := range secretList.Items {
            if err := r.Delete(ctx, &secret); err != nil {
				log.Error(err, "Error deleting during cleanup", "Namespace", namespace)
            }
        }
    }
    return nil
}

func containsString(slice []string, s string) bool {
    for _, item := range slice {
        if item == s {
            return true
        }
    }
    return false
}

func removeString(slice []string, s string) []string {
    result := []string{}
    for _, item := range slice {
        if item != s {
            result = append(result, item)
        }
    }
    return result
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&managedsecretv1alpha1.ManagedSecret{}).
		Complete(r)
}

