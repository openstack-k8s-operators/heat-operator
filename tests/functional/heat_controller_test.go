/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package functional_test

import (
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	mariadb_test "github.com/openstack-k8s-operators/mariadb-operator/api/test/helpers"

	heatv1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/heat-operator/pkg/heat"
	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

var _ = Describe("Heat controller", func() {

	var heatName types.NamespacedName
	var heatTransportURLName types.NamespacedName
	var heatConfigSecretName types.NamespacedName
	var memcachedName types.NamespacedName
	var memcachedSpec memcachedv1.MemcachedSpec
	var keystoneAPI *keystonev1.KeystoneAPI
	var heatDbSyncName types.NamespacedName

	BeforeEach(func() {

		heatName = types.NamespacedName{
			Name:      "heat",
			Namespace: namespace,
		}
		heatTransportURLName = types.NamespacedName{
			Namespace: namespace,
			Name:      heatName.Name + "-heat-transport",
		}
		heatConfigSecretName = types.NamespacedName{
			Namespace: namespace,
			Name:      heatName.Name + "-config-data",
		}
		memcachedName = types.NamespacedName{
			Name:      "memcached",
			Namespace: namespace,
		}
		memcachedSpec = memcachedv1.MemcachedSpec{
			MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
				Replicas: ptr.To[int32](3),
			},
		}
		heatDbSyncName = types.NamespacedName{
			Name:      "heat-db-sync",
			Namespace: namespace,
		}
		err := os.Setenv("OPERATOR_TEMPLATES", "../../templates")
		Expect(err).NotTo(HaveOccurred())
	})

	When("A Heat instance is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
		})

		It("should have the Spec fields initialized", func() {
			Heat := GetHeat(heatName)
			Expect(Heat.Spec.DatabaseInstance).Should(Equal("openstack"))
			Expect(Heat.Spec.DatabaseAccount).Should(Equal("heat"))
			Expect(Heat.Spec.RabbitMqClusterName).Should(Equal("rabbitmq"))
			Expect(Heat.Spec.ServiceUser).Should(Equal("heat"))
			Expect(*(Heat.Spec.HeatAPI.Replicas)).Should(Equal(int32(1)))
			Expect(*(Heat.Spec.HeatCfnAPI.Replicas)).Should(Equal(int32(1)))
			Expect(*(Heat.Spec.HeatEngine.Replicas)).Should(Equal(int32(1)))
		})

		It("should have the Status fields initialized", func() {
			Heat := GetHeat(heatName)
			Expect(Heat.Status.Hash).To(BeEmpty())
			Expect(Heat.Status.DatabaseHostname).To(Equal(""))
			Expect(Heat.Status.TransportURLSecret).To(Equal(""))
			Expect(Heat.Status.HeatAPIReadyCount).To(Equal(int32(0)))
			Expect(Heat.Status.HeatCfnAPIReadyCount).To(Equal(int32(0)))
			Expect(Heat.Status.HeatEngineReadyCount).To(Equal(int32(0)))
		})

		It("should have input not ready and unknown Conditions initialized", func() {
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)

			for _, cond := range []condition.Type{
				condition.RabbitMqTransportURLReadyCondition,
				condition.MemcachedReadyCondition,
				condition.ServiceConfigReadyCondition,
				condition.DBReadyCondition,
				condition.DBSyncReadyCondition,
				heatv1.HeatStackDomainReadyCondition,
				heatv1.HeatAPIReadyCondition,
				heatv1.HeatCfnAPIReadyCondition,
				heatv1.HeatEngineReadyCondition,
			} {
				th.ExpectCondition(
					heatName,
					ConditionGetterFunc(HeatConditionGetter),
					cond,
					corev1.ConditionUnknown,
				)
			}
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetHeat(heatName).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/heat"))
		})

		It("should not create a config secret", func() {
			th.AssertSecretDoesNotExist(heatConfigSecretName)
		})
	})

	When("The proper secret is provided", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatSecret(namespace, SecretName))
		})

		It("should have input ready", func() {
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.MemcachedReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.RabbitMqTransportURLReadyCondition,
				corev1.ConditionUnknown,
			)
		})

		It("should not create a config secret", func() {
			th.AssertSecretDoesNotExist(heatConfigSecretName)
		})
	})

	When("Memcached is available", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatSecret(namespace, SecretName))
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(memcachedName)
		})

		It("should have memcached ready", func() {
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.MemcachedReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.RabbitMqTransportURLReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionUnknown,
			)
		})
	})

	When("TransportURL Created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatSecret(namespace, SecretName))
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(memcachedName)
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatMessageBusSecret(namespace, HeatMessageBusSecretName))
			infra.SimulateTransportURLReady(heatTransportURLName)
		})

		It("should have transporturl ready", func() {
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.RabbitMqTransportURLReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionUnknown,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionUnknown,
			)
		})

		It("should not create a config secret", func() {
			th.AssertSecretDoesNotExist(heatConfigSecretName)
		})
	})

	When("keystoneAPI instance is available", func() {
		var keystoneAPIName types.NamespacedName
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatSecret(namespace, SecretName))
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(memcachedName)
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatMessageBusSecret(namespace, HeatMessageBusSecretName))
			infra.SimulateTransportURLReady(heatTransportURLName)
			keystoneAPIName = keystone.CreateKeystoneAPI(namespace)
			keystoneAPI = keystone.GetKeystoneAPI(keystoneAPIName)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetHeat(heatName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetHeat(heatName).Spec.DatabaseAccount})
			mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: heat.DatabaseCRName})
		})

		It("should have service config ready", func() {
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("should create a Secret for heat.conf", func() {
			cm := th.GetSecret(heatConfigSecretName)

			memcacheInstance := infra.GetMemcached(memcachedName)
			heatCfg := string(cm.Data["00-default.conf"])
			Expect(heatCfg).Should(
				ContainSubstring("stack_domain_admin=heat_stack_domain_admin"))
			Expect(heatCfg).Should(
				ContainSubstring("auth_uri=%s/v3/ec2tokens", keystoneAPI.Status.APIEndpoints["internal"]))
			Expect(heatCfg).Should(
				ContainSubstring("auth_url=%s", keystoneAPI.Status.APIEndpoints["internal"]))
			Expect(heatCfg).Should(
				ContainSubstring("www_authenticate_uri=http://keystone-internal.openstack.svc"))
			Expect(heatCfg).Should(
				ContainSubstring("backend = dogpile.cache.memcached"))
			Expect(heatCfg).Should(
				ContainSubstring(fmt.Sprintf("memcache_servers = %s", memcacheInstance.GetMemcachedServerListWithInetString())))
			Expect(heatCfg).Should(
				ContainSubstring(fmt.Sprintf("memcached_servers=%s", memcacheInstance.GetMemcachedServerListString())))
			Expect(heatCfg).Should(
				ContainSubstring("tls_enabled=false"))
			Expect(heatCfg).Should(
				ContainSubstring("stack_domain_admin_password=12345678"))
			Expect(string(cm.Data["my.cnf"])).To(
				ContainSubstring("[client]\nssl=0"))
			Expect(string(cm.Data["heat-api-httpd.conf"])).To(
				ContainSubstring(fmt.Sprintf("heat-api-public.%s.svc", heatName.Namespace)))
			Expect(string(cm.Data["heat-cfnapi-httpd.conf"])).To(
				ContainSubstring(fmt.Sprintf("heat-cfnapi-public.%s.svc", heatName.Namespace)))
		})

		It("updates the KeystoneAuthURL if keystone internal endpoint changes", func() {
			newInternalEndpoint := "https://keystone-internal"

			keystone.UpdateKeystoneAPIEndpoint(keystoneAPIName, "internal", newInternalEndpoint)
			logger.Info("Reconfigured")

			Eventually(func(g Gomega) {
				confSecret := th.GetSecret(heatConfigSecretName)
				g.Expect(confSecret).ShouldNot(BeNil())

				conf := string(confSecret.Data["00-default.conf"])
				g.Expect(string(conf)).Should(
					ContainSubstring("auth_url=%s", newInternalEndpoint))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("DB is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatSecret(namespace, SecretName))
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(memcachedName)
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatMessageBusSecret(namespace, HeatMessageBusSecretName))
			infra.SimulateTransportURLReady(heatTransportURLName)
			keystoneAPI := keystone.CreateKeystoneAPI(namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPI)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetHeat(heatName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetHeat(heatName).Spec.DatabaseAccount})
			mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: heat.DatabaseCRName})
		})

		It("should have db ready condition", func() {
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				heatv1.HeatStackDomainReadyCondition,
				corev1.ConditionUnknown,
			)
		})
	})

	When("DB sync is completed", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatSecret(namespace, SecretName))
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(memcachedName)
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatMessageBusSecret(namespace, HeatMessageBusSecretName))
			infra.SimulateTransportURLReady(heatTransportURLName)
			keystoneAPI := keystone.CreateKeystoneAPI(namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPI)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetHeat(heatName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetHeat(heatName).Spec.DatabaseAccount})
			mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: heat.DatabaseCRName})
			dbSyncJobName := types.NamespacedName{
				Name:      "heat-db-sync",
				Namespace: namespace,
			}
			th.SimulateJobSuccess(dbSyncJobName)
		})

		It("should have db sync ready condition", func() {
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				heatv1.HeatStackDomainReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})

	When("heatAPI is configured with CA bundle", func() {
		BeforeEach(func() {
			spec := GetDefaultHeatSpec()
			heatAPI := GetDefaultHeatAPISpec()
			heatAPI["tls"] = map[string]any{
				"caBundleSecretName": "combined-ca-bundle",
			}
			spec["heatAPI"] = heatAPI
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, spec))

			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatSecret(namespace, SecretName))
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			// memcached instance support tls
			infra.SimulateTLSMemcachedReady(memcachedName)
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatMessageBusSecret(namespace, HeatMessageBusSecretName))
			infra.SimulateTransportURLReady(heatTransportURLName)
			keystoneAPIName := keystone.CreateKeystoneAPI(namespace)
			keystoneAPI = keystone.GetKeystoneAPI(keystoneAPIName)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetHeat(heatName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetHeat(heatName).Spec.DatabaseAccount})
			// db supports tls
			mariadb.SimulateMariaDBTLSDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: heat.DatabaseCRName})
		})

		It("should have service config ready", func() {
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("should create a Secret for heat.conf with memcached + DB using tls connection", func() {
			cm := th.GetSecret(heatConfigSecretName)

			memcacheInstance := infra.GetMemcached(memcachedName)
			heatCfg := string(cm.Data["00-default.conf"])
			Expect(heatCfg).Should(
				ContainSubstring("backend = oslo_cache.memcache_pool"))
			Expect(heatCfg).Should(
				ContainSubstring(fmt.Sprintf("memcache_servers = %s", memcacheInstance.GetMemcachedServerListString())))
			Expect(heatCfg).Should(
				ContainSubstring(fmt.Sprintf("memcached_servers=%s", memcacheInstance.GetMemcachedServerListString())))
			Expect(heatCfg).Should(
				ContainSubstring("tls_enabled=true"))
			Expect(string(cm.Data["my.cnf"])).To(
				ContainSubstring("[client]\nssl-ca=/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem\nssl=1"))
		})
	})

	When("heatAPI is created with nodeSelector", func() {
		BeforeEach(func() {
			spec := GetDefaultHeatSpec()
			spec["nodeSelector"] = map[string]any{
				"foo": "bar",
			}
			heatAPI := GetDefaultHeatAPISpec()
			spec["heatAPI"] = heatAPI
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, spec))

			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatSecret(namespace, SecretName))
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(memcachedName)
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatMessageBusSecret(namespace, HeatMessageBusSecretName))
			infra.SimulateTransportURLReady(heatTransportURLName)
			keystoneAPIName := keystone.CreateKeystoneAPI(namespace)
			keystoneAPI = keystone.GetKeystoneAPI(keystoneAPIName)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetHeat(heatName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetHeat(heatName).Spec.DatabaseAccount})
			mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: heat.DatabaseCRName})
			th.SimulateJobSuccess(heatDbSyncName)
			// TODO: assert deployment once it's supported in the tests
		})

		It("sets nodeSelector in resource specs", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetJob(heatDbSyncName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())
		})

		It("updates nodeSelector in resource specs when changed", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetJob(heatDbSyncName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				heat := GetHeat(heatName)
				newNodeSelector := map[string]string{
					"foo2": "bar2",
				}
				heat.Spec.NodeSelector = &newNodeSelector
				g.Expect(k8sClient.Update(ctx, heat)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(th.GetJob(heatDbSyncName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo2": "bar2"}))
			}, timeout, interval).Should(Succeed())
		})

		It("removes nodeSelector from resource specs when cleared", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetJob(heatDbSyncName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				heat := GetHeat(heatName)
				emptyNodeSelector := map[string]string{}
				heat.Spec.NodeSelector = &emptyNodeSelector
				g.Expect(k8sClient.Update(ctx, heat)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(th.GetJob(heatDbSyncName).Spec.Template.Spec.NodeSelector).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("removes nodeSelector from resource specs when nilled", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetJob(heatDbSyncName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				heat := GetHeat(heatName)
				heat.Spec.NodeSelector = nil
				g.Expect(k8sClient.Update(ctx, heat)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(th.GetJob(heatDbSyncName).Spec.Template.Spec.NodeSelector).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})
	})

	// Run MariaDBAccount suite tests.  these are pre-packaged ginkgo tests
	// that exercise standard account create / update patterns that should be
	// common to all controllers that ensure MariaDBAccount CRs.
	mariadbSuite := &mariadb_test.MariaDBTestHarness{
		PopulateHarness: func(harness *mariadb_test.MariaDBTestHarness) {
			harness.Setup(
				"Heat",
				heatName.Namespace,
				heat.DatabaseName,
				"openstack.org/heat",
				mariadb, timeout, interval,
			)
		},

		// Generate a fully running service given an accountName
		// needs to make it all the way to the end where the mariadb finalizers
		// are removed from unused accounts since that's part of what we are testing
		SetupCR: func(accountName types.NamespacedName) {
			spec := GetDefaultHeatSpec()
			spec["databaseAccount"] = accountName.Name

			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatSecret(namespace, SecretName))
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, spec))
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(memcachedName)
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatMessageBusSecret(namespace, HeatMessageBusSecretName))
			infra.SimulateTransportURLReady(heatTransportURLName)
			keystoneAPIName := keystone.CreateKeystoneAPI(namespace)
			keystoneAPI = keystone.GetKeystoneAPI(keystoneAPIName)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetHeat(heatName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			mariadb.SimulateMariaDBAccountCompleted(accountName)
			mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: heat.DatabaseCRName})

			dbSyncJobName := types.NamespacedName{
				Name:      "heat-db-sync",
				Namespace: namespace,
			}
			th.SimulateJobSuccess(dbSyncJobName)

			// TODO(zzzeek) we would prefer to simulate everything else here
			// so we can get to the end of reconcile:
			// * ensureStackDomain passes
			// * engineDeploymentCreateOrUpdate passes
			// * apiDeploymentCreateOrUpdate passes
			// * cfnapiDeploymentCreateOrUpdate

			// then in heat_controller we can move
			// DeleteUnusedMariaDBAccountFinalizers to the end of the reconcile
			// method.
		},
		// Change the account name in the service to a new name
		UpdateAccount: func(newAccountName types.NamespacedName) {

			Eventually(func(g Gomega) {
				heat := GetHeat(heatName)
				heat.Spec.DatabaseAccount = newAccountName.Name
				g.Expect(th.K8sClient.Update(ctx, heat)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

		},
		// delete the CR instance to exercise finalizer removal
		DeleteCR: func() {
			th.DeleteInstance(GetHeat(heatName))
		},
	}

	mariadbSuite.RunBasicSuite()

	mariadbSuite.RunURLAssertSuite(func(_ types.NamespacedName, username string, password string) {
		Eventually(func(g Gomega) {
			cm := th.GetSecret(heatConfigSecretName)

			conf := cm.Data["00-default.conf"]

			g.Expect(string(conf)).Should(
				ContainSubstring(fmt.Sprintf("connection=mysql+pymysql://%s:%s@hostname-for-openstack.%s.svc/heat?read_default_file=/etc/my.cnf",
					username, password, namespace)))

		}).Should(Succeed())

	})

	// TODO(zzzeek) we can also do a CONFIG_HASH test here if we have fixtures
	// that simulate a full deployment
	/* mariadbSuite.RunConfigHashSuite(func() string {
		deployment := th.GetDeployment(names.DeploymentName)
		return GetEnvVarValue(deployment.Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
	})*/

	When("HeatAuthEncryptionKey is too short", func() {

		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatSecret(namespace, SecretName))
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(memcachedName)
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatMessageBusSecret(namespace, HeatMessageBusSecretName))
			infra.SimulateTransportURLReady(heatTransportURLName)
			keystoneAPI := keystone.CreateKeystoneAPI(namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPI)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetHeat(heatName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetHeat(heatName).Spec.DatabaseAccount})
			mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: heat.DatabaseCRName})
			dbSyncJobName := types.NamespacedName{
				Name:      "heat-db-sync",
				Namespace: namespace,
			}
			th.SimulateJobSuccess(dbSyncJobName)

		})

		It("Should complain about the Key length", func() {
			Eventually(func(g Gomega) {
				heat := GetHeat(heatName)
				heat.Spec.PasswordSelectors.AuthEncryptionKey = "TooShortAuthEncKey"
				g.Expect(th.K8sClient.Update(ctx, heat)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionFalse,
			)

			conditions := HeatConditionGetter(heatName)
			message := &conditions.Get(condition.ServiceConfigReadyCondition).Message
			Expect(*message).Should(ContainSubstring("AuthEncryptionKey must be at least 32 characters"))
		})
	})

	When("Quorum queues are enabled", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatSecret(namespace, SecretName))
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(memcachedName)
			DeferCleanup(
				k8sClient.Delete, ctx, infra.CreateTransportURLSecret(namespace, HeatMessageBusSecretName, true))
			infra.SimulateTransportURLReady(heatTransportURLName)
			keystoneAPIName := keystone.CreateKeystoneAPI(namespace)
			keystoneAPI = keystone.GetKeystoneAPI(keystoneAPIName)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetHeat(heatName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetHeat(heatName).Spec.DatabaseAccount})
			mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: heat.DatabaseCRName})
		})

		It("should generate config with quorum queue settings", func() {
			Eventually(func(g Gomega) {
				cm := th.GetSecret(heatConfigSecretName)
				g.Expect(cm).ShouldNot(BeNil())

				heatCfg := string(cm.Data["00-default.conf"])
				g.Expect(heatCfg).Should(ContainSubstring("[oslo_messaging_rabbit]"))
				g.Expect(heatCfg).Should(ContainSubstring("rabbit_quorum_queue=true"))
				g.Expect(heatCfg).Should(ContainSubstring("rabbit_transient_quorum_queue=true"))
				g.Expect(heatCfg).Should(ContainSubstring("amqp_durable_queues=true"))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("Quorum queues are toggled from disabled to enabled", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatSecret(namespace, SecretName))
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(memcachedName)
			DeferCleanup(
				k8sClient.Delete, ctx, infra.CreateTransportURLSecret(namespace, HeatMessageBusSecretName, false))
			infra.SimulateTransportURLReady(heatTransportURLName)
			keystoneAPIName := keystone.CreateKeystoneAPI(namespace)
			keystoneAPI = keystone.GetKeystoneAPI(keystoneAPIName)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetHeat(heatName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetHeat(heatName).Spec.DatabaseAccount})
			mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: heat.DatabaseCRName})
		})

		It("should first generate config without quorum queue settings, then with quorum queue settings after enabling", func() {
			// Step 1: Verify quorum queues are initially disabled
			Eventually(func(g Gomega) {
				cm := th.GetSecret(heatConfigSecretName)
				g.Expect(cm).ShouldNot(BeNil())

				heatCfg := string(cm.Data["00-default.conf"])
				g.Expect(heatCfg).ShouldNot(ContainSubstring("rabbit_quorum_queue=true"))
				g.Expect(heatCfg).ShouldNot(ContainSubstring("rabbit_transient_quorum_queue=true"))
				g.Expect(heatCfg).ShouldNot(ContainSubstring("amqp_durable_queues=true"))
			}, timeout, interval).Should(Succeed())

			// Step 2: Enable quorum queues by updating the transport URL secret
			Eventually(func(g Gomega) {
				// Get the Heat instance to find the actual transport URL secret name
				heat := GetHeat(heatName)
				g.Expect(heat.Status.TransportURLSecret).ShouldNot(BeEmpty())

				transportSecret := &corev1.Secret{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: namespace,
					Name:      heat.Status.TransportURLSecret,
				}, transportSecret)).Should(Succeed())

				transportSecret.Data["quorumqueues"] = []byte("true")
				g.Expect(k8sClient.Update(ctx, transportSecret)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			// Step 3: Verify quorum queues are now enabled in the configuration
			// The controller should detect the secret change and regenerate the config
			Eventually(func(g Gomega) {
				cm := th.GetSecret(heatConfigSecretName)
				g.Expect(cm).ShouldNot(BeNil())

				heatCfg := string(cm.Data["00-default.conf"])
				g.Expect(heatCfg).Should(ContainSubstring("[oslo_messaging_rabbit]"))
				g.Expect(heatCfg).Should(ContainSubstring("rabbit_quorum_queue=true"))
				g.Expect(heatCfg).Should(ContainSubstring("rabbit_transient_quorum_queue=true"))
				g.Expect(heatCfg).Should(ContainSubstring("amqp_durable_queues=true"))
			}, time.Second*30, interval).Should(Succeed())
		})
	})

	When("Quorum queues field is missing", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatSecret(namespace, SecretName))
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(memcachedName)
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatMessageBusSecret(namespace, HeatMessageBusSecretName))
			infra.SimulateTransportURLReady(heatTransportURLName)
			keystoneAPIName := keystone.CreateKeystoneAPI(namespace)
			keystoneAPI = keystone.GetKeystoneAPI(keystoneAPIName)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetHeat(heatName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetHeat(heatName).Spec.DatabaseAccount})
			mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: heat.DatabaseCRName})
		})

		It("should generate config without quorum queue settings", func() {
			Eventually(func(g Gomega) {
				cm := th.GetSecret(heatConfigSecretName)
				g.Expect(cm).ShouldNot(BeNil())

				heatCfg := string(cm.Data["00-default.conf"])
				g.Expect(heatCfg).ShouldNot(ContainSubstring("rabbit_quorum_queue=true"))
				g.Expect(heatCfg).ShouldNot(ContainSubstring("rabbit_transient_quorum_queue=true"))
				g.Expect(heatCfg).ShouldNot(ContainSubstring("amqp_durable_queues=true"))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("an ApplicationCredential is created for Heat", func() {
		var (
			acName                string
			acSecretName          string
			servicePasswordSecret string
			passwordSelector      string
		)
		BeforeEach(func() {
			servicePasswordSecret = "ac-test-osp-secret" //nolint:gosec // G101
			passwordSelector = "HeatPassword"

			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatSecret(namespace, servicePasswordSecret))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatMessageBusSecret(namespace, HeatMessageBusSecretName))
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(memcachedName)

			spec := GetDefaultHeatSpec()
			spec["secret"] = servicePasswordSecret
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, spec))
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetHeat(heatName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(namespace))

			acName = fmt.Sprintf("ac-%s", heat.ServiceName)
			acSecretName = acName + "-secret"
			secret := &corev1.Secret{}
			secret.Name = acSecretName
			secret.Namespace = namespace
			secret.Data = map[string][]byte{
				"AC_ID":     []byte("test-ac-id"),
				"AC_SECRET": []byte("test-ac-secret"),
			}
			DeferCleanup(k8sClient.Delete, ctx, secret)
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			ac := &keystonev1.KeystoneApplicationCredential{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      acName,
				},
				Spec: keystonev1.KeystoneApplicationCredentialSpec{
					UserName:         heat.ServiceName,
					Secret:           servicePasswordSecret,
					PasswordSelector: passwordSelector,
					Roles:            []string{"admin", "member"},
					AccessRules:      []keystonev1.ACRule{{Service: "identity", Method: "POST", Path: "/auth/tokens"}},
					ExpirationDays:   30,
					GracePeriodDays:  5,
				},
			}
			DeferCleanup(k8sClient.Delete, ctx, ac)
			Expect(k8sClient.Create(ctx, ac)).To(Succeed())

			fetched := &keystonev1.KeystoneApplicationCredential{}
			key := types.NamespacedName{Namespace: ac.Namespace, Name: ac.Name}
			Expect(k8sClient.Get(ctx, key, fetched)).To(Succeed())

			fetched.Status.SecretName = acSecretName
			now := metav1.Now()
			readyCond := condition.Condition{
				Type:               condition.ReadyCondition,
				Status:             corev1.ConditionTrue,
				Reason:             condition.ReadyReason,
				Message:            condition.ReadyMessage,
				LastTransitionTime: now,
			}
			fetched.Status.Conditions = condition.Conditions{readyCond}
			Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())

			infra.SimulateTransportURLReady(heatTransportURLName)
			mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetHeat(heatName).Spec.DatabaseAccount})
			mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: heat.DatabaseCRName})
		})

		It("should configure Heat to use application credentials", func() {
			Eventually(func(g Gomega) {
				cm := th.GetSecret(heatConfigSecretName)
				g.Expect(cm).ShouldNot(BeNil())

				heatCfg := string(cm.Data["00-default.conf"])
				// Verify [trustee] section uses application credentials
				g.Expect(heatCfg).Should(ContainSubstring("auth_type = v3applicationcredential"))
				g.Expect(heatCfg).Should(ContainSubstring("application_credential_id = test-ac-id"))
				g.Expect(heatCfg).Should(ContainSubstring("application_credential_secret = test-ac-secret"))
				// Verify no password authentication is used (should not contain auth_type=password)
				g.Expect(heatCfg).Should(Not(ContainSubstring("application_credential_secret = password")))
			}, timeout, interval).Should(Succeed())
		})
	})
})
