package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RouterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Router `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Router struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              RouterSpec   `json:"spec"`
	Status            RouterStatus `json:"status,omitempty"`
}

type RouterSpec struct {
	Size int32 `json:"size,omitempty"`
	Console bool `json:"console,omitempty"`
	Addresses []Address `json:"addresses,omitempty"`
	AutoLinks []Address `json:"autoLinks,omitempty"`
	LinkRoutes []LinkRoute `json:"linkRoutes,omitempty"`
	Connectors []Connector `json:"connectors,omitempty"`
	InterRouterConnectors []Connector `json:"interRouterConnectors,omitempty"`
	Listeners []Listener `json:"listeners,omitempty"`
	InterRouterListeners []Listener `json:"interRouterListeners,omitempty"`
	SslProfiles []SslProfile `json:"sslProfiles,omitempty"`
}

type RouterStatus struct {
	Nodes []string `json:"nodes"`
}

type Address struct {
	Prefix string `json:"prefix,omitempty"`
	Pattern string `json:"pattern,omitempty"`
	Distribution string `json:"distribution,omitempty"`
	Waypoint bool `json:"waypoint,omitempty"`
	IngressPhase *int32 `json:"ingressPhase,omitempty"`
	EgressPhase *int32 `json:"ingressPhase,omitempty"`
}

type LinkRoute struct {
	Prefix string `json:"prefix,omitempty"`
	Pattern string `json:"pattern,omitempty"`
	Direction string `json:"direction,omitempty"`
	ContainerId string `json:"containerId,omitempty"`
	Connection string `json:"connection,omitempty"`
	AddExternalPrefix string `json:"addExternalPrefix,omitempty"`
	RemoveExternalPrefix string `json:"removeExternalPrefix,omitempty"`
}

type AutoLink struct {
	Address string `json:"address"`
	Direction string `json:"direction"`
	ContainerId string `json:"containerId,omitempty"`
	Connection string `json:"connection,omitempty"`
	ExternalPrefix string `json:"externalPrefix,omitempty"`
	Phase *int32 `json:"phase,omitempty"`
}

type Connector struct {
	Name string `json:"name,omitempty"`
	Host string `json:"host"`
	Port int32 `json:"port"`
	RouteContainer bool `json:"role,omitempty"`
	Cost int32 `json:"cost,omitempty"`
	SslProfile string `json:"sslProfile,omitempty"`
}

type Listener struct {
	Name string `json:"name,omitempty"`
	Host string `json:"host,omitempty"`
	Port int32 `json:"port"`
	RouteContainer bool `json:"role,omitempty"`
	Http bool `json:"http,omitempty"`
	Cost int32 `json:"cost,omitempty"`
	SslProfile string `json:"sslProfile,omitempty"`
}

type SslProfile struct {
	Name string `json:"name,omitempty"`
	Credentials string `json:"credentials,omitempty"`
	CaCert string `json:"caCert,omitempty"`
	RequireClientCerts bool `json:"requireClientCerts,omitempty"`
	Ciphers string `json:"ciphers,omitempty"`
	Protocols string `json:"protocols,omitempty"`
}
