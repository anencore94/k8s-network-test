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
	PodName1Prefix  = "alpha-"
	PodName2Prefix  = "beta-"

	GoogleDNS = "google.com"
	GoogleIP  = "8.8.8.8"

	Timeout         = time.Second * 300
	PollingInterval = time.Second * 10
)

var (
	err       error // BeforeEach, AfterEach 때문에 변수로 초기 선언
	clientset *kubernetes.Clientset
	config    *restclient.Config

	defaultNamespaceName = "default"
	testingNamespace     *corev1.Namespace
	nodesNum             int
	testCaseNum          int
)

var _ = Describe("SIMPLE NETWORK TESTING TOOL", func() {
	BeforeSuite(func() {
		clientset, config = getClientSet()
		glog.Info("========== [TEST] End Fetching Current kubernetes client ==========\n")

		glog.Info("========== [TEST] Start Checking Current Cluster ==========\n")
		glog.Info("Get the number of nodes")
		//TODO ready 인 node list 를 받아놓고 name 을 저장하여 추후 pod 생성 시 사용하도록
		nodes, err := clientset.CoreV1().Nodes().List(metav1.ListOptions{})
		if err != nil {
			panic(err)
		}
		nodesNum := len(nodes.Items)
		Expect(err).ToNot(HaveOccurred())
		Expect(nodesNum).NotTo(Equal(0))

		glog.Infof("The number of nodes is %d", nodesNum)
		glog.Info("========== [TEST] End Checking Current Cluster ==========\n")
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
		}, Timeout, PollingInterval).Should(BeTrue())
	})

	// TODO Tests Cases :
	// node 개수 n 일 때,
	// case A) (같은 노드 같은 ns), (같은 노드 다른 ns), (다른 노드 같은 ns), (다른 노드 다른 ns) 사이 : 4 개
	// case B) (노드 1에서 외부망), (노드 2에서 외부망), (노드 3), ... 에서 외부망(google.com, 8.8.8.8) : 2n 개
	// case C) (임의의 노드 default ns 에서 임의의 노드 custom ns) 사이 : 1 개
	// case D) (임의의 노드 default ns 에서 google.com 으로), (임의의 노드 default ns 에서 8.8.8.8 로) : 1 개

	// case A-1
	Describe("Test Pod Network In the same Namespace", func() {
		It("Check ping between pods in the same namespace by ip address", func() {
			pod1, err := createPod(clientset, PodName1Prefix, testingNamespace.Name)
			Expect(err).ToNot(HaveOccurred())
			glog.Infof("pod %s is created\n", pod1.Name)

			pod2, err := createPod(clientset, PodName2Prefix, testingNamespace.Name)
			Expect(err).ToNot(HaveOccurred())
			glog.Infof("pod %s is created\n", pod2.Name)

			err = waitTimeoutForPodStatus(clientset, pod1.Name, pod1.Namespace, corev1.PodRunning, time.Second*30)
			Expect(err).ToNot(HaveOccurred())
			err = waitTimeoutForPodStatus(clientset, pod2.Name, pod2.Namespace, corev1.PodRunning, time.Second*30)
			Expect(err).ToNot(HaveOccurred())

			pod1IP, err := getPodIP(clientset, pod1.Name, testingNamespace.Name)
			Expect(err).ToNot(HaveOccurred())
			pod2IP, err := getPodIP(clientset, pod2.Name, testingNamespace.Name)
			Expect(err).ToNot(HaveOccurred())

			glog.Infof("IP of pod_1 is %s\n", pod1IP)
			glog.Infof("IP of pod_2 is %s\n", pod2IP)

			// check ping each other
			Eventually(func() bool {
				return isPossibleToPingFromPodToIP(pod1.Name, testingNamespace.Name, pod2IP, clientset, config)
			}, Timeout, PollingInterval).Should(BeTrue())

			Eventually(func() bool {
				return isPossibleToPingFromPodToIP(pod2.Name, testingNamespace.Name, pod1IP, clientset, config)
			}, Timeout, PollingInterval).Should(BeTrue())
		})
	})

	// case A-4)
	Describe("Test Pod Network Between the different Namespace", func() {
		It("Check ping between pods in the different namespace by ip address", func() {
			pod1, err := createPod(clientset, PodName1Prefix, testingNamespace.Name)
			Expect(err).ToNot(HaveOccurred())
			glog.Infof("pod %s is created\n", pod1.Name)

			// creating another Namespace
			anotherNamespace, err := createNamespace(clientset, makeNamespaceSpec(NamespacePrefix+"another-"))
			Expect(err).ToNot(HaveOccurred())
			glog.Infof("Another Namespace %s is created\n", anotherNamespace.Name)

			pod2, err := createPod(clientset, PodName2Prefix, anotherNamespace.Name)
			Expect(err).ToNot(HaveOccurred())
			glog.Infof("pod %s is created\n", anotherNamespace.Name)

			err = waitTimeoutForPodStatus(clientset, pod1.Name, pod1.Namespace, corev1.PodRunning, time.Second*30)
			Expect(err).ToNot(HaveOccurred())
			err = waitTimeoutForPodStatus(clientset, pod2.Name, pod2.Namespace, corev1.PodRunning, time.Second*30)
			Expect(err).ToNot(HaveOccurred())

			pod1IP, err := getPodIP(clientset, pod1.Name, testingNamespace.Name)
			Expect(err).ToNot(HaveOccurred())
			pod2IP, err := getPodIP(clientset, pod2.Name, anotherNamespace.Name)
			Expect(err).ToNot(HaveOccurred())

			glog.Infof("IP of pod_1 is %s\n", pod1IP)
			glog.Infof("IP of pod_2 is %s\n", pod2IP)

			Eventually(func() bool {
				return isPossibleToPingFromPodToIP(pod1.Name, testingNamespace.Name, pod2IP, clientset, config)
			}, Timeout, PollingInterval).Should(BeTrue())

			Eventually(func() bool {
				return isPossibleToPingFromPodToIP(pod2.Name, anotherNamespace.Name, pod1IP, clientset, config)
			}, Timeout, PollingInterval).Should(BeTrue())

			// TODO must Delete another namespace
			err = clientset.CoreV1().Namespaces().Delete(anotherNamespace.Name, &metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() bool {
				ns, err := clientset.CoreV1().Namespaces().Get(anotherNamespace.Name, metav1.GetOptions{})
				if err != nil || errors.IsNotFound(err) {
					return true
				}

				if ns.Status.Phase == corev1.NamespaceTerminating {
					glog.Infof("Namespace %s is still in phase %s\n", anotherNamespace.Name, ns.Status.Phase)
					return false
				}
				return false
			}, Timeout, PollingInterval).Should(BeTrue())
		})
	})

	// case D-1 (임의의 노드 default ns 에서 google.com 으로)
	Describe("Test Pod Network From default ns To external server by DNS", func() {
		It("Check ping from default namespaced pod to google.com", func() {
			defaultNamespacedPod, err := createPod(clientset, "default-ns-"+PodName2Prefix, defaultNamespaceName)
			Expect(err).ToNot(HaveOccurred())
			glog.Infof("pod %s is created\n", defaultNamespacedPod.Name)

			err = waitTimeoutForPodStatus(clientset, defaultNamespacedPod.Name, defaultNamespacedPod.Namespace, corev1.PodRunning, time.Second*30)
			Expect(err).ToNot(HaveOccurred())

			testingPod, err := getPodIP(clientset, defaultNamespacedPod.Name, defaultNamespaceName)
			Expect(err).ToNot(HaveOccurred())

			glog.Infof("IP of testingPod is %s\n", testingPod)

			Eventually(func() bool {
				return isPossibleToPingFromPodToIP(defaultNamespacedPod.Name, defaultNamespaceName, GoogleDNS, clientset, config)
			}, Timeout, PollingInterval).Should(BeTrue())

			// TODO must Delete default ns pod but my harmful
			err = clientset.CoreV1().Pods(defaultNamespaceName).Delete(defaultNamespacedPod.Name, &metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() bool {
				pod, err := clientset.CoreV1().Pods(defaultNamespaceName).Get(defaultNamespacedPod.Name, metav1.GetOptions{})
				if err != nil || errors.IsNotFound(err) {
					return true
				}

				if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodPending {
					glog.Infof("Pod %s is still in phase %s\n", defaultNamespacedPod.Name, pod.Status.Phase)
					return false
				}
				return false
			}, Timeout, PollingInterval).Should(BeTrue())
		})
	})

	// case D-2 (임의의 노드 default ns 에서 8.8.8.8 로)
	Describe("Test Pod Network From default ns To 8.8.8.8", func() {
		It("Check ping to 8.8.8.8, you may need to check /etc/resolve.conf if this test failed", func() {
			defaultNamespacedPod, err := createPod(clientset, "default-ns-"+PodName2Prefix, defaultNamespaceName)
			Expect(err).ToNot(HaveOccurred())
			glog.Infof("pod %s is created\n", defaultNamespacedPod.Name)

			err = waitTimeoutForPodStatus(clientset, defaultNamespacedPod.Name, defaultNamespacedPod.Namespace, corev1.PodRunning, time.Second*30)
			Expect(err).ToNot(HaveOccurred())

			pod2IP, err := getPodIP(clientset, defaultNamespacedPod.Name, defaultNamespaceName)
			Expect(err).ToNot(HaveOccurred())

			glog.Infof("IP of pod_2 is %s\n", pod2IP)

			Eventually(func() bool {
				return isPossibleToPingFromPodToIP(defaultNamespacedPod.Name, defaultNamespaceName, GoogleIP, clientset, config)
			}, Timeout, PollingInterval).Should(BeTrue())

			// TODO must Delete default ns pod but my harmful
			err = clientset.CoreV1().Pods(defaultNamespaceName).Delete(defaultNamespacedPod.Name, &metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() bool {
				pod, err := clientset.CoreV1().Pods(defaultNamespaceName).Get(defaultNamespacedPod.Name, metav1.GetOptions{})
				if err != nil || errors.IsNotFound(err) {
					return true
				}

				if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodPending {
					glog.Infof("Pod %s is still in phase %s\n", defaultNamespacedPod.Name, pod.Status.Phase)
					return false
				}
				return false
			}, Timeout, PollingInterval).Should(BeTrue())
		})
	})
})
