package deploy_test

import (
	"fmt"
	"sync"

	etcdclient "acceptance-tests/testing/etcd"
	"acceptance-tests/testing/helpers"

	"github.com/pivotal-cf-experimental/bosh-test/bosh"
	"github.com/pivotal-cf-experimental/destiny/etcd"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Scaling up instances", func() {
	ScaleUpInstances := func(enableSSL bool) {

		var (
			manifest   etcd.Manifest
			etcdClient etcdclient.Client

			testKey   string
			testValue string
		)

		BeforeEach(func() {
			guid, err := helpers.NewGUID()
			Expect(err).NotTo(HaveOccurred())

			testKey = "etcd-key-" + guid
			testValue = "etcd-value-" + guid

			manifest, err = helpers.DeployEtcdWithInstanceCount(1, client, config, enableSSL)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() ([]bosh.VM, error) {
				return client.DeploymentVMs(manifest.Name)
			}, "1m", "10s").Should(ConsistOf(helpers.GetVMsFromManifest(manifest)))
		})

		AfterEach(func() {
			if !CurrentGinkgoTestDescription().Failed {
				err := client.DeleteDeployment(manifest.Name)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("scales from 1 to 3 nodes", func() {
			var keyVals map[string]string

			By("setting a persistent value", func() {
				etcdClient = etcdclient.NewClient(fmt.Sprintf("http://%s:6769", manifest.Jobs[2].Networks[0].StaticIPs[0]))

				err := etcdClient.Set(testKey, testValue)
				Expect(err).ToNot(HaveOccurred())
			})

			By("scaling up to 3 nodes", func() {
				manifest.Jobs[1], manifest.Properties = etcd.SetJobInstanceCount(manifest.Jobs[1], manifest.Networks[0], manifest.Properties, 3)

				members := manifest.EtcdMembers()
				Expect(members).To(HaveLen(3))

				yaml, err := manifest.ToYAML()
				Expect(err).NotTo(HaveOccurred())

				var wg sync.WaitGroup
				done := make(chan struct{})

				keysChan := helpers.SpamEtcd(done, &wg, etcdClient)

				_, err = client.Deploy(yaml)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() ([]bosh.VM, error) {
					return client.DeploymentVMs(manifest.Name)
				}, "1m", "10s").Should(ConsistOf(helpers.GetVMsFromManifest(manifest)))

				close(done)
				wg.Wait()
				keyVals = <-keysChan

				if err, ok := keyVals["error"]; ok {
					Fail(err)
				}

			})

			By("reading the value from each etcd node in the cluster", func() {
				value, err := etcdClient.Get(testKey)
				Expect(err).ToNot(HaveOccurred())
				Expect(value).To(Equal(testValue))

				for k, v := range keyVals {
					value, err := etcdClient.Get(k)
					Expect(err).ToNot(HaveOccurred())
					Expect(value).To(Equal(v))
				}
			})

		})
	}

	Context("without TLS", func() {
		ScaleUpInstances(false)
	})

	Context("with TLS", func() {
		ScaleUpInstances(true)
	})
})
