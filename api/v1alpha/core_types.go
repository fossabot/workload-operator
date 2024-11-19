package v1alpha

// References a secret in the same namespace as the entity defining the
// reference.
type LocalSecretReference struct {
	// The name of the secret
	//
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

type ClusterProfileReference struct {
	// Name of a cluster profile
	//
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace for the cluster profile
	//
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`
}
