/*
 * This file is part of the KubeVirt project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright The KubeVirt Authors
 *
 */

package virtctl

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	"kubevirt.io/kubevirt/pkg/virtctl/guestfs"
	"kubevirt.io/kubevirt/tests/decorators"
	"kubevirt.io/kubevirt/tests/exec"
	"kubevirt.io/kubevirt/tests/framework/kubevirt"
	"kubevirt.io/kubevirt/tests/framework/matcher"
	"kubevirt.io/kubevirt/tests/libstorage"
	"kubevirt.io/kubevirt/tests/testsuite"
)

func dlog(format string, args ...any) {
	currentTime := time.Now()
	t := currentTime.Format("15:04:05.000")
	fmt.Fprintf(GinkgoWriter, "[debug] "+t+" "+format+"\n", args...)
}

func CheckGuestfsToolsImage() (int, error) {
	var image string
	info, err := kubevirt.Client().GuestfsVersion().Get()
	Expect(err).NotTo(HaveOccurred())
	if info.GsImage != "" {
		image = info.GsImage
	} else {
		image = fmt.Sprintf("%s%s", info.ImagePrefix, guestfs.DefaultImageName)
		if info.Digest != "" {
			image = fmt.Sprintf("%s@%s", image, info.Digest)
		} else if info.Tag != "" {
			image = fmt.Sprintf("%s:%s", image, info.Tag)
		} else {
			return 0, fmt.Errorf("Neither the digest nor the tag for the image has been specified")
		}

		if info.Registry != "" {
			image = fmt.Sprintf("%s/%s", info.Registry, image)
		}
	}

	dlog("The guestfs image to use is %s", image)

	// timing and pull image

	return 0, nil

}

func delay(dur int) {
	time.Sleep(time.Duration(dur) * time.Minute)
}

