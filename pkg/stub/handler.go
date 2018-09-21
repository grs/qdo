package stub

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strconv"
	"text/template"

	"github.com/grs/qdo/pkg/apis/grs/v1alpha1"

        "github.com/operator-framework/operator-sdk/pkg/sdk"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const certRequestAnnotation = "service.alpha.openshift.io/serving-cert-secret-name"

func NewHandler() sdk.Handler {
	return &Handler{}
}

type Handler struct {
}

func (h *Handler) Handle(ctx context.Context, event sdk.Event) error {
	switch o := event.Object.(type) {
	case *v1alpha1.Router:
		router := o

		// Ignore the delete event since the garbage collector will clean up all secondary resources for the CR
		// All secondary resources must have the CR set as their OwnerReference for this to be the case
		if event.Deleted {
			return nil
		}

		requestCert := setRouterDefaults(router)

		// Create the deployment if it doesn't exist
		config := configForRouter(router)
		container := containerForRouter(router, config)
		dep := deploymentForRouter(router)
		err := sdk.Create(dep)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create deployment: %v", err)
		}

		err = sdk.Get(dep)
		if err != nil {
			return fmt.Errorf("failed to get deployment: %v", err)
		}
		update := false
		// Ensure the deployment size is the same as the spec
		size := router.Spec.Size
		if size != 0 && *dep.Spec.Replicas != size {
			*dep.Spec.Replicas = size
			update = true
		}
		// Ensure the containers in the deployment matches the router spec
		if len(dep.Spec.Template.Spec.Containers) != 1 || !checkContainer(&container, &dep.Spec.Template.Spec.Containers[0]) {
			dep.Spec.Template.Spec.Containers = []v1.Container{container}
			update = true
		}
		if update {
			err = sdk.Update(dep)
			if err != nil {
				return fmt.Errorf("failed to update deployment: %v", err)
			}
		}

		service := serviceForRouter(router, requestCert)
		err = sdk.Create(service)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create service: %v", err)
		}
		actual := service.DeepCopy()
		err = sdk.Get(actual)
		if err != nil {
			return fmt.Errorf("failed to get service: %v", err)
		}
		if checkService(service, actual) {
			err = sdk.Update(actual)
			if err != nil {
				return fmt.Errorf("failed to update service: %v", err)
			}
		}

		// Update the Router status with the pod names
		podList := podList()
		labelSelector := labels.SelectorFromSet(labelsForRouter(router.Name)).String()
		listOps := &metav1.ListOptions{LabelSelector: labelSelector}
		err = sdk.List(router.Namespace, podList, sdk.WithListOptions(listOps))
		if err != nil {
			return fmt.Errorf("failed to list pods: %v", err)
		}
		podNames := getPodNames(podList.Items)
		if !reflect.DeepEqual(podNames, router.Status.Nodes) {
			router.Status.Nodes = podNames
			err := sdk.Update(router)
			if err != nil {
				return fmt.Errorf("failed to update router status: %v", err)
			}
		}
	}
	return nil
}

func isDefaultSslProfileDefined(m *v1alpha1.Router) bool {
	for _, profile := range m.Spec.SslProfiles {
		if profile.Name == "default" {
			return true
		}
	}
	return false
}

func isDefaultSslProfileUsed(m *v1alpha1.Router) bool {
	for _, listener := range m.Spec.Listeners {
		if listener.SslProfile == "default" {
			return true
		}
	}
	for _, listener := range m.Spec.InterRouterListeners {
		if listener.SslProfile == "default" {
			return true
		}
	}
	return false
}

