package sntt

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/golang/glog"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	wait "k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const pollIntervalToPing = 2 * time.Second // retry every 3 s

// getKubeconfigPathFromEnv gets the path to the first kubeconfig
func getKubeconfigPathFromEnv() string {
	kubeConfigEnv := os.Getenv("KUBECONFIG")

	if kubeConfigEnv == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			panic(err)
		}
		kubeConfigEnv = filepath.Join(home, ".kube", "config")
	}

	return kubeConfigEnv
}

func getClientSet() (*kubernetes.Clientset, *restclient.Config) {
	var kubeconfig *string
	flag.Set("logtostderr", "true")
	glog.Info("========== [TEST] Start Fetching Current kubernetes client ==========\n")

	kubeconfig = flag.String("kubeconfig", getKubeconfigPathFromEnv(), "absolute path to the kubeconfig file")

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return clientset, config
}

func makeNamespaceSpec(namespacePrefix string) *corev1.Namespace {
	namespaceSpec := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "appsv1",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: namespacePrefix,
		},
	}

	return namespaceSpec
}

func createNamespace(clientset *kubernetes.Clientset, nsSpec *corev1.Namespace) (*corev1.Namespace, error) {
	ns, err := clientset.CoreV1().Namespaces().Create(nsSpec)

	return ns, err
}

func makePodSpecInSpecificNode(podNamePrefix string, nodeName string, namespace string) *corev1.Pod {
	//TODO need to be clean
	cmd := []string{"sleep", "3600"}

	podSpec := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "appsv1",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: podNamePrefix,
			Namespace:    namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Image:           "busybox",
					Name:            "busybox",
					Command:         cmd,
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
			},
			RestartPolicy: corev1.RestartPolicyAlways,
			NodeName:      nodeName,
		},
	}

	return podSpec
}

func makePodSpec(podNamePrefix string, namespace string) *corev1.Pod {
	//TODO need to be clean
	cmd := []string{"sleep", "3600"}

	podSpec := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "appsv1",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: podNamePrefix,
			Namespace:    namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Image:           "busybox",
					Name:            "busybox",
					Command:         cmd,
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
			},
			RestartPolicy: corev1.RestartPolicyAlways,
		},
	}

	return podSpec
}

func makeDaemonsetSpec(dmsNamePrefix string, namespace string) *appsv1.DaemonSet {
	cmd := []string{"sleep", "3600"}

	dmsSpec := &appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: dmsNamePrefix,
			Namespace:    namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"sntt": "daemonset",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"sntt": "daemonset",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image:           "busybox",
							Name:            "busybox",
							Command:         cmd,
							ImagePullPolicy: corev1.PullIfNotPresent,
						},
					},
					RestartPolicy: corev1.RestartPolicyAlways,
				},
			},
		},
	}

	return dmsSpec
}

func createPodInSpecificNode(clientset *kubernetes.Clientset, podName string, nodeName string, namespace string) (*corev1.Pod, error) {
	pod := makePodSpecInSpecificNode(podName, nodeName, namespace)
	podOut, err := clientset.CoreV1().Pods(namespace).Create(pod)

	return podOut, err
}

func createPodInRandomNode(clientset *kubernetes.Clientset, podName string, namespace string) (*corev1.Pod, error) {
	pod := makePodSpec(podName, namespace)
	podOut, err := clientset.CoreV1().Pods(namespace).Create(pod)

	return podOut, err
}

func createDaemonset(clientset *kubernetes.Clientset, dmsName string, namespace string) (*appsv1.DaemonSet, error) {
	dms := makeDaemonsetSpec(dmsName, namespace)
	dmsOut, err := clientset.AppsV1().DaemonSets(namespace).Create(dms)

	return dmsOut, err
}

func waitTimeoutForPodStatus(clientset *kubernetes.Clientset, podName string, namespace string,
	desiredStatus corev1.PodPhase, timeout time.Duration) error {
	var pod *corev1.Pod

	err := wait.PollImmediate(pollIntervalToPing, timeout, func() (bool, error) {
		pod, err := clientset.CoreV1().Pods(namespace).Get(podName, metav1.GetOptions{})
		if err != nil || pod.Status.Phase != desiredStatus {
			return false, err
		}
		return true, nil
	})

	if err != nil {
		return fmt.Errorf("Pod %s not in phase %s within %v ", pod, desiredStatus, timeout)
	}

	return nil
}

func waitTimeoutForDaemonsetReady(clientset *kubernetes.Clientset, dmsName string, namespace string,
	timeout time.Duration) error {

	err := wait.PollImmediate(pollIntervalToPing, timeout, func() (bool, error) {
		dmsout, err := clientset.AppsV1().DaemonSets(namespace).Get(dmsName, metav1.GetOptions{})
		if err != nil || dmsout.Status.DesiredNumberScheduled != dmsout.Status.NumberReady ||
			dmsout.Status.DesiredNumberScheduled != dmsout.Status.NumberAvailable {
			glog.Infof("Daemonset %s is still creating", dmsout.Name)
			return false, err
		}
		return true, nil
	})

	if err != nil {
		return fmt.Errorf("Daemonset %s is not ready yet", dmsName)
	}

	return nil
}

func getPodIP(clientset *kubernetes.Clientset, podName string, namespace string) (string, error) {
	out, err := clientset.CoreV1().Pods(namespace).Get(podName, metav1.GetOptions{})

	return out.Status.PodIP, err
}

/////////////////////
// TODO pod2pod network test 더 간단하게 하는 방법
// 아래 코드는 a4abhishek / Client-Go-Examples 의 github 참고
func isPossibleToPingFromPodToIP(podName string, namespace string, destinationIPAddress string, clientset *kubernetes.Clientset,
	config *restclient.Config) bool {
	glog.Infof("====== Trying to ping from '%s' pod => '%s' for every %.1f seconds ======", podName, destinationIPAddress, pollIntervalToPing.Seconds())
	//TODO 커맨드에 ping 명령어 이후 파이프라인(|)이랑 "> /dev/null" 먹지 않아서 조잡하게 코드 짰는데 확인 필요
	command := []string{"/bin/ping", "-c", "2", destinationIPAddress}

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec")

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return false
	}

	parameterCodec := runtime.NewParameterCodec(scheme)
	req.VersionedParams(&corev1.PodExecOptions{
		Command:   command,
		Container: "",
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, parameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return false
	}

	var stdout, stderr bytes.Buffer
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})

	if err != nil {
		return false
	}

	if !strings.Contains(stdout.String(), "0% packet loss") {
		return false
	}

	return true
}
