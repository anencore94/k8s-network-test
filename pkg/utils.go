package sntt

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/golang/glog"
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

func getClientSet() (*kubernetes.Clientset, *restclient.Config) {
	var kubeconfig *string
	flag.Set("logtostderr", "true")

	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	if home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

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
			APIVersion: "v1",
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

func makePodSpec(podNamePrefix string, namespace string) *corev1.Pod {
	//TODO need to be clean
	cmd := []string{"sleep", "3600"}

	podSpec := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
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

func createPod(clientset *kubernetes.Clientset, podName string, namespace string) (*corev1.Pod, error) {
	pod := makePodSpec(podName, namespace)
	podOut, err := clientset.CoreV1().Pods(namespace).Create(pod)

	return podOut, err
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

func getPodIp(clientset *kubernetes.Clientset, podName string, namespace string) (string, error) {
	out, err := clientset.CoreV1().Pods(namespace).Get(podName, metav1.GetOptions{})

	return out.Status.PodIP, err
}

/////////////////////
// TODO pod2pod network test 더 간단하게 하는 방법
// 아래 코드는 a4abhishek / Client-Go-Examples 의 github 참고
func canPingFromPodToIpAddr(podName string, namespace string, destinationIpAddress string, clientset *kubernetes.Clientset,
	config *restclient.Config) bool {
	glog.Infof("====== Trying to ping from '%s' pod => '%s' for every %.1f seconds ======", podName, destinationIpAddress, pollIntervalToPing.Seconds())
	//TODO 커맨드에 ping 명령어 이후 파이프라인(|)이랑 "> /dev/null" 먹지 않아서 조잡하게 코드 짰는데 확인 필요
	command := []string{"/bin/ping", "-c", "2", destinationIpAddress}

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