func setRouterDefaults(m *v1alpha1.Router) bool {
	requestCert := false
	if len(m.Spec.Listeners) == 0 {
		m.Spec.Listeners = append(m.Spec.Listeners, v1alpha1.Listener{
			Port: 5672,
		}, v1alpha1.Listener{
			Port: 5671,
			SslProfile: "default",
		}, v1alpha1.Listener{
			Port: 8672,
			Http: true,
			SslProfile: "default",
		})
	}
	if len(m.Spec.InterRouterListeners) == 0 {
		m.Spec.InterRouterListeners = append(m.Spec.InterRouterListeners, v1alpha1.Listener{
			Port: 55672,
		})
	}
	if !isDefaultSslProfileDefined(m) && isDefaultSslProfileUsed(m) {
		m.Spec.SslProfiles = append(m.Spec.SslProfiles, v1alpha1.SslProfile{
			Name: "default",
			Credentials: m.Name + "-cert",
		})
		requestCert = true
	}
	for _, profile := range m.Spec.SslProfiles {
		if profile.Credentials == "" {
			profile.Credentials = m.Name + "-cert"
			requestCert = true
		}
	}
	return requestCert
}


func configForRouter(m *v1alpha1.Router) string {
	config := `
    router {
        mode: interior
        id: ${HOSTNAME}
    }

    {{range .Listeners}}
    listener {
        {{- if .Name}}
        name: {{.Name}}
        {{- end}}
        {{- if .Host}}
        host: {{.Host}}
        {{- else}}
        host: 0.0.0.0
        {{- end}}
        {{- if .Port}}
        port: {{.Port}}
        {{- end}}
        {{- if .RouteContainer}}
        role: route-container
        {{- else}}
        role: normal
        {{- end}}
        {{- if .Http}}
        http: true
        httpRootDir: /usr/share/qpid-dispatch/console
        {{- end}}
        {{- if .SslProfile}}
        sslProfile: {{.SslProfile}}
        {{- end}}
    }
    {{- end}}

    {{range .InterRouterListeners}}
    listener {
        {{- if .Name}}
        name: {{.Name}}
        {{- end}}
        role: inter-router
        {{- if .Host}}
        host: {{.Host}}
        {{- else}}
        host: 0.0.0.0
        {{- end}}
        {{- if .Port}}
        port: {{.Port}}
        {{- end}}
        {{- if .Cost}}
        cost: {{.Cost}}
        {{- end}}
        {{- if .SslProfile}}
        sslProfile: {{.SslProfile}}
        {{- end}}
    }
    {{- end}}

    {{range .SslProfiles}}
    sslProfile {
       name: {{.Name}}
       {{- if .Credentials}}
       certFile: /etc/qpid-dispatch-certs/{{.Name}}/{{.Credentials}}/tls.crt
       privateKeyFile: /etc/qpid-dispatch-certs/{{.Name}}/{{.Credentials}}/tls.key
       {{- end}}
       {{- if .CaCert}}
       caCertFile: /etc/qpid-dispatch-certs/{{.Name}}/{{.CaCert}}/ca.crt
       {{- else if .RequireClientCerts}}
       caCertFile: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
       {{- end}}
    }
    {{- end}}

    {{range .Addresses}}
    address {
        {{- if .Prefix}}
        prefix: {{.Prefix}}
        {{- end}}
        {{- if .Pattern}}
        pattern: {{.Pattern}}
        {{- end}}
        {{- if .Distribution}}
        distribution: {{.Distribution}}
        {{- end}}
        {{- if .Waypoint}}
        waypoint: {{.Waypoint}}
        {{- end}}
        {{- if .IngressPhase}}
        ingressPhase: {{.IngressPhase}}
        {{- end}}
        {{- if .EgressPhase}}
        egressPhase: {{.EgressPhase}}
        {{- end}}
    }
    {{- end}}

    {{range .LinkRoutes}}
    linkRoute {
        {{- if .Prefix}}
        prefix: {{.Prefix}}
        {{- end}}
        {{- if .Pattern}}
        pattern: {{.Pattern}}
        {{- end}}
        {{- if .Direction}}
        direction: {{.Direction}}
        {{- end}}
        {{- if .Connection}}
        connection: {{.Connection}}
        {{- end}}
        {{- if .ContainerId}}
        containerId: {{.ContainerId}}
        {{- end}}
        {{- if .AddExternalPrefix}}
        addExternalPrefix: {{.AddExternalPrefix}}
        {{- end}}
        {{- if .RemoveExternalPrefix}}
        removeExternalPrefix: {{.RemoveExternalPrefix}}
        {{- end}}
    }
    {{- end}}

    {{range .AutoLinks}}
    autoLink {
        {{- if .Address}}
        addr: {{.Address}}
        {{- end}}
        {{- if .Direction}}
        direction: {{.Direction}}
        {{- end}}
        {{- if .ContainerId}}
        containerId: {{.ContainerId}}
        {{- end}}
        {{- if .Connection}}
        connection: {{.Connection}}
        {{- end}}
        {{- if .ExternalPrefix}}
        externalPrefix: {{.ExternalPrefix}}
        {{- end}}
        {{- if .Phase}}
        Phase: {{.Phase}}
        {{- end}}
    }
    {{- end}}

    {{range .Connectors}}
    connector {
        {{- if .Name}}
        name: {{.Name}}
        {{- end}}
        {{- if .Host}}
        host: {{.Host}}
        {{- end}}
        {{- if .Port}}
        port: {{.Port}}
        {{- end}}
        {{- if .Role}}
        role: {{.Role}}
        {{- end}}
        {{- if .Cost}}
        cost: {{.Cost}}
        {{- end}}
    }
    {{- end}}`
	var buff bytes.Buffer
	routerconfig := template.Must(template.New("routerconfig").Parse(config))
	routerconfig.Execute(&buff, m.Spec)
	return buff.String()
}

