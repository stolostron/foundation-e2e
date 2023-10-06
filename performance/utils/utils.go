// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	workv1 "open-cluster-management.io/api/work/v1"
)

const (
	PerformanceTestLabel       = "perftest.open-cluster-management.io"
	WorkCreationTimeRecordFile = "works-creation-time"
	ResourceMetricsRecordFile  = "resource-metrics"

	ExpectedWorkCountAnnotation = "perftest.open-cluster-management.io/expected-work-count"
	DefaultContent              = "I'm a test configmap"

	CreateInterval = 0 * time.Second
)

var (
	genericScheme = runtime.NewScheme()
	genericCodecs = serializer.NewCodecFactory(genericScheme)
	genericCodec  = genericCodecs.UniversalDeserializer()
)

func init() {
	utilruntime.Must(workv1.AddToScheme(genericScheme))
}

func PrintMsg(msg string) {
	now := time.Now()
	fmt.Fprintf(os.Stdout, "[%s] %s\n", now.Format(time.RFC3339), msg)
}

func AppendRecordToFile(filename, record string) error {
	if filename == "" {
		return nil
	}

	if record == "" {
		return nil
	}

	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.WriteString(fmt.Sprintf("%s\n", record)); err != nil {
		return err
	}
	return nil
}

func GenerateManifestWorks(workCount int, clusterName, templateDir string) ([]*workv1.ManifestWork, error) {
	if len(templateDir) == 0 {
		totalWorkloadSize := 0
		works := []*workv1.ManifestWork{}
		for i := 0; i < workCount; i++ {
			workName := fmt.Sprintf("perftest-%s-work-%d", clusterName, i)
			data := []byte(DefaultContent)
			works = append(works, toManifestWork(clusterName, workName, data))
			totalWorkloadSize = totalWorkloadSize + len(data)
		}
		PrintMsg(fmt.Sprintf("Total workload size is %d bytes in cluster %s", totalWorkloadSize, clusterName))
		return works, nil
	}

	works, totalWorkloadSize, err := getManifestWorksFromTemplate(clusterName, templateDir)
	if err != nil {
		return nil, err
	}

	// if template works count is less than expected work count, continue to create default works
	for i := 0; i < workCount-len(works); i++ {
		workName := fmt.Sprintf("perftest-%s-work-%d", clusterName, i)
		data := []byte(DefaultContent)
		works = append(works, toManifestWork(clusterName, workName, data))
		totalWorkloadSize = totalWorkloadSize + len(data)
	}
	PrintMsg(fmt.Sprintf("Total workload size is %d bytes in cluster %s", totalWorkloadSize, clusterName))
	return works, nil
}

func getManifestWorksFromTemplate(clusterName, templateDir string) ([]*workv1.ManifestWork, int, error) {
	files, err := os.ReadDir(templateDir)
	if err != nil {
		return nil, 0, err
	}

	totalWorkloadSize := 0
	works := []*workv1.ManifestWork{}
	for _, file := range files {
		info, err := file.Info()
		if err != nil {
			return nil, 0, err
		}

		if info.IsDir() {
			continue
		}

		if filepath.Ext(info.Name()) != ".yaml" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(templateDir, info.Name()))
		if err != nil {
			return nil, 0, err
		}

		obj, _, err := genericCodec.Decode(data, nil, nil)
		if err != nil {
			return nil, 0, err
		}
		switch work := obj.(type) {
		case *workv1.ManifestWork:
			expectedWorkCountStr, ok := work.Annotations[ExpectedWorkCountAnnotation]
			if !ok {
				return nil, 0, fmt.Errorf("annotation %q is required", ExpectedWorkCountAnnotation)
			}

			expectedWorkCount, err := strconv.Atoi(expectedWorkCountStr)
			if err != nil {
				return nil, 0, err
			}

			for i := 0; i < expectedWorkCount; i++ {
				workName := fmt.Sprintf("perftest-%s-work-%s-%d", clusterName, work.Name, i)
				PrintMsg(fmt.Sprintf("work %s is created from template %s", workName, info.Name()))
				works = append(works, toManifestWork(clusterName, workName, data))
				totalWorkloadSize = totalWorkloadSize + len(data)
			}
		}
	}

	return works, totalWorkloadSize, nil
}

func generateManifest(workName string, data []byte) workv1.Manifest {
	manifest := workv1.Manifest{}
	manifest.Object = &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      workName,
		},
		BinaryData: map[string][]byte{
			"test-data": data,
		},
	}
	return manifest
}

func toManifestWork(clusterName, workName string, data []byte) *workv1.ManifestWork {
	return &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workName,
			Namespace: clusterName,
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{
					generateManifest(workName, data),
				},
			},
		},
	}
}

const kubeConfigFileEnv = "KUBECONFIG"