var _ = Describe(SIG("[sig-storage]Guestfs", decorators.SigStorage, func() {
	var (
		pvcClaim string
		done     chan struct{}
	)

	// fakeCreateAttacher simulates the attacher to the pod console. It has to block until the test terminates.
	fakeCreateAttacher := func(_ *guestfs.K8sClient, _ *corev1.Pod, _ string) error {
		<-done
		return nil
	}

	BeforeEach(func() {
		guestfs.CreateAttacherFunc = fakeCreateAttacher
		const randNameTail = 5
		pvcClaim = "pvc-" + rand.String(randNameTail)
		done = make(chan struct{})
	})

	AfterEach(func() {
		guestfs.CreateAttacherFunc = guestfs.CreateAttacher
		close(done)
		if CurrentSpecReport().State.Is(types.SpecStateFailed) {
			dlog("failed test, need get some debug info")
			// delay(60)
		}
	})

	Context("[rfe_id:6364]Run libguestfs on PVCs without root", func() {
		// TODO: Always setGroup to true until we have the ability to control how virtctl guestfs is run
		const setGroup = true

		Context("on a FS PVC", func() {
			BeforeEach(func() {
				libstorage.CreateFSPVC(pvcClaim, testsuite.GetTestNamespace(nil), "500Mi", libstorage.WithStorageProfile())
			})

			It("[test_id:11669]Should successfully run libguestfs-test-tool", func() {
				runGuestfsOnPVC(done, pvcClaim, testsuite.GetTestNamespace(nil), setGroup)
				stdout, _, err := execCommandLibguestfsPod(
					getGuestfsPodName(pvcClaim), testsuite.GetTestNamespace(nil), []string{"libguestfs-test-tool"},
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(stdout).To(ContainSubstring("===== TEST FINISHED OK ====="))
			})

			DescribeTable("[posneg:positive]Should successfully run guestfs command on a filesystem-based PVC", func(extraArgs ...string) {
				runGuestfsOnPVC(done, pvcClaim, testsuite.GetTestNamespace(nil), setGroup, extraArgs...)
				verifyCanRunOnFSPVC(getGuestfsPodName(pvcClaim), testsuite.GetTestNamespace(nil))
			},
				Entry("[test_id:6480]without extra arguments"),
				Entry("[test_id:11670]setting the uid", "--uid", "1002"),
			)

			It("[posneg:negative][test_id:11671]Should fail to run the guestfs command on a PVC in use", func() {
				runGuestfsOnPVC(done, pvcClaim, testsuite.GetTestNamespace(nil), setGroup)
				cmd := guestfsCmd(pvcClaim, testsuite.GetTestNamespace(nil), setGroup)
				Expect(cmd()).To(MatchError(fmt.Sprintf("PVC %s is used by another pod", pvcClaim)))
			})
		})

		FIt("[posneg:positive][test_id:6479]Should successfully run guestfs command on a block-based PVC",
			decorators.Conformance, decorators.RequiresBlockStorage, func() {
				dlog("checking image")
				CheckGuestfsToolsImage()

				dlog("Running guestfs on block PVC, creating PVC %s", pvcClaim)
				libstorage.CreateBlockPVC(pvcClaim, testsuite.GetTestNamespace(nil), "500Mi", libstorage.WithStorageProfile())

				dlog("Starting guestfs on block PVC %s", pvcClaim)
				runGuestfsOnPVC(done, pvcClaim, testsuite.GetTestNamespace(nil), setGroup)

				dlog("Verifying guestfs can run on block PVC %s", pvcClaim)
				stdout, stderr, err := execCommandLibguestfsPod(
					getGuestfsPodName(pvcClaim), testsuite.GetTestNamespace(nil), []string{"guestfish", "-a", "/dev/vda", "run"},
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(stderr).To(BeEmpty())
				Expect(stdout).To(BeEmpty())
				// dlog("fail for debug")
				Fail("fail for debug")
			})
	})

	It("[rfe_id:6364]Should successfully run guestfs command on a filesystem-based PVC with root", func() {
		libstorage.CreateFSPVC(pvcClaim, testsuite.NamespacePrivileged, "500Mi", libstorage.WithStorageProfile())
		runGuestfsOnPVC(done, pvcClaim, testsuite.NamespacePrivileged, false, "--root")
		verifyCanRunOnFSPVC(getGuestfsPodName(pvcClaim), testsuite.NamespacePrivileged)
	})
}))

func getGuestfsPodName(pvc string) string {
	return "libguestfs-tools-" + pvc
}

func execCommandLibguestfsPod(podName, namespace string, args []string) (stdout, stderr string, err error) {
	pod, err := kubevirt.Client().CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())
	return exec.ExecuteCommandOnPodWithResults(pod, "libguestfs", args)
}

func guestfsCmd(pvcClaim, namespace string, setGroup bool, extraArgs ...string) func() error {
	args := append([]string{
		"guestfs", pvcClaim,
		"--namespace", namespace,
	}, extraArgs...)
	if setGroup {
		const testGroup = "2000"
		args = append(args, "--fsGroup", testGroup)
	}
	dlog("making virtctl command with args...")
	for i, arg := range args {
		dlog("---arg %d: %s", i, arg)
	}
	return newRepeatableVirtctlCommand(args...)
}

func runGuestfsOnPVC(done chan struct{}, pvcClaim, namespace string, setGroup bool, extraArgs ...string) {

	dlog("runGuestfsOnPVC %s in namespace %s", pvcClaim, namespace)

	go guestfsWithSync(done, guestfsCmd(pvcClaim, namespace, setGroup, extraArgs...))

	dlog("getting pod name...")
	podName := getGuestfsPodName(pvcClaim)

	dlog("podname is %s", podName)
	// Waiting until the libguestfs pod is running
	Eventually(func(g Gomega) {
		dlog("in eventually, getting pod...")
		pod, err := kubevirt.Client().CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
		dlog("any error %v", err)
		g.Expect(err).ToNot(HaveOccurred())
		dlog("checking pod condition ready...")
		g.Expect(pod).To(matcher.HaveConditionTrue(corev1.ContainersReady))
		dlog("end of eventually loop")
		dlog("we got good pod dump it")

		bytes, _ := yaml.Marshal(pod)
		dlog("pod:\n %s", string(bytes))

	}, 90*time.Second, 2*time.Second).Should(Succeed())
	// Verify that the appliance has been extracted before running any tests by checking the done file
	Eventually(func(g Gomega) {
		_, _, err := execCommandLibguestfsPod(podName, namespace, []string{"ls", "/usr/local/lib/guestfs/appliance/done"})
		g.Expect(err).ToNot(HaveOccurred())
	}, 30*time.Second, 2*time.Second).Should(Succeed())
}

func guestfsWithSync(done chan struct{}, cmd func() error) {
	defer GinkgoRecover()
	dlog("guestfsWithSync(), run command async")
	errChan := make(chan error)
	go func() {
		dlog("---running cmd..")
		errChan <- cmd()
		dlog("---command done")
	}()

	dlog("waiting on cmd and done channels ...")
	select {
	case <-done:
		dlog("done channel returned, test done")
	case err := <-errChan:
		dlog("cmd returns something %v", err)
		Expect(err).ToNot(HaveOccurred())
	}
	dlog("command completed")
}

func verifyCanRunOnFSPVC(podName, namespace string) {
	stdout, stderr, err := execCommandLibguestfsPod(podName, namespace, []string{"qemu-img", "create", "/disk/disk.img", "500M"})
	Expect(stderr).To(BeEmpty())
	Expect(stdout).To(ContainSubstring("Formatting"))
	Expect(err).ToNot(HaveOccurred())
	stdout, stderr, err = execCommandLibguestfsPod(podName, namespace, []string{"guestfish", "-a", "/disk/disk.img", "run"})
	Expect(stderr).To(BeEmpty())
	Expect(stdout).To(BeEmpty())
	Expect(err).ToNot(HaveOccurred())
}