func checkContainer(desired *v1.Container, actual *v1.Container) bool {
	//TODO: check image
	if !reflect.DeepEqual(desired.Env, actual.Env) {
		return false
	}
	if !reflect.DeepEqual(desired.Ports, actual.Ports) {
		return false
	}
	if !reflect.DeepEqual(desired.VolumeMounts, actual.VolumeMounts) {
		return false
	}
	return true
}

func containerForRouter(m *v1alpha1.Router, config string) v1.Container {
	container := v1.Container {
		//TODO: allow alternate image to be specified
		Image:   "amq-interconnect/amq-interconnect-1.2-openshift:latest",
		Name:    "router",
		Env: []v1.EnvVar{
			{
				Name: "QDROUTERD_CONF",
				Value: config,
			},
			{
				//TODO: allow auto-mesh strategy to be configured
				Name: "QDROUTERD_AUTO_MESH_DISCOVERY",
				Value: "QUERY",
			},
			{
				Name: "APPLICATION_NAME",
				Value: m.Name,
			},
			{
				Name: "POD_NAMESPACE",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
			{
				Name: "POD_IP",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "status.podIP",
					},
				},
			},
		},
		Ports: containerPortsForRouter(m),
	}
	if m.Spec.SslProfiles != nil && len(m.Spec.SslProfiles)  > 0 {
		volumeMounts := []v1.VolumeMount{}
		for _, profile := range m.Spec.SslProfiles {
			if len(profile.Credentials) > 0 {
				volumeMounts = append(volumeMounts, v1.VolumeMount{
					Name: profile.Credentials,
					MountPath: "/etc/qpid-dispatch-certs/" + profile.Name + "/" + profile.Credentials,
				})
			}
			if len(profile.CaCert) > 0 && profile.CaCert != profile.Credentials {
				volumeMounts = append(volumeMounts, v1.VolumeMount{
					Name: profile.CaCert,
					MountPath: "/etc/qpid-dispatch-certs/" + profile.Name + "/" + profile.CaCert,
				})
			}

		}
		container.VolumeMounts = volumeMounts
	}
	return container
}

func nameForListener(l *v1alpha1.Listener) string {
	if l.Name == "" {
		return "port-" + strconv.Itoa(int(l.Port))
	} else {
		return l.Name
	}
}

func containerPortsForListeners(listeners []v1alpha1.Listener) []v1.ContainerPort{
	ports := []v1.ContainerPort{}
	for _, listener := range listeners {
		ports = append(ports, v1.ContainerPort{
			Name: nameForListener(&listener),
			ContainerPort: listener.Port,
		})
	}
	return ports
}

func containerPortsForRouter(m *v1alpha1.Router) []v1.ContainerPort{
	ports := containerPortsForListeners(m.Spec.Listeners)
	ports = append(ports, containerPortsForListeners(m.Spec.InterRouterListeners)...)
	return ports
}