type QueryResult struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric struct {
				Container string `json:"container"`
			} `json:"metric,omitempty"`
			Value []interface{} `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

var routeGvr = schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}

func GetKubeConfigFile() (string, error) {
	kubeConfigFile := os.Getenv(kubeConfigFileEnv)
	if kubeConfigFile == "" {
		user, err := user.Current()
		if err != nil {
			return "", err
		}
		kubeConfigFile = path.Join(user.HomeDir, ".kube", "config")
	}

	return kubeConfigFile, nil
}

func NewKubeConfig() (*rest.Config, error) {
	kubeConfigFile, err := GetKubeConfigFile()
	if err != nil {
		return nil, err
	}

	cfg, err := clientcmd.BuildConfigFromFlags("", kubeConfigFile)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func NewKubeClient() (kubernetes.Interface, error) {
	kubeConfigFile, err := GetKubeConfigFile()
	if err != nil {
		return nil, err
	}

	cfg, err := clientcmd.BuildConfigFromFlags("", kubeConfigFile)
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(cfg)
}

func NewDynamicClient() (dynamic.Interface, error) {
	kubeConfigFile, err := GetKubeConfigFile()
	if err != nil {
		return nil, err
	}
	fmt.Printf("Use kubeconfig file: %s\n", kubeConfigFile)

	clusterCfg, err := clientcmd.BuildConfigFromFlags("", kubeConfigFile)
	if err != nil {
		return nil, err
	}

	dynamicClient, err := dynamic.NewForConfig(clusterCfg)
	if err != nil {
		return nil, err
	}

	return dynamicClient, nil
}

func GetPromotheusUrl(dynamicClient dynamic.Interface) (string, error) {
	premothusRoute, err := dynamicClient.Resource(routeGvr).Namespace("openshift-monitoring").Get(context.Background(), "prometheus-k8s", metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	url, _, err := unstructured.NestedString(premothusRoute.Object, "spec", "host")
	return url, err
}

func GetPromotheusToken(kubeClient kubernetes.Interface) (string, error) {
	sa, err := kubeClient.CoreV1().ServiceAccounts("openshift-monitoring").Get(context.Background(), "prometheus-k8s", metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	if len(sa.Secrets) == 0 {
		return "", fmt.Errorf("no promethus sa secret.")
	}
	promethusSecret, err := kubeClient.CoreV1().Secrets("openshift-monitoring").Get(context.Background(), sa.Secrets[0].Name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	if _, ok := promethusSecret.Annotations["openshift.io/token-secret.value"]; !ok {
		return "", fmt.Errorf("no token-secret found in promethus secret.")
	}
	return promethusSecret.Annotations["openshift.io/token-secret.value"], nil
}

func GetHttpsSkip(url, token string) (bodyBytes []byte, err error) {
	var client *http.Client
	var request *http.Request
	var resp *http.Response
	client = &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}}
	request, err = http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Add("Authorization", token)
	resp, err = client.Do(request)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("response error: %v", resp)
	}
	if resp.StatusCode == http.StatusOK {
		bodyBytes, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			klog.Fatal(err)
		}
	}
	defer client.CloseIdleConnections()
	return bodyBytes, nil
}

func QueryMetrics(kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, promQL string) (queryResult *QueryResult, err error) {
	token, err := GetPromotheusToken(kubeClient)
	if err != nil {
		return nil, err
	}
	url, err := GetPromotheusUrl(dynamicClient)
	if err != nil {
		return nil, err
	}
	promURL := "https://" + url + "/api/v1/query?query=" + promQL
	klog.Infof("queryMetrics: %s", promURL)
	data, err := GetHttpsSkip(promURL, "Bearer "+token)

	err = json.Unmarshal(data, &queryResult)
	if err != nil {
		return nil, err
	}
	klog.Errorf("query result data: %v", queryResult.Data.Result)

	return queryResult, nil
}

func GetKubeApiPodsName(kubeClient kubernetes.Interface) ([]string, error) {
	var kubepods []string
	podlist, err := kubeClient.CoreV1().Pods("openshift-kube-apiserver").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, p := range podlist.Items {
		if strings.HasPrefix(p.Name, "kube-apiserver-ip") {
			kubepods = append(kubepods, p.Name)
		}
	}
	return kubepods, nil
}

func GetKubeApiResourceUsage(queryResult *QueryResult) (float64, error) {
	if queryResult == nil {
		return 0, fmt.Errorf("Failed to get kube-apiserver cpu usage")
	}
	for _, result := range queryResult.Data.Result {
		if result.Metric.Container == "kube-apiserver" {
			return strconv.ParseFloat(result.Value[1].(string), 64)
		}
	}
	return 0, fmt.Errorf("Failed to get kube-apiserver cpu usage")
}

func CreateClusterNamespace(ctx context.Context, hubClient kubernetes.Interface, name string) error {
	_, err := hubClient.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	_, err = hubClient.CoreV1().Namespaces().Create(
		ctx,
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					PerformanceTestLabel: "true",
				},
			},
		},
		metav1.CreateOptions{},
	)
	return err
}

func DeleteClusterNamespace(ctx context.Context, hubClient kubernetes.Interface, name string) error {
	err := hubClient.CoreV1().Namespaces().Delete(context.Background(), name, metav1.DeleteOptions{})
	if err == nil {
		gomega.Eventually(func() error {
			_, err := hubClient.CoreV1().Namespaces().Get(context.Background(), name, v1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("Wait namespace %v deleted. err:%v", name, err)
		}, 300*time.Second, 1*time.Second).ShouldNot(gomega.HaveOccurred())
		return nil
	}

	if errors.IsNotFound(err) {
		return nil
	}

	return err
}

func Noc(n int) *int32 {
	noc := int32(n)
	return &noc
}
