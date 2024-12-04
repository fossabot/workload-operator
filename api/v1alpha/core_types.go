package v1alpha

// References a secret in the same namespace as the entity defining the
// reference.
type LocalSecretReference struct {
	// The name of the secret
	//
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}
