package sntt

import (
	"github.com/golang/glog"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"time"
)

const (
	NamespacePrefix = "test-ns-"
	podName1Prefix  = "alpha-"
	podName2Prefix  = "beta-"
	timeout         = time.Second * 300
	pollingInterval = time.Second * 10
)

var (
	err       error // BeforeEach, AfterEach 때문에 변수로 초기 선언
	clientset *kubernetes.Clientset
	config    *restclient.Config

	testingNamespace *corev1.Namespace
	nodesNum         int
	testCaseNum      int
)

var _ = Describe("SIMPLE NETWORK TESTING TOOL", func() {
	BeforeSuite(func() {
		glog.Info("========== [TEST] Checking Prerequisites Started ==========\n")
		clientset, config = getClientSet()

		glog.Info("Get the number of nodes")
		nodes, err := clientset.CoreV1().Nodes().List(metav1.ListOptions{})
		if err != nil {
			panic(err)
		}
		nodesNum := len(nodes.Items)
		Expect(err).ToNot(HaveOccurred())
		Expect(nodesNum).NotTo(Equal(0))

		glog.Infof("The number of nodes is %d", nodesNum)
		glog.Info("========== [TEST] Checking Prerequisites End ==========\n")
	})
	BeforeEach(func() {
		testCaseNum++
		glog.Infof("========== [TEST][CASE-#%d] Started ==========\n", testCaseNum)

		// create testing namespace
		testingNamespace, err = createNamespace(clientset, makeNamespaceSpec(NamespacePrefix))
		Expect(err).ToNot(HaveOccurred())
		glog.Infof("Namespace %s is created\n", testingNamespace.Name)
	})
	AfterEach(func() {
		err := clientset.CoreV1().Namespaces().Delete(testingNamespace.Name, &metav1.DeleteOptions{})
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() bool {
			ns, err := clientset.CoreV1().Namespaces().Get(testingNamespace.Name, metav1.GetOptions{})
			if err != nil || errors.IsNotFound(err) {
				return true
			}

			if ns.Status.Phase == corev1.NamespaceTerminating {
				glog.Infof("Namespace %s is still in phase %s\n", testingNamespace.Name, ns.Status.Phase)
				return false
			}
			return false
		}, timeout, pollingInterval).Should(BeTrue())
	})

	Describe("Pod Networking In the same Namespace", func() {
		It("Check ping between pods in the same namespace by ip address", func() {
			pod1, err := createPod(clientset, podName1Prefix, testingNamespace.Name)
			Expect(err).ToNot(HaveOccurred())
			glog.Infof("pod %s is created\n", pod1.Name)

			pod2, err := createPod(clientset, podName2Prefix, testingNamespace.Name)
			Expect(err).ToNot(HaveOccurred())
			glog.Infof("pod %s is created\n", pod2.Name)

			err = waitTimeoutForPodStatus(clientset, pod1.Name, pod1.Namespace, corev1.PodRunning, time.Second*30)
			Expect(err).ToNot(HaveOccurred())
			err = waitTimeoutForPodStatus(clientset, pod2.Name, pod2.Namespace, corev1.PodRunning, time.Second*30)
			Expect(err).ToNot(HaveOccurred())

			pod1Ip, err := getPodIp(clientset, pod1.Name, testingNamespace.Name)
			Expect(err).ToNot(HaveOccurred())
			pod2Ip, err := getPodIp(clientset, pod2.Name, testingNamespace.Name)
			Expect(err).ToNot(HaveOccurred())

			glog.Infof("IP of pod_1 is %s\n", pod1Ip)
			glog.Infof("IP of pod_2 is %s\n", pod2Ip)

			// check each ping test case
			Eventually(func() bool {
				return canPingFromPodToIpAddr(pod1.Name, testingNamespace.Name, pod2Ip, clientset, config)
			}, timeout, pollingInterval).Should(BeTrue())

			Eventually(func() bool {
				return canPingFromPodToIpAddr(pod2.Name, testingNamespace.Name, pod1Ip, clientset, config)
			}, timeout, pollingInterval).Should(BeTrue())
		})
	})

	Describe("Pod Networking To external dns server", func() {
		It("Check ping to google.com", func() {
			pod1, err := createPod(clientset, podName1Prefix, testingNamespace.Name)
			Expect(err).ToNot(HaveOccurred())
			glog.Infof("pod %s is created\n", pod1.Name)

			// default-ns-pod to external dns server
			defaultNamespace := "default"
			defaultNamespacedPod, err := createPod(clientset, "default-ns-"+podName2Prefix, defaultNamespace)
			Expect(err).ToNot(HaveOccurred())
			glog.Infof("pod %s is created\n", defaultNamespacedPod.Name)

			err = waitTimeoutForPodStatus(clientset, pod1.Name, pod1.Namespace, corev1.PodRunning, time.Second*30)
			Expect(err).ToNot(HaveOccurred())
			err = waitTimeoutForPodStatus(clientset, defaultNamespacedPod.Name, defaultNamespacedPod.Namespace, corev1.PodRunning, time.Second*30)
			Expect(err).ToNot(HaveOccurred())

			pod1Ip, err := getPodIp(clientset, pod1.Name, testingNamespace.Name)
			Expect(err).ToNot(HaveOccurred())
			pod2Ip, err := getPodIp(clientset, defaultNamespacedPod.Name, defaultNamespace)
			Expect(err).ToNot(HaveOccurred())

			glog.Infof("IP of pod_1 is %s\n", pod1Ip)
			glog.Infof("IP of pod_2 is %s\n", pod2Ip)

			googleAddress := "google.com"
			Eventually(func() bool {
				return canPingFromPodToIpAddr(pod1.Name, testingNamespace.Name, googleAddress, clientset, config)
			}, timeout, pollingInterval).Should(BeTrue())

			Eventually(func() bool {
				return canPingFromPodToIpAddr(defaultNamespacedPod.Name, defaultNamespace, googleAddress, clientset, config)
			}, timeout, pollingInterval).Should(BeTrue())

			// TODO must Delete default ns pod but my harmful
			err = clientset.CoreV1().Pods(defaultNamespace).Delete(defaultNamespacedPod.Name, &metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() bool {
				pod, err := clientset.CoreV1().Pods(defaultNamespace).Get(defaultNamespacedPod.Name, metav1.GetOptions{})
				if err != nil || errors.IsNotFound(err) {
					return true
				}

				if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodPending {
					glog.Infof("Pod %s is still in phase %s\n", defaultNamespacedPod.Name, pod.Status.Phase)
					return false
				}
				return false
			}, timeout, pollingInterval).Should(BeTrue())
		})
	})
})