func servicePortsForListeners(listeners []v1alpha1.Listener) []v1.ServicePort{
	ports := []v1.ServicePort{}
	for _, listener := range listeners {
		ports = append(ports, v1.ServicePort{
			Name: nameForListener(&listener),
			Protocol: "TCP",
			Port: listener.Port,
			TargetPort: intstr.FromInt(int(listener.Port)),
		})
	}
	return ports
}

func portsForRouter(m *v1alpha1.Router) []v1.ServicePort{
	ports := []v1.ServicePort{}
	external := servicePortsForListeners(m.Spec.Listeners)
	internal := servicePortsForListeners(m.Spec.InterRouterListeners)
	ports = append(ports, external...)
	ports = append(ports, internal...)
	return ports
}

func checkService(desired *v1.Service, actual *v1.Service) bool {
	update := false
	if !reflect.DeepEqual(desired.Annotations[certRequestAnnotation], actual.Annotations[certRequestAnnotation]) {
		actual.Annotations[certRequestAnnotation] = desired.Annotations[certRequestAnnotation]
	}
	if !reflect.DeepEqual(desired.Spec.Selector, actual.Spec.Selector) {
		actual.Spec.Selector = desired.Spec.Selector
	}
	if !reflect.DeepEqual(desired.Spec.Ports, actual.Spec.Ports) {
		actual.Spec.Ports = desired.Spec.Ports
	}
	return update
}

func serviceForRouter(m *v1alpha1.Router, requestCert bool) *v1.Service {
	ls := labelsForRouter(m.Name)
	service := &v1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Spec: v1.ServiceSpec{
			Selector: ls,
			Ports: portsForRouter(m),
		},
	}
	if requestCert {
		service.Annotations = map[string]string{certRequestAnnotation: m.Name + "-cert"}
	}
	addOwnerRefToObject(service, asOwner(m))
	return service
}

// deploymentForRouter returns a router Deployment object
func deploymentForRouter(m *v1alpha1.Router) *appsv1.Deployment {
	ls := labelsForRouter(m.Name)
	config := configForRouter(m)
	dep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{containerForRouter(m, config)},
				},
			},
		},
	}
	volumes := []v1.Volume{}
	for _, profile := range m.Spec.SslProfiles {
		if len(profile.Credentials) > 0 {
			volumes = append(volumes, v1.Volume{
				Name: profile.Credentials,
				VolumeSource: v1.VolumeSource{
					Secret: &v1.SecretVolumeSource{
						SecretName: profile.Credentials,
					},
				},
			})
		}
		if len(profile.CaCert) > 0 && profile.CaCert != profile.Credentials {
			volumes = append(volumes, v1.Volume{
				Name: profile.CaCert,
				VolumeSource: v1.VolumeSource{
					Secret: &v1.SecretVolumeSource{
						SecretName: profile.CaCert,
					},
				},
			})
		}
	}
	dep.Spec.Template.Spec.Volumes = volumes
	size := m.Spec.Size
	if size != 0 {
		dep.Spec.Replicas = &size
	}
	addOwnerRefToObject(dep, asOwner(m))
	return dep
}

// labelsForRouter returns the labels for selecting the resources
// belonging to the given router CR name.
func labelsForRouter(name string) map[string]string {
	return map[string]string{"application": name, "router_cr": name}
}

// addOwnerRefToObject appends the desired OwnerReference to the object
func addOwnerRefToObject(obj metav1.Object, ownerRef metav1.OwnerReference) {
	obj.SetOwnerReferences(append(obj.GetOwnerReferences(), ownerRef))
}

// asOwner returns an OwnerReference set as the router CR
func asOwner(m *v1alpha1.Router) metav1.OwnerReference {
	trueVar := true
	return metav1.OwnerReference{
		APIVersion: m.APIVersion,
		Kind:       m.Kind,
		Name:       m.Name,
		UID:        m.UID,
		Controller: &trueVar,
	}
}

// podList returns a v1.PodList object
func podList() *v1.PodList {
	return &v1.PodList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
	}
}

// getPodNames returns the pod names of the array of pods passed in
func getPodNames(pods []v1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}
