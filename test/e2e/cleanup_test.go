// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"fmt"
	"time"

	ginkgo "github.com/onsi/ginkgo"
	gomega "github.com/onsi/gomega"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
	"open-cluster-management.io/api/addon/v1alpha1"
	workv1 "open-cluster-management.io/api/work/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = ginkgo.Describe("test cleanup resource after a cluster is detached", func() {
	ginkgo.Context("Importing a self managed cluster and detach the cluster", func() {
		const localClusterName = "local-cluster"
		ginkgo.BeforeEach(func() {
			ginkgo.By(fmt.Sprintf("Create managed cluster %s", localClusterName), func() {
				_, err := util.CreateManagedCluster(hubClusterClient, localClusterName, util.NewLable("local-cluster", "true"))
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})
		})

		ginkgo.AfterEach(func() {
			assertManagedClusterDeleted(localClusterName)
		})

		ginkgo.It("Should not delete addon manifest", func() {
			assertManagedClusterImportSecretApplied(localClusterName)
			assertManagedClusterAvailable(localClusterName)
			assertManagedClusterManifestWorks(localClusterName)

			// apply an add manifestWork
			manifestwork := &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "addon-helloworld-deploy",
					Namespace: localClusterName,
				},
				Spec: workv1.ManifestWorkSpec{
					Workload: workv1.ManifestsTemplate{
						Manifests: []workv1.Manifest{
							{
								RawExtension: runtime.RawExtension{Raw: []byte("{\"apiVersion\": \"v1\", \"kind\": " +
									"\"Namespace\", \"metadata\": {\"name\": \"open-cluster-management-agent-addon\"}}")},
							},
						},
					},
				},
			}
			_, err := hubWorkClient.WorkV1().ManifestWorks(localClusterName).Create(context.TODO(), manifestwork, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// detach the cluster
			err = hubClusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), localClusterName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// addon manifestWork should not be deleted
			checkCount := 0
			gomega.Eventually(func() error {
				_, err := hubWorkClient.WorkV1().ManifestWorks(localClusterName).Get(context.TODO(), manifestwork.GetName(), metav1.GetOptions{})
				if err != nil {
					return err
				}
				checkCount++
				if checkCount > 4 {
					return nil
				}
				return fmt.Errorf("wait 20s to check if manifestwork is deleted")
			}, 1*time.Minute, 5*time.Second).ShouldNot(gomega.HaveOccurred())

			err = hubWorkClient.WorkV1().ManifestWorks(localClusterName).Delete(context.TODO(), manifestwork.GetName(), metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func() error {
				allManifestWorks, err := hubWorkClient.WorkV1().ManifestWorks(localClusterName).List(context.TODO(), metav1.ListOptions{})
				if err != nil {
					if errors.IsNotFound(err) {
						return nil
					}
					return err
				}
				if len(allManifestWorks.Items) == 0 {
					return nil
				}
				return fmt.Errorf("all of the manifestworks should be deleted")
			}, 1*time.Minute, 5*time.Second).ShouldNot(gomega.HaveOccurred())
		})
	})
	ginkgo.Context("Importing a managed cluster and detach the cluster", func() {
		var managedClusterName string
		ginkgo.BeforeEach(func() {
			managedClusterName = fmt.Sprintf("cluster-test-%s", rand.String(6))

			ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
				_, err := util.CreateManagedCluster(hubClusterClient, managedClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})
		})

		ginkgo.AfterEach(func() {
			assertManagedClusterDeleted(managedClusterName)
		})

		ginkgo.It("should clean up the addons with finalizer", func() {
			assertManagedClusterNamespace(managedClusterName)
			// deploy an addon with finalizer
			addon := &v1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{
						"cluster.open-cluster-management.io/addon-pre-delete",
					},
					Name:      "test-addon",
					Namespace: managedClusterName,
				},
				Spec: v1alpha1.ManagedClusterAddOnSpec{
					InstallNamespace: "default",
				},
			}
			_, err := addonClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Create(context.TODO(), addon, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// detach the cluster
			err = hubClusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), managedClusterName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// the addon manifestWork should be force deleted.
			gomega.Eventually(func() error {
				_, err := addonClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Get(context.TODO(), addon.Name, metav1.GetOptions{})
				if err != nil {
					if errors.IsNotFound(err) {
						return nil
					}
					return err
				}
				return fmt.Errorf("expected no addon, but got %v", addon.Name)
			}, 6*time.Minute, 5*time.Second).ShouldNot(gomega.HaveOccurred())
		})
	})
})
