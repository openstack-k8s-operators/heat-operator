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

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	corev1 "k8s.io/api/core/v1"
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
	var memcachedSpec memcachedv1.MemcachedSpec
	var keystoneAPI *keystonev1.KeystoneAPI

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
		memcachedSpec = memcachedv1.MemcachedSpec{
			MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
				Replicas: ptr.To[int32](3),
			},
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
			infra.SimulateMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})
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
			infra.SimulateMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})
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
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatSecret(namespace, SecretName))
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})
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
				ContainSubstring(fmt.Sprintf("memcache_servers=memcached-0.memcached.%s.svc:11211,memcached-1.memcached.%s.svc:11211,memcached-2.memcached.%s.svc:11211",
					heatName.Namespace, heatName.Namespace, heatName.Namespace)))
			Expect(heatCfg).Should(
				ContainSubstring(fmt.Sprintf("memcached_servers=inet:[memcached-0.memcached.%s.svc]:11211,inet:[memcached-1.memcached.%s.svc]:11211,inet:[memcached-2.memcached.%s.svc]:11211",
					heatName.Namespace, heatName.Namespace, heatName.Namespace)))
			Expect(heatCfg).Should(
				ContainSubstring("tls_enabled=false"))
			Expect(string(cm.Data["my.cnf"])).To(
				ContainSubstring("[client]\nssl=0"))
			Expect(string(cm.Data["heat-api-httpd.conf"])).To(
				ContainSubstring(fmt.Sprintf("heat-api-public.%s.svc", heatName.Namespace)))
			Expect(string(cm.Data["heat-cfnapi-httpd.conf"])).To(
				ContainSubstring(fmt.Sprintf("heat-cfnapi-public.%s.svc", heatName.Namespace)))
		})
	})

	When("DB is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatSecret(namespace, SecretName))
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})
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
			infra.SimulateMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})
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
			heatAPI["tls"] = map[string]interface{}{
				"caBundleSecretName": "combined-ca-bundle",
			}
			spec["heatAPI"] = heatAPI
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, spec))

			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatSecret(namespace, SecretName))
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			// memcached instance support tls
			infra.SimulateTLSMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})
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

			heatCfg := string(cm.Data["00-default.conf"])
			Expect(heatCfg).Should(
				ContainSubstring(fmt.Sprintf("memcache_servers=memcached-0.memcached.%s.svc:11211,memcached-1.memcached.%s.svc:11211,memcached-2.memcached.%s.svc:11211",
					heatName.Namespace, heatName.Namespace, heatName.Namespace)))
			Expect(heatCfg).Should(
				ContainSubstring(fmt.Sprintf("memcached_servers=inet:[memcached-0.memcached.%s.svc]:11211,inet:[memcached-1.memcached.%s.svc]:11211,inet:[memcached-2.memcached.%s.svc]:11211",
					heatName.Namespace, heatName.Namespace, heatName.Namespace)))
			Expect(heatCfg).Should(
				ContainSubstring("tls_enabled=true"))
			Expect(string(cm.Data["my.cnf"])).To(
				ContainSubstring("[client]\nssl-ca=/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem\nssl=1"))
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
			infra.SimulateMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})
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
			infra.SimulateMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})
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
})
