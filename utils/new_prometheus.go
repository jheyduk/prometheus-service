package utils

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	promConfig "github.com/prometheus/prometheus/config"
	"gopkg.in/yaml.v2"
	"strings"

	"k8s.io/client-go/rest"
)

const prometheusYml = `global:
  scrape_interval: 5s
  evaluation_interval: 5s
rule_files:
  - /etc/prometheus/prometheus.rules
alerting:
  alertmanagers:
  - scheme: http
    static_configs:
    - targets:
      - "alertmanager.monitoring.svc:9093"

scrape_configs:
  - job_name: 'kubernetes-apiservers'

    kubernetes_sd_configs:
    - role: endpoints
    scheme: https

    tls_config:
      ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
    bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token

    relabel_configs:
    - source_labels: [__meta_kubernetes_namespace, __meta_kubernetes_service_name, __meta_kubernetes_endpoint_port_name]
      action: keep
      regex: default;kubernetes;https

  - job_name: 'kubernetes-nodes'

    scheme: https

    tls_config:
      ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
    bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token

    kubernetes_sd_configs:
    - role: node

    relabel_configs:
    - action: labelmap
      regex: __meta_kubernetes_node_label_(.+)
    - target_label: __address__
      replacement: kubernetes.default.svc:443
    - source_labels: [__meta_kubernetes_node_name]
      regex: (.+)
      target_label: __metrics_path__
      replacement: /api/v1/nodes/${1}/proxy/metrics

  
  - job_name: 'kubernetes-pods'

    kubernetes_sd_configs:
    - role: pod

    relabel_configs:
    - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
      action: keep
      regex: true
    - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
      action: replace
      target_label: __metrics_path__
      regex: (.+)
    - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
      action: replace
      regex: ([^:]+)(?::\d+)?;(\d+)
      replacement: $1:$2
      target_label: __address__
    - action: labelmap
      regex: __meta_kubernetes_pod_label_(.+)
    - source_labels: [__meta_kubernetes_namespace]
      action: replace
      target_label: kubernetes_namespace
    - source_labels: [__meta_kubernetes_pod_name]
      action: replace
      target_label: kubernetes_pod_name

  - job_name: 'kubernetes-cadvisor'

    scheme: https

    tls_config:
      ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
    bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token

    kubernetes_sd_configs:
    - role: node

    relabel_configs:
    - action: labelmap
      regex: __meta_kubernetes_node_label_(.+)
    - target_label: __address__
      replacement: kubernetes.default.svc:443
    - source_labels: [__meta_kubernetes_node_name]
      regex: (.+)
      target_label: __metrics_path__
      replacement: /api/v1/nodes/${1}/proxy/metrics/cadvisor
  
  - job_name: 'kubernetes-service-endpoints'

    kubernetes_sd_configs:
    - role: endpoints

    relabel_configs:
    - source_labels: [__meta_kubernetes_service_annotation_prometheus_io_scrape]
      action: keep
      regex: true
    - source_labels: [__meta_kubernetes_service_annotation_prometheus_io_scheme]
      action: replace
      target_label: __scheme__
      regex: (https?)
    - source_labels: [__meta_kubernetes_service_annotation_prometheus_io_path]
      action: replace
      target_label: __metrics_path__
      regex: (.+)
    - source_labels: [__address__, __meta_kubernetes_service_annotation_prometheus_io_port]
      action: replace
      target_label: __address__
      regex: ([^:]+)(?::\d+)?;(\d+)
      replacement: $1:$2
    - action: labelmap
      regex: __meta_kubernetes_service_label_(.+)
    - source_labels: [__meta_kubernetes_namespace]
      action: replace
      target_label: kubernetes_namespace
    - source_labels: [__meta_kubernetes_service_name]
      action: replace
      target_label: kubernetes_name`

type PrometheusHelper struct {
	KubeApi *kubernetes.Clientset
}

// NewPrometheusHelper creates a new PrometheusHelper
func NewPrometheusHelper() (*PrometheusHelper, error) {

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)

	if err != nil {
		return nil, err
	}

	return &PrometheusHelper{KubeApi: clientset}, nil
}

func IsScrapeConfigcontains(s []*promConfig.ScrapeConfig, str string) bool {
	for _, v := range s {
		if v.JobName == str {
			return true
		}
	}

	return false
}

func (p *PrometheusHelper) UpdatePrometheusConfigMap() error {

	cm, err := p.GetConfigMap("prometheus-server-conf", "monitoring")
	if err != nil {
		return err
	}

	var keptnPromConfig promConfig.Config
	err = yaml.Unmarshal([]byte(prometheusYml), &keptnPromConfig)
	if err != nil {
		return err
	}

	if strings.Contains(fmt.Sprint(cm.Data), "scrape_configs") {
		var config promConfig.Config

		for key, proms := range cm.Data {

			err = yaml.Unmarshal([]byte(proms), &config)
			if err != nil {
				return err
			}

			for _, sc := range keptnPromConfig.ScrapeConfigs {
				if !IsScrapeConfigcontains(config.ScrapeConfigs, sc.JobName) {
					config.ScrapeConfigs = append(config.ScrapeConfigs, keptnPromConfig.ScrapeConfigs...)
				} else {
					return err
				}
			}


			cm.Data[key] = fmt.Sprint(config)
		}
	} else {
		yamlString, err := yaml.Marshal(keptnPromConfig)
		if err != nil {
			return err
		}

		cm.Data = map[string]string{
			"prometheus.yml": string(yamlString),
		}
	}


	return p.UpdateConfigMap(cm)
}

func (p *PrometheusHelper) UpdateConfigMap(cm *v1.ConfigMap) error {
	//_, err := p.KubeApi.CoreV1().ConfigMaps("monitoring").Create(cm)
	//if err != nil {
	//	_, err := p.KubeApi.CoreV1().ConfigMaps("monitoring").Update(cm)
	//	if err != nil {
	//		return err
	//	}
	//}
	_, err := p.KubeApi.CoreV1().ConfigMaps("monitoring").Update(cm)
	if err != nil {
		return err
	}

	return nil
}

func (p *PrometheusHelper) GetConfigMap(name string, namespace string) (*v1.ConfigMap, error) {
	return p.KubeApi.CoreV1().ConfigMaps(namespace).Get(name, metav1.GetOptions{})
}
